// 本文件用于文件上传执行链路。
// 文件职责：处理入队、上传重试、通知发送和上传相关指标。

package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/email"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/logger"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/metrics"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/models"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/upload"
)

// processFile 处理单个文件：上传、触发构建、发送通知
// processFile 是上传执行主路径
// 它统一负责上传 重试 通知和运行态更新 避免分散状态更新导致状态不一致
func (fs *FileService) processFile(ctx context.Context, filePath string) error {
	start := time.Now()
	manual := fs.consumeManualOnce(filePath)
	// 手动上传不受自动开关限制
	if fs.state != nil && !manual && !fs.state.AutoUploadEnabled(filePath) {
		fs.state.MarkSkipped(filePath)
		return nil
	}

	logger.Info("开始处理文件: %s", filePath)
	downloadURL, err := fs.uploadFileWithRetry(ctx, filePath)
	if err != nil {
		fs.recordUploadFailure(err)
		if fs.state != nil {
			fs.state.MarkFailed(filePath, err)
		}
		return fmt.Errorf("上传文件到OSS失败: %w", err)
	}

	fileName := filepath.Base(filePath)
	logger.Info("文件信息 - 文件名: %s", fileName)

	if fs.state != nil {
		fs.state.MarkUploaded(filePath, downloadURL, time.Since(start), manual)
	}
	metrics.Global().ObserveUploadSuccess(time.Since(start))

	// 自动上传开关与上传执行并发发生时，可能出现“上传已完成但开关已关闭”的窗口。
	// 这里做二次检查，确保关闭后不会继续向外发送“File uploaded”通知。
	if fs.state != nil && !manual && !fs.state.AutoUploadEnabled(filePath) {
		logger.Info("文件在上传期间关闭自动上传，已跳过通知: %s", filePath)
		logger.Info("文件处理完成: %s", filePath)
		return nil
	}

	fullPath := filepath.Clean(filePath)
	aiSummary := ""
	if fs.dingtalkRobot != nil || fs.emailSender != nil {
		aiSummary = fs.buildUploadAISummary(ctx, fullPath)
	}
	fs.sendDingTalk(ctx, downloadURL, fullPath, aiSummary)
	fs.sendEmailNotification(ctx, downloadURL, fullPath, aiSummary)

	logger.Info("文件处理完成: %s", filePath)
	return nil
}

// uploadFileWithRetry 负责上传重试，避免短暂失败导致任务丢失
// uploadFileWithRetry 把重试策略与上传调用绑定
// 每次失败都记录可观测指标 便于区分瞬时波动与持续性故障
func (fs *FileService) uploadFileWithRetry(ctx context.Context, filePath string) (string, error) {
	if fs.ossClient == nil {
		return "", fmt.Errorf("OSS客户端未初始化")
	}
	if !isUploadRetryEnabled(fs.config) {
		return fs.ossClient.UploadFile(ctx, filePath)
	}
	delays := buildUploadRetryPlan(fs.config)
	tries := len(delays) + 1
	var lastErr error
	for attempt := 1; attempt <= tries; attempt++ {
		if ctx != nil && ctx.Err() != nil {
			return "", ctx.Err()
		}
		downloadURL, err := fs.ossClient.UploadFile(ctx, filePath)
		if err == nil {
			return downloadURL, nil
		}
		lastErr = err
		if attempt == tries {
			break
		}
		// 失败后按配置间隔退避，降低瞬时抖动导致的连续失败
		delay := delays[attempt-1]
		fs.recordRetryAttempt()
		logger.Warn("上传失败，准备重试: %s, 第 %d/%d 次, 等待 %v, 错误: %v", filePath, attempt, tries, delay, err)
		if err := sleepWithContext(ctx, delay); err != nil {
			return "", err
		}
	}
	return "", lastErr
}

// isUploadRetryEnabled 判断是否启用上传重试
func isUploadRetryEnabled(cfg *models.Config) bool {
	if cfg == nil {
		return true
	}
	if cfg.UploadRetryEnabled == nil {
		return true
	}
	return *cfg.UploadRetryEnabled
}

// parseUploadRetryDelays 解析上传重试间隔配置
func parseUploadRetryDelays(cfg *models.Config) []time.Duration {
	if cfg == nil {
		return defaultUploadRetryDelays
	}
	raw := strings.TrimSpace(cfg.UploadRetryDelays)
	if raw == "" {
		return defaultUploadRetryDelays
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		switch r {
		case ',', ';', ' ', '\n', '\r', '\t':
			return true
		default:
			return false
		}
	})
	delays := make([]time.Duration, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		d, err := time.ParseDuration(trimmed)
		if err != nil || d <= 0 {
			logger.Warn("上传重试间隔解析失败，已忽略: %s", trimmed)
			continue
		}
		delays = append(delays, d)
	}
	if len(delays) == 0 {
		return defaultUploadRetryDelays
	}
	return delays
}

