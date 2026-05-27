// 本文件用于 FileService 告警运行态。
// 文件职责：暴露告警状态，并支持控制台热更新告警配置与规则。

package service

import (
	"fmt"
	"strings"

	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/alert"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/models"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/state"
)

// State 暴露运行态给 API 服务
func (fs *FileService) State() *state.RuntimeState {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.state
}

// AlertState 暴露告警运行态给 API 服务
func (fs *FileService) AlertState() *alert.State {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.alertManager == nil {
		return nil
	}
	return fs.alertManager.State()
}

// AlertEnabled 返回告警是否启用
func (fs *FileService) AlertEnabled() bool {
	fs.mu.Lock()
	manager := fs.alertManager
	cfg := fs.config
	fs.mu.Unlock()
	if manager != nil {
		return manager.Enabled()
	}
	if cfg == nil {
		return false
	}
	return cfg.AlertEnabled
}

// UpdateAlertConfig 运行时更新告警配置（仅内存）
func (fs *FileService) UpdateAlertConfig(enabled bool, suppressEnabled bool, rulesFile, logPaths, pollInterval string, startFromEnd bool) (*models.Config, error) {
	fs.mu.Lock()
	if fs.config == nil {
		fs.mu.Unlock()
		return nil, fmt.Errorf("配置未初始化")
	}
	current := *fs.config
	manager := fs.alertManager
	running := fs.running
	fs.mu.Unlock()

	updated := current
	updated.AlertEnabled = enabled
	updated.AlertSuppressEnabled = &suppressEnabled
	updated.AlertRulesFile = ""
	updated.AlertLogPaths = strings.TrimSpace(logPaths)
	updated.AlertPollInterval = strings.TrimSpace(pollInterval)
	updated.AlertStartFromEnd = &startFromEnd
	if strings.TrimSpace(updated.AlertPollInterval) == "" {
		updated.AlertPollInterval = "2s"
	}

	// 告警管理器按需创建或热更新
	if manager == nil {
		if enabled {
			newManager, err := alert.NewManager(&updated, &alert.NotifierSet{
				DingTalk: fs.dingtalkRobot,
				Email:    fs.emailSender,
			})
			if err != nil {
				return nil, err
			}
			manager = newManager
			if running && manager != nil {
				manager.Start()
			}
		}
	} else {
		if err := manager.UpdateConfig(alert.ConfigUpdate{
			Enabled:         enabled,
			SuppressEnabled: suppressEnabled,
			LogPaths:        updated.AlertLogPaths,
			PollInterval:    updated.AlertPollInterval,
			StartFromEnd:    startFromEnd,
		}, running); err != nil {
			return nil, err
		}
	}

	fs.mu.Lock()
	fs.config = &updated
	fs.alertManager = manager
	fs.mu.Unlock()

	fs.persistRuntimeConfig(&updated)
	return fs.Config(), nil
}

// UpdateAlertRules 运行时更新告警规则并持久化
func (fs *FileService) UpdateAlertRules(ruleset *alert.Ruleset) (*models.Config, error) {
	fs.mu.Lock()
	if fs.config == nil {
		fs.mu.Unlock()
		return nil, fmt.Errorf("配置未初始化")
	}
	current := *fs.config
	manager := fs.alertManager
	running := fs.running
	fs.mu.Unlock()

	if ruleset == nil {
		return nil, fmt.Errorf("告警规则不能为空")
	}
	if err := alert.NormalizeRuleset(ruleset); err != nil {
		return nil, err
	}
	current.AlertRules = ruleset

	if manager == nil {
		if current.AlertEnabled {
			newManager, err := alert.NewManager(&current, &alert.NotifierSet{
				DingTalk: fs.dingtalkRobot,
				Email:    fs.emailSender,
			})
			if err != nil {
				return nil, err
			}
			manager = newManager
			if running && manager != nil {
				manager.Start()
			}
		}
	} else {
		if err := manager.UpdateRules(ruleset); err != nil {
			return nil, err
		}
	}

	fs.mu.Lock()
	fs.config = &current
	fs.alertManager = manager
	fs.mu.Unlock()

	fs.persistRuntimeConfig(&current)
	return fs.Config(), nil
}
