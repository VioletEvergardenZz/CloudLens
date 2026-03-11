// 本文件用于定义告警输入源与镜像路径
// 文件职责：兼容文件路径与 Docker 容器日志源，保持控制台配置入口稳定
// 关键路径：先标准化配置字符串，再按类型拆分给对应采集器
// 边界与容错：显式返回格式错误，避免运行后才发现配置不可用

package alert

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultDockerHost      = "unix:///var/run/docker.sock"
	defaultAlertMirrorRoot = "logs/alert-sources"
)

type logSourceKind string

const (
	logSourceFile            logSourceKind = "file"
	logSourceDockerContainer logSourceKind = "docker_container"
	logSourceDockerService   logSourceKind = "docker_service"
)

type logSource struct {
	Raw      string
	Kind     logSourceKind
	Path     string
	Selector string
	Project  string
}

// normalizeLogSources 用于标准化配置中的告警输入源列表。
func normalizeLogSources(raw string) ([]string, error) {
	parts := splitLogSourceTokens(raw)
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{})
	for _, part := range parts {
		normalized, err := normalizeLogSourceToken(part)
		if err != nil {
			return nil, err
		}
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out, nil
}

// splitAlertLogSources 用于把标准化后的输入源拆分给文件与 Docker 采集器。
func splitAlertLogSources(sources []string) ([]string, []logSource, error) {
	filePaths := make([]string, 0, len(sources))
	dockerSources := make([]logSource, 0, len(sources))
	for _, source := range sources {
		spec, err := parseLogSourceToken(source)
		if err != nil {
			return nil, nil, err
		}
		switch spec.Kind {
		case logSourceFile:
			filePaths = append(filePaths, spec.Path)
		case logSourceDockerContainer, logSourceDockerService:
			dockerSources = append(dockerSources, spec)
		default:
			return nil, nil, fmt.Errorf("不支持的告警输入源类型: %s", source)
		}
	}
	return filePaths, dockerSources, nil
}

// HasDockerLogSource 用于判断是否存在 docker:// 形式的日志源。
func HasDockerLogSource(raw string) bool {
	for _, part := range splitLogSourceTokens(raw) {
		if isDockerSourceToken(part) {
			return true
		}
	}
	return false
}

// AlertSourceMirrorRoot 返回容器日志镜像落盘目录。
func AlertSourceMirrorRoot() string {
	cwd, err := os.Getwd()
	if err != nil || strings.TrimSpace(cwd) == "" {
		return filepath.Clean(defaultAlertMirrorRoot)
	}
	return filepath.Join(cwd, defaultAlertMirrorRoot)
}

func splitLogSourceTokens(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r' || r == '\t' || r == ' ' || r == '，' || r == '；'
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

func normalizeLogSourceToken(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	if !isDockerSourceToken(trimmed) {
		cleaned := filepath.Clean(filepath.FromSlash(trimmed))
		if cleaned == "" || cleaned == "." {
			return "", nil
		}
		return cleaned, nil
	}
	return normalizeDockerSourceToken(trimmed)
}

func parseLogSourceToken(normalized string) (logSource, error) {
	if !isDockerSourceToken(normalized) {
		return logSource{Raw: normalized, Kind: logSourceFile, Path: normalized}, nil
	}
	parsed, err := url.Parse(normalized)
	if err != nil {
		return logSource{}, fmt.Errorf("docker 日志源格式无效: %w", err)
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Host))
	segments := splitPathSegments(parsed.Path)
	switch host {
	case "container":
		if len(segments) != 1 || strings.TrimSpace(segments[0]) == "" {
			return logSource{}, fmt.Errorf("docker 容器日志源格式应为 docker://container/<容器名或ID>")
		}
		return logSource{Raw: normalized, Kind: logSourceDockerContainer, Selector: segments[0]}, nil
	case "service":
		if len(segments) != 1 || strings.TrimSpace(segments[0]) == "" {
			return logSource{}, fmt.Errorf("docker 服务日志源格式应为 docker://service/<服务名>?project=<compose项目>")
		}
		return logSource{Raw: normalized, Kind: logSourceDockerService, Selector: segments[0], Project: strings.TrimSpace(parsed.Query().Get("project"))}, nil
	case "compose":
		if len(segments) != 2 || strings.TrimSpace(segments[0]) == "" || strings.TrimSpace(segments[1]) == "" {
			return logSource{}, fmt.Errorf("docker compose 日志源格式应为 docker://compose/<项目>/<服务>")
		}
		return logSource{Raw: normalized, Kind: logSourceDockerService, Selector: segments[1], Project: segments[0]}, nil
	default:
		return logSource{}, fmt.Errorf("不支持的 docker 日志源类型: %s", normalized)
	}
}

func normalizeDockerSourceToken(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("docker 日志源格式无效: %w", err)
	}
	if !strings.EqualFold(parsed.Scheme, "docker") {
		return "", fmt.Errorf("不支持的 docker 日志源: %s", raw)
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Host))
	segments := splitPathSegments(parsed.Path)
	switch host {
	case "":
		if len(segments) != 1 || strings.TrimSpace(segments[0]) == "" {
			return "", fmt.Errorf("docker 日志源缺少容器名称: %s", raw)
		}
		return fmt.Sprintf("docker://container/%s", segments[0]), nil
	case "container":
		if len(segments) != 1 || strings.TrimSpace(segments[0]) == "" {
			return "", fmt.Errorf("docker 容器日志源格式应为 docker://container/<容器名或ID>")
		}
		return fmt.Sprintf("docker://container/%s", segments[0]), nil
	case "service":
		if len(segments) != 1 || strings.TrimSpace(segments[0]) == "" {
			return "", fmt.Errorf("docker 服务日志源格式应为 docker://service/<服务名>?project=<compose项目>")
		}
		selector := segments[0]
		project := strings.TrimSpace(parsed.Query().Get("project"))
		if project == "" {
			return fmt.Sprintf("docker://service/%s", selector), nil
		}
		query := url.Values{}
		query.Set("project", project)
		return fmt.Sprintf("docker://service/%s?%s", selector, query.Encode()), nil
	case "compose":
		if len(segments) != 2 || strings.TrimSpace(segments[0]) == "" || strings.TrimSpace(segments[1]) == "" {
			return "", fmt.Errorf("docker compose 日志源格式应为 docker://compose/<项目>/<服务>")
		}
		return fmt.Sprintf("docker://compose/%s/%s", segments[0], segments[1]), nil
	default:
		if len(segments) == 0 {
			return fmt.Sprintf("docker://container/%s", host), nil
		}
		return "", fmt.Errorf("不支持的 docker 日志源类型: %s", raw)
	}
}

func isDockerSourceToken(value string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(value)), "docker://")
}

func splitPathSegments(path string) []string {
	rawSegments := strings.Split(strings.Trim(path, "/"), "/")
	out := make([]string, 0, len(rawSegments))
	for _, segment := range rawSegments {
		decoded, err := url.PathUnescape(segment)
		if err != nil {
			decoded = segment
		}
		cleaned := strings.TrimSpace(decoded)
		if cleaned == "" {
			continue
		}
		out = append(out, cleaned)
	}
	return out
}
