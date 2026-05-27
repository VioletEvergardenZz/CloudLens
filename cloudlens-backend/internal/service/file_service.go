// 本文件用于文件监控服务的核心协作流程
// 文件职责：实现当前模块的核心业务逻辑与数据流转
// 关键路径：入口参数先校验再执行业务处理 最后返回统一结果
// 边界与容错：异常场景显式返回错误 由上层决定重试或降级

package service

import (
	"strings"
	"sync"
	"time"

	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/alert"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/dingtalk"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/email"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/logger"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/models"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/oss"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/persistqueue"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/state"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/upload"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/watcher"
)

// FileService 负责协调文件监控、上传与通知流程
type FileService struct {
	config        *models.Config
	configPath    string
	ossClient     *oss.Client
	dingtalkRobot *dingtalk.Robot
	emailSender   *email.Sender
	uploadPool    *upload.WorkerPool
	persistQueue  *persistqueue.FileQueue
	watcher       *watcher.FileWatcher
	alertManager  *alert.Manager
	state         *state.RuntimeState
	running       bool
	mu            sync.Mutex      //互斥锁，用来保护 FileService 内部共享数据的并发读写
	manualOnce    map[string]bool //标记“某个路径的下一次处理是手动上传”
	metricsMu     sync.Mutex
	queueFull     uint64
	queueShed     uint64
	retryTotal    uint64
	uploadFailure uint64
	failReasons   map[string]uint64
}

const shutdownTimeout = 30 * time.Second
const defaultUploadQueuePersistFile = "logs/upload-queue.json"
const defaultQueueSaturationThreshold = 0.9

var defaultUploadRetryDelays = []time.Duration{
	1 * time.Second,
	2 * time.Second,
	5 * time.Second,
}

const (
	defaultUploadRetryMaxAttempts = 4
	maxUploadRetryDelay           = 60 * time.Second
)

// NewFileService 构造并初始化 FileService 的依赖
// 初始化顺序固定为 状态 -> 存储客户端 -> 告警管理 -> 上传池 -> 监听器
// 这样任一环节失败都能在启动前暴露 不把半初始化实例暴露给外层
func NewFileService(config *models.Config, configPath string) (*FileService, error) {
	runtimeState := state.NewRuntimeState(config)
	if err := runtimeState.BootstrapExisting(); err != nil {
		logger.Warn("预加载历史文件失败: %v", err)
	}

	ossClient, err := newOSSClient(config)
	if err != nil {
		return nil, err
	}

	fileService := &FileService{
		config:        config,
		configPath:    strings.TrimSpace(configPath),
		ossClient:     ossClient,
		dingtalkRobot: newDingTalkRobot(config),
		emailSender:   newEmailSender(config),
		state:         runtimeState,
		manualOnce:    make(map[string]bool),
		failReasons:   make(map[string]uint64),
	}
	// 初始化告警管理器并复用通知器
	alertManager, err := alert.NewManager(config, &alert.NotifierSet{
		DingTalk: fileService.dingtalkRobot,
		Email:    fileService.emailSender,
	})
	if err != nil {
		return nil, err
	}
	fileService.alertManager = alertManager

	uploadPool, persistStore, err := newUploadPool(config, fileService.processFile, fileService.handlePoolStats)
	if err != nil {
		return nil, err
	}
	fileService.uploadPool = uploadPool
	fileService.persistQueue = persistStore
	runtimeState.SetQueueStats(uploadPool.GetStats())

	fileWatcher, err := newFileWatcher(config, fileService)
	if err != nil {
		return nil, err
	}
	fileService.watcher = fileWatcher

	return fileService, nil
}
