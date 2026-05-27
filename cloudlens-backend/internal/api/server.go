// 本文件用于 API 服务与路由处理
// 文件职责：实现当前模块的核心业务逻辑与数据流转
// 关键路径：入口参数先校验再执行业务处理 最后返回统一结果
// 边界与容错：异常场景显式返回错误 由上层决定重试或降级

package api

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/alert"
	appcloud "github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/app/cloud"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/kb"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/logger"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/metrics"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/models"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/service"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/store"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/sysinfo"
)

// Server 管理接口服务的启动与关闭
// 统一启动/停止 HTTP 服务
type Server struct {
	httpServer *http.Server //负责监听端口、接收请求、管理超时/连接等
	kbService  *kb.Service
	controlDB  *controlSQLiteStore
	cloudDB    *cloudAccountStore
	resourceDB *store.ResourceIndexStore
}

// 请求处理器
type handler struct {
	cfg *models.Config
	fs  *service.FileService
	sys *sysinfo.Collector
	kb  *kb.Service
	// alertStateOverride 仅用于测试注入，运行态统一通过 fs.AlertState() 读取
	alertStateOverride *alert.State

	controlMu           sync.RWMutex
	controlAgents       map[string]controlAgentState
	controlAgentKeyIdx  map[string]string
	controlTasks        map[string]controlTaskState
	controlNextAgentSeq uint64
	controlNextTaskSeq  uint64
	controlStore        *controlSQLiteStore
	cloudStore          *cloudAccountStore
	resourceIndexStore  *store.ResourceIndexStore
	resourceService     *appcloud.ResourceService
	nodeLinkService     *appcloud.NodeLinkService
	domainProber        registryDomainProbeRunner

	dashboardCacheMu     sync.Mutex
	dashboardCacheData   any
	dashboardCacheExpire time.Time
	dashboardCacheTTL    time.Duration
}

// 日志读取的限制常量
const (
	maxFileLogBytes        = 512 * 1024  //最多读取 512KB 的内容
	maxFileLogLines        = 500         //最多返回 500 行内容
	maxFileSearchLines     = 2000        //最多返回 2000 行匹配结果
	maxFileSearchLineBytes = 1024 * 1024 //单行最大 1MB，避免超长行撑爆内存
	defaultDashboardTTL    = 2 * time.Second
)

// NewServer 创建接口服务并注册路由
// 路由层只做编排 不在这里写业务细节 便于后续按域拆分 handler
// 中间件顺序固定为 recovery -> cors，保证异常与跨域行为可预期
func NewServer(cfg *models.Config, fs *service.FileService) *Server {
	kbService, err := kb.NewService("")
	if err != nil {
		logger.Error("知识库初始化失败: %v", err)
	}
	controlStore, err := newControlSQLiteStore("")
	if err != nil {
		logger.Warn("控制面存储初始化失败，已降级内存模式: %v", err)
	}
	cloudStore, err := newCloudAccountStore("")
	if err != nil {
		logger.Warn("云账号存储初始化失败，云账号配置接口不可用: %v", err)
	}
	var resourceIndexStore *store.ResourceIndexStore
	if cloudStore != nil {
		resourceIndexStore, err = store.NewResourceIndexStore(cloudStore.DBPath())
		if err != nil {
			logger.Warn("资源索引存储初始化失败，统一资源接口将降级: %v", err)
		}
	}
	h := &handler{
		cfg:                cfg,
		fs:                 fs,
		sys:                sysinfo.NewCollector(sysinfo.Options{}),
		kb:                 kbService,
		controlAgents:      make(map[string]controlAgentState),
		controlAgentKeyIdx: make(map[string]string),
		controlTasks:       make(map[string]controlTaskState),
		controlStore:       controlStore,
		cloudStore:         cloudStore,
		resourceIndexStore: resourceIndexStore,
		resourceService:    appcloud.NewResourceService(resourceIndexStore),
		nodeLinkService:    appcloud.NewNodeLinkService(),
		domainProber:       newRegistryDomainProber(registryDomainProbeOptions{}),
		dashboardCacheTTL:  defaultDashboardTTL,
	}
	if h.controlStore != nil {
		if err := h.loadControlSnapshot(); err != nil {
			logger.Warn("控制面存储加载失败，已降级内存模式: %v", err)
			_ = h.controlStore.Close()
			h.controlStore = nil
		} else {
			logger.Info("控制面存储已加载: %s", h.controlStore.DBPath())
		}
	}
	if h.cloudStore != nil {
		logger.Info("云账号存储已加载: %s", h.cloudStore.DBPath())
	}
	if h.resourceIndexStore != nil {
		logger.Info("云资源索引已加载")
	}
	srv := &http.Server{
		Addr:         cfg.APIBind,
		Handler:      withRecovery(withCORS(cfg, h.routes())),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: resolveWriteTimeout(cfg),
	}
	return &Server{httpServer: srv, kbService: kbService, controlDB: h.controlStore, cloudDB: h.cloudStore, resourceDB: h.resourceIndexStore}
}

// prometheusMetrics 导出 Prometheus 采集格式的运行指标
// 这里在读指标前先做一次状态快照汇总 避免各指标来源时间点不一致
func (h *handler) prometheusMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if h != nil && h.fs != nil {
		metrics.Global().SetQueueStats(h.fs.GetStats())
	}
	if h != nil {
		now := time.Now().UTC()
		h.controlMu.RLock()
		agentTotal := len(h.controlAgents)
		agentOnline := 0
		var heartbeatLagMax time.Duration
		for _, agent := range h.controlAgents {
			if agent.Status == "draining" {
				continue
			}
			if !controlAgentIsActive(agent, now) {
				continue
			}
			agentOnline++
			if !agent.LastSeenAt.IsZero() {
				lag := now.Sub(agent.LastSeenAt)
				if lag > heartbeatLagMax {
					heartbeatLagMax = lag
				}
			}
		}
		taskCounts := make(map[string]uint64)
		backlog := 0
		for _, task := range h.controlTasks {
			key := task.Status + "|" + task.Type
			taskCounts[key]++
			if task.Status == "pending" {
				backlog++
			}
		}
		h.controlMu.RUnlock()

		metrics.Global().SetControlAgentSnapshot(agentTotal, agentOnline, heartbeatLagMax)
		metrics.Global().SetControlTaskSnapshot(backlog, taskCounts)
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = w.Write([]byte(metrics.MustGlobalPrometheus()))
}

// Start 启动接口服务并开始监听
func (s *Server) Start() {
	go func() {
		logger.Info("API 服务监听 %s", s.httpServer.Addr)
		//过滤掉“正常关闭”，只记录真正的异常
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("API 服务异常退出: %v", err)
		}
	}()
}

// Shutdown 优雅关闭接口服务
func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}
	err := s.httpServer.Shutdown(ctx)
	if s.kbService != nil {
		if closeErr := s.kbService.Close(); closeErr != nil {
			logger.Warn("关闭知识库失败: %v", closeErr)
		}
	}
	if s.controlDB != nil {
		if closeErr := s.controlDB.Close(); closeErr != nil {
			logger.Warn("关闭控制面存储失败: %v", closeErr)
		}
	}
	if s.cloudDB != nil {
		if closeErr := s.cloudDB.Close(); closeErr != nil {
			logger.Warn("关闭云账号存储失败: %v", closeErr)
		}
	}
	if s.resourceDB != nil {
		if closeErr := s.resourceDB.Close(); closeErr != nil {
			logger.Warn("关闭云资源索引失败: %v", closeErr)
		}
	}
	return err
}
