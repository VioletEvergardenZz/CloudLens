// 本文件用于 Docker 容器日志采集
// 文件职责：通过 Docker Engine API 跟随容器日志，并镜像到本地文件复用既有链路
// 关键路径：刷新发现容器 -> 为每个目标容器建立独立流协程 -> 持续写入镜像文件并回调规则引擎
// 边界与容错：Docker 不可用时只记录轮询错误并降级，不阻塞后端主服务启动

package alert

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/logger"
)

type dockerContainerInfo struct {
	ID     string            `json:"Id"`
	Names  []string          `json:"Names"`
	Labels map[string]string `json:"Labels"`
}

type dockerStreamHandle struct {
	cancel context.CancelFunc
}

type dockerAPI struct {
	client   *http.Client
	baseURL  string
	endpoint string
}

// DockerTailer 使用“刷新发现 + 独立流协程”的模型采集容器日志。
// 这样做的原因是 compose 重建容器后 ID 会变化，必须周期性重新发现；
// 同时每个容器独立跟随，避免单个流异常拖垮其他业务系统的告警采集。
type DockerTailer struct {
	api          *dockerAPI
	sources      []logSource
	interval     time.Duration
	startFromEnd bool
	onLine       func(path, line string)
	onPoll       func(at time.Time, err error)

	mu          sync.Mutex
	streams     map[string]*dockerStreamHandle
	cursorSince map[string]time.Time
	mirrorRoot  string
}

// NewDockerTailer 创建 Docker 容器日志采集器。
func NewDockerTailer(sources []logSource, interval time.Duration, startFromEnd bool, onLine func(path, line string), onPoll func(at time.Time, err error)) (*DockerTailer, error) {
	if len(sources) == 0 {
		return nil, nil
	}
	api, err := newDockerAPIFromEnv()
	if err != nil {
		return nil, err
	}
	return &DockerTailer{
		api:          api,
		sources:      append([]logSource(nil), sources...),
		interval:     interval,
		startFromEnd: startFromEnd,
		onLine:       onLine,
		onPoll:       onPoll,
		streams:      make(map[string]*dockerStreamHandle),
		cursorSince:  make(map[string]time.Time),
		mirrorRoot:   filepath.Join(AlertSourceMirrorRoot(), "docker"),
	}, nil
}

// Run 启动 Docker 日志发现与跟随循环。
func (t *DockerTailer) Run(ctx context.Context) {
	if t == nil {
		return
	}
	if t.interval <= 0 {
		t.interval = 2 * time.Second
	}
	t.refreshOnce(ctx)
	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			t.stopAllStreams()
			return
		case <-ticker.C:
			t.refreshOnce(ctx)
		}
	}
}

func (t *DockerTailer) refreshOnce(ctx context.Context) {
	now := time.Now()
	err := t.refresh(ctx, now)
	if t.onPoll != nil {
		t.onPoll(now, err)
	}
}

func (t *DockerTailer) refresh(ctx context.Context, now time.Time) error {
	containers, err := t.api.listRunningContainers(ctx)
	if err != nil {
		return err
	}
	matched := make(map[string]dockerContainerInfo)
	for _, container := range containers {
		if t.matchesAnySource(container) {
			matched[container.ID] = container
		}
	}

	t.mu.Lock()
	toStart := make([]dockerContainerInfo, 0, len(matched))
	for id, handle := range t.streams {
		if _, ok := matched[id]; ok {
			continue
		}
		handle.cancel()
		delete(t.streams, id)
	}
	for id, container := range matched {
		if _, ok := t.streams[id]; ok {
			continue
		}
		t.streams[id] = &dockerStreamHandle{cancel: func() {}}
		if _, ok := t.cursorSince[id]; !ok && t.startFromEnd {
			t.cursorSince[id] = now
		}
		toStart = append(toStart, container)
	}
	t.mu.Unlock()

	for _, container := range toStart {
		go t.runStream(ctx, container)
	}
	return nil
}

func (t *DockerTailer) runStream(parent context.Context, container dockerContainerInfo) {
	t.mu.Lock()
	handle := t.streams[container.ID]
	since := t.cursorSince[container.ID]
	t.mu.Unlock()
	if handle == nil {
		return
	}

	streamCtx, cancel := context.WithCancel(parent)
	t.mu.Lock()
	current := t.streams[container.ID]
	if current != handle {
		t.mu.Unlock()
		cancel()
		return
	}
	current.cancel = cancel
	t.mu.Unlock()
	defer func() {
		cancel()
		t.removeStream(container.ID, handle)
	}()

	mirrorPath := dockerMirrorPath(t.mirrorRoot, container)
	if err := os.MkdirAll(filepath.Dir(mirrorPath), 0o755); err != nil {
		logger.Warn("Docker 日志镜像目录创建失败: container=%s err=%v", container.ID, err)
		return
	}
	file, err := os.OpenFile(mirrorPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		logger.Warn("Docker 日志镜像文件打开失败: container=%s err=%v", container.ID, err)
		return
	}
	defer file.Close()

	body, err := t.api.openContainerLogs(streamCtx, container.ID, since)
	if err != nil {
		if streamCtx.Err() == nil {
			logger.Warn("Docker 日志流打开失败: container=%s err=%v", container.ID, err)
		}
		return
	}
	defer body.Close()

	collector := &dockerLineCollector{
		mirrorPath: mirrorPath,
		mirrorFile: file,
		onLine:     t.onLine,
		onTimestamp: func(ts time.Time) {
			if ts.IsZero() {
				return
			}
			t.mu.Lock()
			if ts.After(t.cursorSince[container.ID]) {
				// 记录下一次重连的起点，减少容器重建或网络抖动时的日志丢失窗口。
				t.cursorSince[container.ID] = ts.Add(time.Nanosecond)
			}
			t.mu.Unlock()
		},
	}
	if err := collectDockerLogStream(body, collector); err != nil && streamCtx.Err() == nil {
		logger.Warn("Docker 日志流处理中断: container=%s err=%v", container.ID, err)
	}
}

