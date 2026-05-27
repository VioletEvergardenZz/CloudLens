// 本文件用于 FileService 生命周期。
// 文件职责：启动和停止 watcher、告警管理器、上传工作池。

package service

import (
	"fmt"

	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/logger"
)

// Start 启动文件服务
// Start 启动监听 告警和上传执行链路
// 启动失败时直接返回错误 避免服务处于“部分可用”状态
func (fs *FileService) Start() error {
	logger.Info("启动文件服务...")
	// 先启动文件监听 再启动告警轮询
	if err := fs.watcher.Start(); err != nil {
		return fmt.Errorf("启动文件监控失败: %w", err)
	}
	if fs.alertManager != nil {
		fs.alertManager.Start()
	}
	fs.mu.Lock()
	fs.running = true
	fs.mu.Unlock()
	logger.Info("文件服务启动成功")
	return nil
}

// Stop 停止文件服务
// Stop 按 watcher -> alert -> upload 的顺序关闭
// 顺序关闭可以减少“新任务继续入队但消费已停止”的竞态窗口
func (fs *FileService) Stop() error {
	logger.Info("停止文件服务...")
	fs.mu.Lock()
	fs.running = false
	fs.mu.Unlock()
	if fs.alertManager != nil {
		fs.alertManager.Stop()
	}
	if fs.uploadPool != nil {
		if err := fs.uploadPool.ShutdownGraceful(shutdownTimeout); err != nil {
			logger.Warn("关闭上传工作池超时，已发出取消信号: %v", err)
		}
	}
	if fs.watcher != nil {
		if err := fs.watcher.Close(); err != nil {
			logger.Error("关闭文件监控器失败: %v", err)
		}
	}
	logger.Info("文件服务已停止")
	return nil
}
