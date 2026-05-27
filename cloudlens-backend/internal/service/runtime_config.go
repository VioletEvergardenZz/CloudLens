// 本文件用于 FileService 运行态配置。
// 文件职责：读取配置快照、持久化控制台配置，并热更新 watcher/upload 组件。

package service

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/config"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/logger"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/match"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/models"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/pathutil"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/state"
)

// Config 返回当前配置的副本
func (fs *FileService) Config() *models.Config {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.config == nil {
		return nil
	}
	cfgCopy := *fs.config
	return &cfgCopy
}

// persistRuntimeConfig 将控制台更新的配置写入运行时配置文件
func (fs *FileService) persistRuntimeConfig(cfg *models.Config) {
	if cfg == nil {
		return
	}
	if strings.TrimSpace(fs.configPath) == "" {
		return
	}
	if err := config.SaveRuntimeConfig(fs.configPath, cfg); err != nil {
		logger.Warn("write runtime config failed: %v", err)
	}
}

// UpdateConfig 更新运行时配置并重建监听器与上传池
// UpdateConfig 负责运行态配置热更新
// 仅允许更新白名单字段 其余静态策略保持重启生效原则
func (fs *FileService) UpdateConfig(watchDir, fileExt, silence string, uploadWorkers, uploadQueueSize int, uploadRetryDelays string, uploadRetryEnabled *bool, systemResourceEnabled *bool) (*models.Config, error) {
	// 先加锁读取当前配置与组件引用
	fs.mu.Lock()
	if fs.config == nil {
		// 配置不存在时直接返回
		fs.mu.Unlock()
		return nil, fmt.Errorf("配置未初始化")
	}
	// 保存旧组件用于失败回滚
	oldCfg := fs.config
	oldState := fs.state
	oldWatcher := fs.watcher
	oldPool := fs.uploadPool
	oldOSS := fs.ossClient
	oldPersistQueue := fs.persistQueue
	// 复制旧配置作为更新基线
	current := *oldCfg
	fs.mu.Unlock()

	// 基于当前配置构造更新版
	updated := current

	// 处理 watchDir 更新并做校验
	if strings.TrimSpace(watchDir) != "" && strings.TrimSpace(watchDir) != current.WatchDir {
		normalized, err := normalizeWatchDirs(strings.TrimSpace(watchDir))
		if err != nil {
			return nil, err
		}
		updated.WatchDir = normalized
	}
	// 处理 fileExt 更新并做校验
	if strings.TrimSpace(fileExt) != current.FileExt {
		if err := validateFileExt(strings.TrimSpace(fileExt)); err != nil {
			return nil, err
		}
		updated.FileExt = strings.TrimSpace(fileExt)
	}
	// 处理静默窗口更新
	if strings.TrimSpace(silence) != "" && strings.TrimSpace(silence) != current.Silence {
		updated.Silence = strings.TrimSpace(silence)
	}
	// 处理上传 worker 数量更新
	if uploadWorkers > 0 && uploadWorkers != current.UploadWorkers {
		updated.UploadWorkers = uploadWorkers
	}
	// 处理上传队列长度更新
	if uploadQueueSize > 0 && uploadQueueSize != current.UploadQueueSize {
		updated.UploadQueueSize = uploadQueueSize
	}
	if strings.TrimSpace(uploadRetryDelays) != "" && strings.TrimSpace(uploadRetryDelays) != current.UploadRetryDelays {
		updated.UploadRetryDelays = strings.TrimSpace(uploadRetryDelays)
	}
	currentRetryEnabled := true
	if current.UploadRetryEnabled != nil {
		currentRetryEnabled = *current.UploadRetryEnabled
	}
	if uploadRetryEnabled != nil && *uploadRetryEnabled != currentRetryEnabled {
		enabled := *uploadRetryEnabled
		updated.UploadRetryEnabled = &enabled
	}
	if systemResourceEnabled != nil && *systemResourceEnabled != current.SystemResourceEnabled {
		updated.SystemResourceEnabled = *systemResourceEnabled
	}

	// 基于新配置初始化运行态
	newState := state.NewRuntimeState(&updated)
	if err := newState.BootstrapExisting(); err != nil {
		logger.Warn("预加载历史文件失败: %v", err)
	}
	// 迁移旧的计数与历史数据
	newState.CarryOverFrom(oldState)

	// 初始化新的 OSS 客户端
	newOSS, err := newOSSClient(&updated)
	if err != nil {
		return nil, err
	}

	// 初始化新的上传工作池
	newPool, newPersistQueue, err := newUploadPool(&updated, fs.processFile, fs.handlePoolStats)
	if err != nil {
		return nil, err
	}

	// watcher 为空时新建实例
	activeWatcher := oldWatcher
	if activeWatcher == nil {
		created, createErr := newFileWatcher(&updated, fs)
		if createErr != nil {
			_ = newPool.ShutdownGraceful(shutdownTimeout)
			return nil, createErr
		}
		activeWatcher = created
	}

	// 切换到新配置与新组件
	// 先原子替换再启动或重置 watcher，避免运行中读到半更新状态
	fs.mu.Lock()
	fs.config = &updated
	fs.state = newState
	fs.uploadPool = newPool
	fs.persistQueue = newPersistQueue
	fs.watcher = activeWatcher
	fs.ossClient = newOSS
	fs.state.SetQueueStats(fs.uploadPool.GetStats())
	fs.mu.Unlock()

	if oldWatcher == nil {
		// 使用新配置启动 watcher
		if err := activeWatcher.Start(); err != nil {
			// watcher 启动失败则回滚
			// 回滚顺序保持和替换顺序相反，确保资源引用一致
			_ = newPool.ShutdownGraceful(shutdownTimeout)
			_ = activeWatcher.Close()
			// 回滚到旧配置，避免留下已关闭的工作池
			fs.mu.Lock()
			fs.config = oldCfg
			fs.state = oldState
			fs.uploadPool = oldPool
			fs.persistQueue = oldPersistQueue
			fs.watcher = oldWatcher
			fs.ossClient = oldOSS
			if fs.state != nil && fs.uploadPool != nil {
				fs.state.SetQueueStats(fs.uploadPool.GetStats())
			}
			fs.mu.Unlock()
			return nil, err
		}
	} else {
		// 已存在 watcher 则按新配置重置
		if err := activeWatcher.Reset(&updated); err != nil {
			// reset 失败则回滚
			// 旧 watcher 仍可继续工作，因此只需丢弃新建组件
			_ = newPool.ShutdownGraceful(shutdownTimeout)
			fs.mu.Lock()
			fs.config = oldCfg
			fs.state = oldState
			fs.uploadPool = oldPool
			fs.persistQueue = oldPersistQueue
			fs.watcher = oldWatcher
			fs.ossClient = oldOSS
			if fs.state != nil && fs.uploadPool != nil {
				fs.state.SetQueueStats(fs.uploadPool.GetStats())
			}
			fs.mu.Unlock()
			return nil, err
		}
	}

	// 新 watcher 启动后关闭旧组件
	if oldPool != nil {
		_ = oldPool.ShutdownGraceful(shutdownTimeout)
	}
	if oldOSS != nil {
		// oss sdk 无需显式关闭客户端，保留占位以示释放顺序
	}

	logger.Info("运行时配置已更新: watchDir=%s, fileExt=%s, silence=%s, workers=%d, queue=%d",
		updated.WatchDir,
		updated.FileExt,
		updated.Silence,
		updated.UploadWorkers,
		updated.UploadQueueSize,
	)

	fs.persistRuntimeConfig(&updated)
	return fs.Config(), nil
}