func (t *DockerTailer) removeStream(containerID string, handle *dockerStreamHandle) {
	t.mu.Lock()
	defer t.mu.Unlock()
	current := t.streams[containerID]
	if current == handle {
		delete(t.streams, containerID)
	}
}

func (t *DockerTailer) stopAllStreams() {
	t.mu.Lock()
	defer t.mu.Unlock()
	for id, handle := range t.streams {
		if handle.cancel != nil {
			handle.cancel()
		}
		delete(t.streams, id)
	}
}

func (t *DockerTailer) matchesAnySource(container dockerContainerInfo) bool {
	for _, source := range t.sources {
		if matchDockerSource(source, container) {
			return true
		}
	}
	return false
}

func matchDockerSource(source logSource, container dockerContainerInfo) bool {
	switch source.Kind {
	case logSourceDockerContainer:
		selector := strings.TrimSpace(source.Selector)
		if selector == "" {
			return false
		}
		if strings.HasPrefix(container.ID, selector) {
			return true
		}
		for _, name := range container.Names {
			if strings.TrimPrefix(name, "/") == selector {
				return true
			}
		}
		return false
	case logSourceDockerService:
		service := strings.TrimSpace(container.Labels["com.docker.compose.service"])
		if service == "" || service != source.Selector {
			return false
		}
		if strings.TrimSpace(source.Project) == "" {
			return true
		}
		return strings.TrimSpace(container.Labels["com.docker.compose.project"]) == source.Project
	default:
		return false
	}
}

func dockerMirrorPath(root string, container dockerContainerInfo) string {
	project := strings.TrimSpace(container.Labels["com.docker.compose.project"])
	service := strings.TrimSpace(container.Labels["com.docker.compose.service"])
	name := dockerContainerName(container)
	parts := make([]string, 0, 3)
	if project != "" {
		parts = append(parts, project)
	}
	if service != "" {
		parts = append(parts, service)
	}
	parts = append(parts, name)
	fileName := sanitizeMirrorName(strings.Join(parts, "__")) + ".log"
	return filepath.Join(root, fileName)
}

func dockerContainerName(container dockerContainerInfo) string {
	for _, name := range container.Names {
		trimmed := strings.TrimSpace(strings.TrimPrefix(name, "/"))
		if trimmed != "" {
			return trimmed
		}
	}
	if len(container.ID) >= 12 {
		return container.ID[:12]
	}
	return strings.TrimSpace(container.ID)
}

func sanitizeMirrorName(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "docker-source"
	}
	var builder strings.Builder
	for _, r := range trimmed {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			builder.WriteRune(r)
		default:
			builder.WriteByte('_')
		}
	}
	result := strings.Trim(builder.String(), "._")
	if result == "" {
		return "docker-source"
	}
	return result
}

type dockerLineCollector struct {
	mirrorPath  string
	mirrorFile  *os.File
	remainder   string
	onLine      func(path, line string)
	onTimestamp func(ts time.Time)
}

func (c *dockerLineCollector) WriteChunk(chunk []byte) error {
	if c == nil || len(chunk) == 0 {
		return nil
	}
	content := c.remainder + string(chunk)
	lines := strings.Split(content, "\n")
	if strings.HasSuffix(content, "\n") {
		c.remainder = ""
	} else {
		c.remainder = lines[len(lines)-1]
		lines = lines[:len(lines)-1]
	}
	for _, line := range lines {
		if err := c.emitLine(line); err != nil {
			return err
		}
	}
	return nil
}

func (c *dockerLineCollector) Flush() error {
	if c == nil || strings.TrimSpace(c.remainder) == "" {
		return nil
	}
	line := c.remainder
	c.remainder = ""
	return c.emitLine(line)
}

func (c *dockerLineCollector) emitLine(raw string) error {
	trimmed := strings.TrimRight(raw, "\r")
	if strings.TrimSpace(trimmed) == "" {
		return nil
	}
	if ts, ok := parseDockerLogTimestamp(trimmed); ok && c.onTimestamp != nil {
		c.onTimestamp(ts)
	}
	if c.mirrorFile != nil {
		if _, err := c.mirrorFile.WriteString(trimmed + "\n"); err != nil {
			return err
		}
	}
	if c.onLine != nil {
		c.onLine(c.mirrorPath, trimmed)
	}
	return nil
}

