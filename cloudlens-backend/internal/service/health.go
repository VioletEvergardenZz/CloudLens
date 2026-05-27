// 本文件用于 FileService 健康快照和上传指标。
// 文件职责：聚合队列、持久化队列、失败原因等运行态指标。

package service

import (
	"sort"
	"strings"

	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/metrics"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/models"
)

// GetStats 获取服务统计信息
func (fs *FileService) GetStats() models.UploadStats {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.uploadPool != nil {
		return fs.uploadPool.GetStats()
	}
	return models.UploadStats{}
}

// HealthSnapshot 返回运行健康指标
func (fs *FileService) HealthSnapshot() models.HealthSnapshot {
	queueStats := fs.GetStats()
	persistHealth := models.PersistQueueHealth{}

	fs.mu.Lock()
	cfg := fs.config
	persistStore := fs.persistQueue
	fs.mu.Unlock()

	if cfg != nil && cfg.UploadQueuePersistEnabled {
		persistHealth.Enabled = true
		persistHealth.StoreFile = strings.TrimSpace(cfg.UploadQueuePersistFile)
		if persistHealth.StoreFile == "" {
			persistHealth.StoreFile = defaultUploadQueuePersistFile
		}
	}
	if persistStore != nil {
		stats := persistStore.HealthStats()
		if strings.TrimSpace(stats.StoreFile) != "" {
			persistHealth.StoreFile = stats.StoreFile
		}
		persistHealth.RecoveredTotal = stats.RecoveredTotal
		persistHealth.CorruptFallbackTotal = stats.CorruptFallbackTotal
		persistHealth.PersistWriteFailureTotal = stats.PersistWriteFailureTotal
	}

	fs.metricsMu.Lock()
	snapshot := models.HealthSnapshot{
		QueueLength:        queueStats.QueueLength,
		Workers:            queueStats.Workers,
		InFlight:           queueStats.InFlight,
		QueueFullTotal:     fs.queueFull,
		QueueShedTotal:     fs.queueShed,
		RetryTotal:         fs.retryTotal,
		UploadFailureTotal: fs.uploadFailure,
		FailureReasons:     make([]models.FailureReasonCount, 0, len(fs.failReasons)),
		PersistQueue:       persistHealth,
	}
	for reason, count := range fs.failReasons {
		snapshot.FailureReasons = append(snapshot.FailureReasons, models.FailureReasonCount{
			Reason: reason,
			Count:  count,
		})
	}
	fs.metricsMu.Unlock()

	sort.Slice(snapshot.FailureReasons, func(i, j int) bool {
		if snapshot.FailureReasons[i].Count == snapshot.FailureReasons[j].Count {
			return snapshot.FailureReasons[i].Reason < snapshot.FailureReasons[j].Reason
		}
		return snapshot.FailureReasons[i].Count > snapshot.FailureReasons[j].Count
	})
	if len(snapshot.FailureReasons) > 10 {
		snapshot.FailureReasons = snapshot.FailureReasons[:10]
	}
	return snapshot
}

// recordQueueFull 记录上传队列满次数
func (fs *FileService) recordQueueFull() {
	fs.metricsMu.Lock()
	fs.queueFull++
	fs.metricsMu.Unlock()
	metrics.Global().IncQueueFull()
}

// recordQueueShed 记录饱和阈值触发的限流次数
func (fs *FileService) recordQueueShed() {
	fs.metricsMu.Lock()
	fs.queueShed++
	fs.metricsMu.Unlock()
	metrics.Global().IncQueueShed()
}

// recordRetryAttempt 记录上传重试次数
func (fs *FileService) recordRetryAttempt() {
	fs.metricsMu.Lock()
	fs.retryTotal++
	fs.metricsMu.Unlock()
	metrics.Global().IncUploadRetry()
}

// recordUploadFailure 记录上传失败次数与失败原因分布
func (fs *FileService) recordUploadFailure(err error) {
	fs.metricsMu.Lock()
	fs.uploadFailure++
	if fs.failReasons == nil {
		fs.failReasons = make(map[string]uint64)
	}
	reason := normalizeFailureReason(err)
	fs.failReasons[reason]++
	fs.metricsMu.Unlock()
	metrics.Global().ObserveUploadFailure(reason)
}

// normalizeFailureReason 将错误信息规整为可聚合的统计键
func normalizeFailureReason(err error) string {
	if err == nil {
		return "unknown"
	}
	reason := strings.TrimSpace(err.Error())
	if reason == "" {
		return "unknown"
	}
	if len(reason) > 120 {
		return reason[:120]
	}
	return reason
}
