// 本文件用于构造 FileService 依赖。
// 文件职责：集中初始化 OSS、通知器、上传池和文件监听器。

package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/dingtalk"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/email"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/logger"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/metrics"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/models"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/oss"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/persistqueue"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/upload"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/watcher"
)

// newOSSClient 初始化 OSS 客户端
func newOSSClient(config *models.Config) (*oss.Client, error) {
	if !ossSettingsConfigured(config) {
		logger.Info("OSS 未配置，文件入云链路进入空闲模式")
		return nil, nil
	}
	if !ossSettingsComplete(config) {
		return nil, fmt.Errorf("OSS配置不完整，启用文件入云时必须配置 bucket、ak、sk、endpoint、region")
	}
	client, err := oss.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("初始化OSS客户端失败: %w", err)
	}
	return client, nil
}

func ossSettingsConfigured(config *models.Config) bool {
	if config == nil {
		return false
	}
	return strings.TrimSpace(config.Bucket) != "" ||
		strings.TrimSpace(config.AK) != "" ||
		strings.TrimSpace(config.SK) != "" ||
		strings.TrimSpace(config.Endpoint) != "" ||
		strings.TrimSpace(config.Region) != ""
}

func ossSettingsComplete(config *models.Config) bool {
	if config == nil {
		return false
	}
	return strings.TrimSpace(config.Bucket) != "" &&
		strings.TrimSpace(config.AK) != "" &&
		strings.TrimSpace(config.SK) != "" &&
		strings.TrimSpace(config.Endpoint) != "" &&
		strings.TrimSpace(config.Region) != ""
}

// newDingTalkRobot 根据配置创建钉钉机器人
func newDingTalkRobot(config *models.Config) *dingtalk.Robot {
	if config.DingTalkWebhook == "" {
		return nil
	}
	return dingtalk.NewRobot(config.DingTalkWebhook, config.DingTalkSecret)
}

// newEmailSender 根据配置创建邮件发送器
func newEmailSender(config *models.Config) *email.Sender {
	// 读取 SMTP 主机配置
	host := strings.TrimSpace(config.EmailHost)
	if host == "" {
		// 未配置则不启用邮件通知
		return nil
	}

	// 解析收件人列表
	recipients := parseEmailRecipients(config.EmailTo)
	if len(recipients) == 0 {
		// 无收件人则禁用
		logger.Warn("邮件通知未启用: email_to 为空")
		return nil
	}

	// 优先使用配置的 From
	from := strings.TrimSpace(config.EmailFrom)
	if from == "" && strings.Contains(config.EmailUser, "@") {
		// 若未设置 From 则退回到账号作为发件人
		from = strings.TrimSpace(config.EmailUser)
	}
	if from == "" {
		// 仍为空则不启用
		logger.Warn("邮件通知未启用: email_from 为空")
		return nil
	}

	// 读取端口与 TLS 配置
	port := config.EmailPort
	useTLS := config.EmailUseTLS
	if port <= 0 {
		// 未指定端口时根据 TLS 选择默认值
		if useTLS {
			port = 587
		} else {
			port = 25
		}
	}
	if port == 465 {
		// 465 端口强制启用 TLS
		useTLS = true
	}
	if port <= 0 || port > 65535 {
		// 端口非法则不启用
		logger.Warn("邮件通知未启用: email_port 无效")
		return nil
	}

	// 生成 SMTP 发送器
	return email.NewSender(host, port, config.EmailUser, config.EmailPass, from, recipients, useTLS)
}

// parseEmailRecipients 解析收件人列表
func parseEmailRecipients(raw string) []string {
	// 支持逗号分号空白等分隔符
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r' || r == '\t' || r == ' '
	})
	// 过滤空项并保留顺序
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		// 逐项清理空格
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

// newUploadPool 创建上传工作池
func newUploadPool(config *models.Config, handler func(context.Context, string) error, onStats func(models.UploadStats)) (*upload.WorkerPool, *persistqueue.FileQueue, error) {
	var queueStore upload.QueueStore
	var persistStore *persistqueue.FileQueue
	if config.UploadQueuePersistEnabled {
		storePath := strings.TrimSpace(config.UploadQueuePersistFile)
		if storePath == "" {
			storePath = defaultUploadQueuePersistFile
		}
		store, err := persistqueue.NewFileQueue(storePath)
		if err != nil {
			return nil, nil, fmt.Errorf("初始化上传持久化队列失败: %w", err)
		}
		persistStore = store
		queueStore = store
		logger.Info("上传持久化队列已启用: %s", storePath)
	}
	pool, err := upload.NewWorkerPool(config.UploadWorkers, config.UploadQueueSize, handler, onStats, queueStore)
	if err != nil {
		return nil, nil, err
	}
	return pool, persistStore, nil
}

// handlePoolStats 将队列统计写入运行态
func (fs *FileService) handlePoolStats(stats models.UploadStats) {
	if fs.state != nil {
		fs.state.SetQueueStats(stats)
	}
	metrics.Global().SetQueueStats(stats)
}

// newFileWatcher 创建文件监听器
func newFileWatcher(config *models.Config, uploadPool watcher.UploadPool) (*watcher.FileWatcher, error) {
	fileWatcher, err := watcher.NewFileWatcher(config, uploadPool)
	if err != nil {
		return nil, fmt.Errorf("初始化文件监控器失败: %w", err)
	}
	return fileWatcher, nil
}