func parseDockerLogTimestamp(line string) (time.Time, bool) {
	firstSpace := strings.IndexByte(line, ' ')
	if firstSpace <= 0 {
		return time.Time{}, false
	}
	ts, err := time.Parse(time.RFC3339Nano, line[:firstSpace])
	if err != nil {
		return time.Time{}, false
	}
	return ts, true
}

func collectDockerLogStream(reader io.Reader, collector *dockerLineCollector) error {
	if collector == nil {
		return nil
	}
	buffered := bufio.NewReader(reader)
	header, err := buffered.Peek(8)
	if err == nil && looksLikeDockerMultiplexHeader(header) {
		if err := collectDockerMultiplexStream(buffered, collector); err != nil {
			return err
		}
		return collector.Flush()
	}
	if err := collectDockerPlainStream(buffered, collector); err != nil {
		return err
	}
	return collector.Flush()
}

func looksLikeDockerMultiplexHeader(header []byte) bool {
	if len(header) < 8 {
		return false
	}
	if header[0] != 1 && header[0] != 2 {
		return false
	}
	if header[1] != 0 || header[2] != 0 || header[3] != 0 {
		return false
	}
	size := binary.BigEndian.Uint32(header[4:8])
	return size <= 16*1024*1024
}

func collectDockerMultiplexStream(reader *bufio.Reader, collector *dockerLineCollector) error {
	header := make([]byte, 8)
	for {
		if _, err := io.ReadFull(reader, header); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return nil
			}
			return err
		}
		size := binary.BigEndian.Uint32(header[4:8])
		if size == 0 {
			continue
		}
		payload := make([]byte, int(size))
		if _, err := io.ReadFull(reader, payload); err != nil {
			return err
		}
		if err := collector.WriteChunk(payload); err != nil {
			return err
		}
	}
}

func collectDockerPlainStream(reader *bufio.Reader, collector *dockerLineCollector) error {
	buf := make([]byte, 32*1024)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			if writeErr := collector.WriteChunk(buf[:n]); writeErr != nil {
				return writeErr
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

func newDockerAPIFromEnv() (*dockerAPI, error) {
	endpoint := strings.TrimSpace(os.Getenv("DOCKER_HOST"))
	if endpoint == "" {
		endpoint = defaultDockerHost
	}
	return newDockerAPI(endpoint)
}

func newDockerAPI(endpoint string) (*dockerAPI, error) {
	trimmed := strings.TrimSpace(endpoint)
	if trimmed == "" {
		trimmed = defaultDockerHost
	}
	if strings.HasPrefix(trimmed, "/") {
		trimmed = "unix://" + trimmed
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("Docker 连接地址无效: %w", err)
	}

	switch parsed.Scheme {
	case "unix":
		socketPath := parsed.Path
		if socketPath == "" && parsed.Host != "" {
			socketPath = "/" + parsed.Host
		}
		if strings.TrimSpace(socketPath) == "" {
			return nil, fmt.Errorf("Docker Unix Socket 路径不能为空")
		}
		transport := &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var dialer net.Dialer
				return dialer.DialContext(ctx, "unix", socketPath)
			},
		}
		return &dockerAPI{
			client:   &http.Client{Transport: transport},
			baseURL:  "http://docker",
			endpoint: trimmed,
		}, nil
	case "tcp":
		parsed.Scheme = "http"
		fallthrough
	case "http", "https":
		baseURL := strings.TrimRight(parsed.String(), "/")
		return &dockerAPI{
			client:   &http.Client{},
			baseURL:  baseURL,
			endpoint: trimmed,
		}, nil
	default:
		return nil, fmt.Errorf("不支持的 Docker 连接协议: %s", parsed.Scheme)
	}
}

func (api *dockerAPI) listRunningContainers(ctx context.Context) ([]dockerContainerInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api.baseURL+"/containers/json?all=0", nil)
	if err != nil {
		return nil, err
	}
	resp, err := api.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("连接 Docker 失败，请确认已挂载 docker.sock 或设置 DOCKER_HOST: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Docker 列表接口失败: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var containers []dockerContainerInfo
	if err := json.NewDecoder(resp.Body).Decode(&containers); err != nil {
		return nil, err
	}
	return containers, nil
}

func (api *dockerAPI) openContainerLogs(ctx context.Context, containerID string, since time.Time) (io.ReadCloser, error) {
	query := url.Values{}
	query.Set("follow", "1")
	query.Set("stdout", "1")
	query.Set("stderr", "1")
	query.Set("timestamps", "1")
	if !since.IsZero() {
		query.Set("since", since.UTC().Format(time.RFC3339Nano))
	}
	endpoint := fmt.Sprintf("%s/containers/%s/logs?%s", api.baseURL, containerID, query.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := api.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		return nil, fmt.Errorf("Docker 日志接口失败: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return resp.Body, nil
}