// resolveUploadRetryMaxAttempts 解析重试上限，包含首次尝试。
func resolveUploadRetryMaxAttempts(cfg *models.Config) int {
	if cfg == nil {
		return defaultUploadRetryMaxAttempts
	}
	if cfg.UploadRetryMaxAttempts <= 0 {
		return defaultUploadRetryMaxAttempts
	}
	if cfg.UploadRetryMaxAttempts > 20 {
		return 20
	}
	return cfg.UploadRetryMaxAttempts
}

// buildUploadRetryPlan 构建退避计划，返回每次重试前等待时长。
func buildUploadRetryPlan(cfg *models.Config) []time.Duration {
	maxAttempts := resolveUploadRetryMaxAttempts(cfg)
	if maxAttempts <= 1 {
		return nil
	}
	need := maxAttempts - 1
	base := parseUploadRetryDelays(cfg)
	plan := make([]time.Duration, 0, need)
	for _, delay := range base {
		if len(plan) >= need {
			break
		}
		if delay <= 0 {
			continue
		}
		plan = append(plan, delay)
	}
	if len(plan) == 0 {
		plan = append(plan, defaultUploadRetryDelays[0])
	}
	for len(plan) < need {
		next := plan[len(plan)-1] * 2
		if next > maxUploadRetryDelay {
			next = maxUploadRetryDelay
		}
		plan = append(plan, next)
	}
	return plan
}