// normalizeWatchDirs 校验并规范化监控目录列表
func normalizeWatchDirs(raw string) (string, error) {
	dirs := pathutil.SplitWatchDirs(raw)
	if len(dirs) == 0 {
		return "", fmt.Errorf("监控目录不能为空")
	}
	normalized := make([]string, 0, len(dirs))
	seen := make(map[string]struct{})
	for _, dir := range dirs {
		absPath, err := filepath.Abs(dir)
		if err != nil {
			return "", fmt.Errorf("监控目录无效: %w", err)
		}
		stat, statErr := os.Stat(absPath)
		if statErr != nil {
			return "", fmt.Errorf("监控目录无效: %w", statErr)
		}
		if !stat.IsDir() {
			return "", fmt.Errorf("监控目录不是一个目录")
		}
		key := normalizeWatchDirKey(absPath)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, absPath)
	}
	return strings.Join(normalized, ","), nil
}

// normalizeWatchDirKey 生成目录去重键
// Windows 下统一转小写，避免盘符大小写导致重复监听
func normalizeWatchDirKey(path string) string {
	key := filepath.ToSlash(path)
	if runtime.GOOS == "windows" {
		key = strings.ToLower(key)
	}
	return key
}

// validateFileExt 校验文件后缀格式
func validateFileExt(ext string) error {
	if strings.TrimSpace(ext) == "" {
		// 允许空字符串，表示不过滤后缀
		return nil
	}
	// 复用多后缀解析进行格式校验
	if _, err := match.ParseExtList(strings.TrimSpace(ext)); err != nil {
		return err
	}
	return nil
}