// sleepWithContext 支持在等待期间响应停止信号
func sleepWithContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	if ctx == nil {
		time.Sleep(delay)
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// consumeManualOnce 消费单次手动上传标记
func (fs *FileService) consumeManualOnce(path string) bool {
	norm := normalizeManualPath(path)
	if norm == "" {
		return false
	}
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.manualOnce[norm] {
		delete(fs.manualOnce, norm)
		return true
	}
	return false
}

// normalizeManualPath 归一化手动上传路径
func normalizeManualPath(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(abs)
}

// formatHostPath 用于格式化输出内容
func formatHostPath(filePath string) string {
	host, err := os.Hostname()
	if err != nil {
		host = ""
	}
	host = strings.TrimSpace(host)
	if host == "" {
		host = "unknown-host"
	}
	cleaned := filepath.ToSlash(filepath.Clean(filePath))
	cleaned = strings.TrimPrefix(cleaned, "/")
	if cleaned == "" {
		return host
	}
	return host + "/" + cleaned
}

// sendDingTalk 发送钉钉通知
func (fs *FileService) sendDingTalk(ctx context.Context, downloadURL, fileName, aiSummary string) {
	if fs.dingtalkRobot == nil {
		return
	}
	displayName := formatHostPath(fileName)
	if err := fs.dingtalkRobot.SendMessage(ctx, downloadURL, displayName, aiSummary); err != nil {
		logger.Error("发送钉钉消息失败: %v", err)
		return
	}
	if fs.state != nil {
		fs.state.RecordNotification("dingtalk")
	}
}

// sendEmailNotification 发送邮件通知
func (fs *FileService) sendEmailNotification(ctx context.Context, downloadURL, filePath, aiSummary string) {
	// 未配置邮件发送器则跳过
	if fs.emailSender == nil {
		return
	}
	// 读取主机名用于邮件内容
	host, _ := os.Hostname()
	// 邮件主题与内容与钉钉保持一致
	subject := "File uploaded"
	body := fmt.Sprintf(
		"Time: %s\nHost: %s\nFile: %s\nDownload: %s\n",
		time.Now().Format("2006-01-02 15:04:05"),
		host,
		formatHostPath(filePath),
		downloadURL,
	)
	if strings.TrimSpace(aiSummary) != "" {
		body += fmt.Sprintf("AI Analysis: %s\n", formatNotificationAISummary(aiSummary))
	}
	// 发送邮件通知
	if err := fs.emailSender.SendMessage(ctx, subject, body); err != nil {
		// QUIT 异常视为已发送但连接结束异常
		if email.IsQuitError(err) {
			logger.Warn("邮件通知已发送但连接退出异常: %v", err)
			if fs.state != nil {
				// 仍记录通知次数
				fs.state.RecordNotification("email")
			}
			return
		}
		// 非 QUIT 异常记为发送失败
		logger.Error("发送邮件通知失败: %v", err)
		return
	}
	if fs.state != nil {
		// 发送成功记录通知次数
		fs.state.RecordNotification("email")
	}
}

// AddFile 实现 watcher.UploadPool 用于入队监控到的文件
func (fs *FileService) AddFile(filePath string) error {
	return fs.enqueueFile(filePath, false)
}

// EnqueueManualUpload 允许 API 触发手动上传
func (fs *FileService) EnqueueManualUpload(filePath string) error {
	return fs.enqueueFile(filePath, true)
}

// enqueueFile 将文件加入上传队列并更新状态
// enqueueFile 统一处理自动与手动入队
// 入队前会执行队列饱和判断与熔断策略 防止雪崩式堆积
func (fs *FileService) enqueueFile(filePath string, manual bool) error {
	norm := normalizeManualPath(filePath)
	if fs.state != nil {
		if manual {
			fs.state.MarkManualQueued(filePath)
			if norm != "" {
				fs.mu.Lock()
				fs.manualOnce[norm] = true
				fs.mu.Unlock()
			}
		} else if !fs.state.AutoUploadEnabled(filePath) {
			logger.Debug("自动上传关闭，跳过入队: %s", filePath)
			fs.state.MarkSkipped(filePath)
			return nil
		} else {
			fs.state.MarkQueued(filePath)
		}
	}
	if fs.uploadPool == nil {
		return fmt.Errorf("上传工作池未初始化")
	}
	if !manual && fs.shouldShedByQueueSaturation() {
		err := fmt.Errorf("upload queue saturated")
		fs.recordQueueShed()
		if fs.state != nil {
			fs.state.MarkFailed(filePath, err)
			fs.state.SetQueueStats(fs.uploadPool.GetStats())
		}
		return err
	}
	if err := fs.uploadPool.AddFile(filePath); err != nil {
		if errors.Is(err, upload.ErrQueueFull) {
			// 队列满单独记指标，便于区分是容量问题还是上传失败
			fs.recordQueueFull()
		}
		if fs.state != nil {
			fs.state.MarkFailed(filePath, err)
			fs.state.SetQueueStats(fs.uploadPool.GetStats())
		}
		return err
	}
	metrics.Global().IncFileEvent()
	if fs.state != nil {
		fs.state.SetQueueStats(fs.uploadPool.GetStats())
	}
	return nil
}

// isQueueCircuitBreakerEnabled 判断是否启用队列饱和熔断。
func isQueueCircuitBreakerEnabled(cfg *models.Config) bool {
	if cfg == nil {
		return true
	}
	if cfg.UploadQueueCircuitBreakerEnabled == nil {
		return true
	}
	return *cfg.UploadQueueCircuitBreakerEnabled
}

// resolveQueueSaturationThreshold 解析队列饱和阈值，范围 (0,1]。
func resolveQueueSaturationThreshold(cfg *models.Config) float64 {
	if cfg == nil {
		return defaultQueueSaturationThreshold
	}
	threshold := cfg.UploadQueueSaturationThreshold
	if threshold <= 0 || threshold > 1 {
		return defaultQueueSaturationThreshold
	}
	return threshold
}

func resolveUploadQueueSize(cfg *models.Config) int {
	if cfg == nil {
		return 100
	}
	if cfg.UploadQueueSize <= 0 {
		return 100
	}
	return cfg.UploadQueueSize
}

// shouldShedByQueueSaturation 在队列接近满载时触发限流，避免持续堆积。
// shouldShedByQueueSaturation 在入队前做背压判定
// 判定逻辑只依赖当前队列快照 保持快速且无阻塞
func (fs *FileService) shouldShedByQueueSaturation() bool {
	if fs == nil || fs.uploadPool == nil {
		return false
	}
	fs.mu.Lock()
	cfg := fs.config
	fs.mu.Unlock()
	if !isQueueCircuitBreakerEnabled(cfg) {
		return false
	}
	queueCap := resolveUploadQueueSize(cfg)
	if queueCap <= 0 {
		return false
	}
	stats := fs.uploadPool.GetStats()
	ratio := float64(stats.QueueLength) / float64(queueCap)
	if ratio < resolveQueueSaturationThreshold(cfg) {
		return false
	}
	logger.Warn("上传队列触发限流: queue=%d cap=%d ratio=%.2f", stats.QueueLength, queueCap, ratio)
	return true
}
