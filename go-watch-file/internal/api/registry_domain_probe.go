package api

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	registryDomainProbeTimeout      = 4 * time.Second
	registryDomainProbeMaxRedirects = 5
	registryDomainProbeBodyLimit    = 64 * 1024
)

const (
	registryAccessTypeUnknown                   = "unknown"
	registryAccessTypeFrontendOnly              = "frontend_only"
	registryAccessTypeFrontendBackendIntegrated = "frontend_backend_integrated"
	registryAccessTypeAPIOnly                   = "api_only"
	registryAccessTypeUnreachable               = "unreachable"
)

var (
	errRegistryDomainProbeInvalidTarget = errors.New("invalid registry domain probe target")
	registryHTMLTitlePattern            = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
)

// registryDomainProbeRunner 抽象域名探测执行器 便于 handler 在测试中注入假实现
type registryDomainProbeRunner interface {
	Probe(ctx context.Context, rawTarget string) (registryDomainProbeResult, error)
}

type registryDomainResolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
	LookupCNAME(ctx context.Context, host string) (string, error)
}

type systemRegistryDomainResolver struct{}

func (systemRegistryDomainResolver) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	return net.DefaultResolver.LookupIPAddr(ctx, host)
}

func (systemRegistryDomainResolver) LookupCNAME(ctx context.Context, host string) (string, error) {
	return net.DefaultResolver.LookupCNAME(ctx, host)
}

type registryDomainProbeOptions struct {
	Client         *http.Client
	Resolver       registryDomainResolver
	Now            func() time.Time
	ValidateTarget func(ctx context.Context, target string) error
}

type registryDomainProber struct {
	client         *http.Client
	resolver       registryDomainResolver
	now            func() time.Time
	validateTarget func(ctx context.Context, target string) error
}

type registryDomainProbeResult struct {
	Domain                   string                          `json:"domain"`
	NormalizedTarget         string                          `json:"normalizedTarget"`
	ProbedAt                 string                          `json:"probedAt"`
	Reachable                bool                            `json:"reachable"`
	RecommendedBaseURL       string                          `json:"recommendedBaseUrl"`
	SuggestedAccessType      string                          `json:"suggestedAccessType"`
	SuggestedAccessTypeLabel string                          `json:"suggestedAccessTypeLabel"`
	DNS                      registryDomainDNSProbe          `json:"dns"`
	HTTPRoot                 registryDomainHTTPProbe         `json:"httpRoot"`
	HTTPSRoot                registryDomainHTTPProbe         `json:"httpsRoot"`
	TLS                      *registryDomainTLSProbe         `json:"tls,omitempty"`
	HealthCandidates         []registryDomainHealthCandidate `json:"healthCandidates"`
	PendingItems             []registryDomainPendingItem     `json:"pendingItems"`
}

type registryDomainDNSProbe struct {
	Host             string   `json:"host"`
	CNAME            string   `json:"cname,omitempty"`
	Addresses        []string `json:"addresses"`
	PubliclyRoutable bool     `json:"publiclyRoutable"`
	Error            string   `json:"error,omitempty"`
}

type registryDomainHTTPProbe struct {
	URL           string                      `json:"url"`
	FinalURL      string                      `json:"finalUrl,omitempty"`
	Reachable     bool                        `json:"reachable"`
	StatusCode    int                         `json:"statusCode,omitempty"`
	ContentType   string                      `json:"contentType,omitempty"`
	ContentKind   string                      `json:"contentKind,omitempty"`
	PageKind      string                      `json:"pageKind,omitempty"`
	Title         string                      `json:"title,omitempty"`
	HasAPIHint    bool                        `json:"hasApiHint"`
	Redirected    bool                        `json:"redirected"`
	RedirectChain []registryDomainRedirectHop `json:"redirectChain,omitempty"`
	TLS           *registryDomainTLSProbe     `json:"tls,omitempty"`
	Error         string                      `json:"error,omitempty"`
}

type registryDomainRedirectHop struct {
	URL        string `json:"url"`
	StatusCode int    `json:"statusCode"`
	Location   string `json:"location,omitempty"`
}

type registryDomainTLSProbe struct {
	Status            string   `json:"status"`
	SubjectCommonName string   `json:"subjectCommonName,omitempty"`
	IssuerCommonName  string   `json:"issuerCommonName,omitempty"`
	NotBefore         string   `json:"notBefore"`
	NotAfter          string   `json:"notAfter"`
	DaysRemaining     int      `json:"daysRemaining"`
	ServerNameMatched bool     `json:"serverNameMatched"`
	TimeValid         bool     `json:"timeValid"`
	DNSNames          []string `json:"dnsNames,omitempty"`
}

type registryDomainHealthCandidate struct {
	Path         string `json:"path"`
	URL          string `json:"url"`
	FinalURL     string `json:"finalUrl,omitempty"`
	Reachable    bool   `json:"reachable"`
	StatusCode   int    `json:"statusCode,omitempty"`
	ContentType  string `json:"contentType,omitempty"`
	ContentKind  string `json:"contentKind,omitempty"`
	LikelyHealth bool   `json:"likelyHealth"`
	Error        string `json:"error,omitempty"`
}

type registryDomainPendingItem struct {
	Code     string `json:"code"`
	Title    string `json:"title"`
	Detail   string `json:"detail"`
	Required bool   `json:"required"`
}

func newRegistryDomainProber(opts registryDomainProbeOptions) *registryDomainProber {
	client := opts.Client
	if client == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		// 这里主动跳过 TLS 校验 仅用于读取证书元数据并给出“证书是否可用”的提示
		// 不把该探测结果当成可信身份校验结论 真实接入仍需人工确认和正式告警兜底
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
		client = &http.Client{Transport: transport}
	} else {
		cloned := *client
		client = &cloned
	}
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resolver := opts.Resolver
	if resolver == nil {
		resolver = systemRegistryDomainResolver{}
	}
	nowFn := opts.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	prober := &registryDomainProber{
		client:   client,
		resolver: resolver,
		now:      nowFn,
	}
	if opts.ValidateTarget != nil {
		prober.validateTarget = opts.ValidateTarget
	} else {
		prober.validateTarget = prober.validatePublicTarget
	}
	return prober
}

func (p *registryDomainProber) Probe(ctx context.Context, rawTarget string) (registryDomainProbeResult, error) {
	target, err := normalizeRegistryDomainProbeTarget(rawTarget)
	if err != nil {
		return registryDomainProbeResult{}, err
	}
	if err := p.validateTarget(ctx, target); err != nil {
		return registryDomainProbeResult{}, err
	}

	result := registryDomainProbeResult{
		Domain:           strings.TrimSpace(rawTarget),
		NormalizedTarget: target,
		ProbedAt:         p.now().UTC().Format(time.RFC3339),
	}
	result.DNS = p.probeDNS(ctx, target)
	result.HTTPRoot = p.probeEndpoint(ctx, "http://"+target+"/")
	result.HTTPSRoot = p.probeEndpoint(ctx, "https://"+target+"/")

	preferred := choosePreferredRegistryProbe(result.HTTPRoot, result.HTTPSRoot)
	result.TLS = preferred.TLS
	result.RecommendedBaseURL = resolveRegistryRecommendedBaseURL(target, preferred)
	result.HealthCandidates = p.probeHealthCandidates(ctx, result.RecommendedBaseURL)
	result.Reachable = result.HTTPRoot.Reachable || result.HTTPSRoot.Reachable || hasReachableRegistryHealthCandidate(result.HealthCandidates)
	result.SuggestedAccessType = classifyRegistryAccessType(result.HTTPRoot, result.HTTPSRoot, result.HealthCandidates)
	result.SuggestedAccessTypeLabel = registryAccessTypeLabel(result.SuggestedAccessType)
	result.PendingItems = buildRegistryPendingItems(preferred, result.SuggestedAccessType, result.HealthCandidates)
	return result, nil
}

func (p *registryDomainProber) validatePublicTarget(ctx context.Context, target string) error {
	hostname := registryTargetHostname(target)
	if hostname == "" {
		return fmt.Errorf("%w: 域名不能为空", errRegistryDomainProbeInvalidTarget)
	}
	if net.ParseIP(hostname) != nil {
		return fmt.Errorf("%w: 当前仅支持域名形式的入口，不支持直接输入 IP", errRegistryDomainProbeInvalidTarget)
	}
	lowerHost := strings.ToLower(hostname)
	if lowerHost == "localhost" {
		return fmt.Errorf("%w: 不允许探测 localhost", errRegistryDomainProbeInvalidTarget)
	}
	if !strings.Contains(hostname, ".") {
		return fmt.Errorf("%w: 请输入完整域名，例如 example.com", errRegistryDomainProbeInvalidTarget)
	}

	lookupCtx, cancel := context.WithTimeout(ctx, registryDomainProbeTimeout)
	defer cancel()
	addrs, err := p.resolver.LookupIPAddr(lookupCtx, hostname)
	if err != nil || len(addrs) == 0 {
		return nil
	}
	for _, addr := range addrs {
		if isRegistryPublicIP(addr.IP) {
			return nil
		}
	}
	return fmt.Errorf("%w: 仅允许探测可公网访问的域名", errRegistryDomainProbeInvalidTarget)
}

func normalizeRegistryDomainProbeTarget(rawTarget string) (string, error) {
	trimmed := strings.TrimSpace(rawTarget)
	if trimmed == "" {
		return "", fmt.Errorf("%w: domain is required", errRegistryDomainProbeInvalidTarget)
	}

	candidate := trimmed
	if !strings.Contains(candidate, "://") {
		candidate = "https://" + candidate
	}
	parsed, err := url.Parse(candidate)
	if err != nil {
		return "", fmt.Errorf("%w: domain format is invalid", errRegistryDomainProbeInvalidTarget)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("%w: only http/https input is supported", errRegistryDomainProbeInvalidTarget)
	}
	hostname := strings.TrimSuffix(strings.TrimSpace(parsed.Hostname()), ".")
	if hostname == "" {
		return "", fmt.Errorf("%w: domain format is invalid", errRegistryDomainProbeInvalidTarget)
	}
	if strings.Contains(hostname, "@") {
		return "", fmt.Errorf("%w: domain format is invalid", errRegistryDomainProbeInvalidTarget)
	}
	if port := strings.TrimSpace(parsed.Port()); port != "" {
		return net.JoinHostPort(hostname, port), nil
	}
	return hostname, nil
}

func registryTargetHostname(target string) string {
	parsed, err := url.Parse("https://" + target)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(parsed.Hostname())
}

func (p *registryDomainProber) probeDNS(ctx context.Context, target string) registryDomainDNSProbe {
	host := registryTargetHostname(target)
	result := registryDomainDNSProbe{
		Host:      host,
		Addresses: []string{},
	}
	if host == "" {
		result.Error = "invalid host"
		return result
	}

	lookupCtx, cancel := context.WithTimeout(ctx, registryDomainProbeTimeout)
	defer cancel()
	if cname, err := p.resolver.LookupCNAME(lookupCtx, host); err == nil {
		result.CNAME = strings.TrimSuffix(strings.TrimSpace(cname), ".")
	}

	addrs, err := p.resolver.LookupIPAddr(lookupCtx, host)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	for _, addr := range addrs {
		ipText := strings.TrimSpace(addr.IP.String())
		if ipText == "" {
			continue
		}
		result.Addresses = append(result.Addresses, ipText)
		if isRegistryPublicIP(addr.IP) {
			result.PubliclyRoutable = true
		}
	}
	return result
}

func (p *registryDomainProber) probeEndpoint(ctx context.Context, rawURL string) registryDomainHTTPProbe {
	result := registryDomainHTTPProbe{URL: rawURL}
	nextURL := rawURL

	for hop := 0; hop <= registryDomainProbeMaxRedirects; hop++ {
		reqCtx, cancel := context.WithTimeout(ctx, registryDomainProbeTimeout)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, nextURL, nil)
		if err != nil {
			cancel()
			result.Error = err.Error()
			return result
		}
		req.Header.Set("User-Agent", "GWF-registry-probe/1.0")
		req.Header.Set("Accept", "application/json,text/html,text/plain;q=0.9,*/*;q=0.8")

		resp, err := p.client.Do(req)
		if err != nil {
			cancel()
			result.Error = err.Error()
			result.FinalURL = nextURL
			return result
		}

		hopInfo := registryDomainRedirectHop{
			URL:        nextURL,
			StatusCode: resp.StatusCode,
			Location:   strings.TrimSpace(resp.Header.Get("Location")),
		}
		result.RedirectChain = append(result.RedirectChain, hopInfo)
		result.StatusCode = resp.StatusCode
		result.FinalURL = nextURL

		if isRegistryRedirectStatus(resp.StatusCode) {
			result.Redirected = true
			location := strings.TrimSpace(resp.Header.Get("Location"))
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
			_ = resp.Body.Close()
			cancel()

			if location == "" {
				result.Error = "redirect response missing location"
				return result
			}
			resolvedURL, err := resolveRegistryRedirectURL(nextURL, location)
			if err != nil {
				result.Error = err.Error()
				return result
			}
			if err := p.validateTarget(ctx, resolvedURL.Host); err != nil {
				result.Error = err.Error()
				return result
			}
			nextURL = resolvedURL.String()
			continue
		}

		bodyPreview, readErr := readRegistryResponsePreview(resp.Body)
		_ = resp.Body.Close()
		cancel()

		result.Reachable = true
		result.ContentType = strings.TrimSpace(resp.Header.Get("Content-Type"))
		result.ContentKind = detectRegistryContentKind(result.ContentType, bodyPreview)
		result.PageKind = detectRegistryPageKind(result.ContentKind, result.StatusCode, bodyPreview)
		result.Title = extractRegistryHTMLTitle(bodyPreview)
		result.HasAPIHint = detectRegistryAPIHint(bodyPreview)
		if resp.TLS != nil {
			result.TLS = buildRegistryTLSProbe(resp.TLS, resp.Request.URL.Hostname(), p.now().UTC())
		}
		if readErr != nil {
			result.Error = readErr.Error()
		}
		return result
	}

	result.Error = "redirect exceeded limit"
	return result
}

func resolveRegistryRedirectURL(baseURL, location string) (*url.URL, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	ref, err := url.Parse(location)
	if err != nil {
		return nil, err
	}
	return base.ResolveReference(ref), nil
}

func (p *registryDomainProber) probeHealthCandidates(ctx context.Context, baseURL string) []registryDomainHealthCandidate {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		return []registryDomainHealthCandidate{}
	}
	paths := []string{"/health", "/actuator/health", "/api/health", "/api/actuator/health"}
	results := make([]registryDomainHealthCandidate, 0, len(paths))
	// 当前版本保持串行探测，优先保证失败原因清晰可追踪，避免并发探测把错误混在一起
	for _, path := range paths {
		probe := p.probeEndpoint(ctx, base+path)
		results = append(results, registryDomainHealthCandidate{
			Path:         path,
			URL:          base + path,
			FinalURL:     probe.FinalURL,
			Reachable:    probe.Reachable,
			StatusCode:   probe.StatusCode,
			ContentType:  probe.ContentType,
			ContentKind:  probe.ContentKind,
			LikelyHealth: isLikelyRegistryHealthCandidate(probe),
			Error:        probe.Error,
		})
	}
	return results
}

func choosePreferredRegistryProbe(httpRoot, httpsRoot registryDomainHTTPProbe) registryDomainHTTPProbe {
	if httpRoot.Reachable && strings.HasPrefix(strings.ToLower(httpRoot.FinalURL), "https://") {
		return httpRoot
	}
	if httpsRoot.Reachable {
		return httpsRoot
	}
	if httpRoot.Reachable {
		return httpRoot
	}
	if strings.TrimSpace(httpsRoot.URL) != "" {
		return httpsRoot
	}
	return httpRoot
}

func resolveRegistryRecommendedBaseURL(target string, preferred registryDomainHTTPProbe) string {
	fallback := "https://" + strings.TrimSpace(target)
	candidate := strings.TrimSpace(preferred.FinalURL)
	if candidate == "" {
		candidate = strings.TrimSpace(preferred.URL)
	}
	if candidate == "" {
		return fallback
	}
	parsed, err := url.Parse(candidate)
	if err != nil {
		return fallback
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return fallback
	}
	return parsed.Scheme + "://" + parsed.Host
}

func classifyRegistryAccessType(httpRoot, httpsRoot registryDomainHTTPProbe, healthCandidates []registryDomainHealthCandidate) string {
	preferred := choosePreferredRegistryProbe(httpRoot, httpsRoot)
	hasLikelyHealth := hasLikelyRegistryHealthCandidate(healthCandidates)
	hasHTMLRoot := preferred.Reachable && (preferred.ContentKind == "html" || preferred.PageKind == "spa" || preferred.Title != "")

	switch {
	case hasLikelyHealth && hasHTMLRoot:
		return registryAccessTypeFrontendBackendIntegrated
	case hasLikelyHealth:
		return registryAccessTypeAPIOnly
	case hasHTMLRoot:
		return registryAccessTypeFrontendOnly
	case httpRoot.Reachable || httpsRoot.Reachable:
		return registryAccessTypeUnknown
	default:
		return registryAccessTypeUnreachable
	}
}

func registryAccessTypeLabel(accessType string) string {
	switch accessType {
	case registryAccessTypeFrontendOnly:
		return "纯前端入口"
	case registryAccessTypeFrontendBackendIntegrated:
		return "前后端一体域名"
	case registryAccessTypeAPIOnly:
		return "纯 API 域名"
	case registryAccessTypeUnreachable:
		return "探测失败域名"
	default:
		return "待人工确认"
	}
}

func buildRegistryPendingItems(preferred registryDomainHTTPProbe, accessType string, healthCandidates []registryDomainHealthCandidate) []registryDomainPendingItem {
	items := make([]registryDomainPendingItem, 0, 6)
	appendItem := func(item registryDomainPendingItem) {
		for _, existing := range items {
			if existing.Code == item.Code {
				return
			}
		}
		items = append(items, item)
	}

	appendItem(registryDomainPendingItem{
		Code:     "confirm_environment",
		Title:    "确认环境归属",
		Detail:   "请确认该域名属于 prod、test 还是临时环境，并补充环境用途说明。",
		Required: true,
	})
	appendItem(registryDomainPendingItem{
		Code:     "confirm_owner_contacts",
		Title:    "补充责任人和值班联系人",
		Detail:   "当前探测无法自动识别负责人、值班人和 SOP 链接，需要人工补充。",
		Required: true,
	})

	if accessType == registryAccessTypeFrontendBackendIntegrated || accessType == registryAccessTypeAPIOnly {
		appendItem(registryDomainPendingItem{
			Code:     "confirm_backend_runtime",
			Title:    "确认后端部署方式",
			Detail:   "请补充后端运行方式（docker、jar、systemd 等）以及对应服务名。",
			Required: true,
		})
	}

	if preferred.ContentKind == "html" || preferred.PageKind == "spa" {
		detail := "已识别首页为前端入口，请补充静态目录或构建产物来源。"
		if preferred.PageKind == "spa" {
			detail = "已识别首页更像 SPA，请补充静态目录、构建产物来源和回滚路径。"
		}
		appendItem(registryDomainPendingItem{
			Code:     "confirm_frontend_source",
			Title:    "补充前端产物来源",
			Detail:   detail,
			Required: false,
		})
	}

	if candidate, ok := firstLikelyRegistryHealthCandidate(healthCandidates); ok {
		appendItem(registryDomainPendingItem{
			Code:     "confirm_health_endpoint",
			Title:    "确认正式健康接口",
			Detail:   fmt.Sprintf("已探测到候选健康接口 %s，请确认它是否作为正式健康检查入口。", candidate.Path),
			Required: true,
		})
	} else {
		appendItem(registryDomainPendingItem{
			Code:     "add_health_endpoint",
			Title:    "补充健康检查入口",
			Detail:   "当前未识别到稳定的健康接口，请补充健康检查路径或烟雾检查地址。",
			Required: true,
		})
	}

	if preferred.TLS != nil {
		switch preferred.TLS.Status {
		case "expiring":
			appendItem(registryDomainPendingItem{
				Code:     "confirm_tls_alert",
				Title:    "纳入证书到期告警",
				Detail:   fmt.Sprintf("当前证书剩余 %d 天，请确认是否纳入证书到期告警。", preferred.TLS.DaysRemaining),
				Required: false,
			})
		case "expired":
			appendItem(registryDomainPendingItem{
				Code:     "fix_tls_expired",
				Title:    "处理证书过期问题",
				Detail:   "当前证书已过期，请先确认续签和告警策略。",
				Required: true,
			})
		case "hostname_mismatch":
			appendItem(registryDomainPendingItem{
				Code:     "fix_tls_hostname",
				Title:    "确认域名与证书匹配关系",
				Detail:   "当前证书 SAN 与探测域名不匹配，请确认是否存在 CDN、网关或错误证书。",
				Required: true,
			})
		}
	}

	return items
}

func firstLikelyRegistryHealthCandidate(candidates []registryDomainHealthCandidate) (registryDomainHealthCandidate, bool) {
	for _, candidate := range candidates {
		if candidate.LikelyHealth {
			return candidate, true
		}
	}
	return registryDomainHealthCandidate{}, false
}

func hasLikelyRegistryHealthCandidate(candidates []registryDomainHealthCandidate) bool {
	_, ok := firstLikelyRegistryHealthCandidate(candidates)
	return ok
}

func hasReachableRegistryHealthCandidate(candidates []registryDomainHealthCandidate) bool {
	for _, candidate := range candidates {
		if candidate.Reachable {
			return true
		}
	}
	return false
}

func isLikelyRegistryHealthCandidate(probe registryDomainHTTPProbe) bool {
	if !probe.Reachable {
		return false
	}
	switch {
	case probe.StatusCode >= 200 && probe.StatusCode < 400:
		return true
	case (probe.StatusCode == http.StatusUnauthorized || probe.StatusCode == http.StatusForbidden) && probe.ContentKind == "json":
		return true
	default:
		return false
	}
}

func isRegistryRedirectStatus(status int) bool {
	return status == http.StatusMovedPermanently ||
		status == http.StatusFound ||
		status == http.StatusSeeOther ||
		status == http.StatusTemporaryRedirect ||
		status == http.StatusPermanentRedirect
}

func detectRegistryContentKind(contentType, body string) string {
	lowerType := strings.ToLower(strings.TrimSpace(contentType))
	switch {
	case strings.Contains(lowerType, "application/json") || looksLikeJSON(body):
		return "json"
	case strings.Contains(lowerType, "text/html") || registryHTMLTitlePattern.MatchString(body):
		return "html"
	case strings.Contains(lowerType, "text/plain") || strings.Contains(lowerType, "text/"):
		return "text"
	default:
		return "unknown"
	}
}

func looksLikeJSON(body string) bool {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return false
	}
	return strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")
}

func detectRegistryPageKind(contentKind string, statusCode int, body string) string {
	lowerBody := strings.ToLower(body)
	switch contentKind {
	case "json":
		return "json_api"
	case "html":
		switch {
		case statusCode >= 500 || strings.Contains(lowerBody, "bad gateway") || strings.Contains(lowerBody, "internal server error"):
			return "error_page"
		case strings.Contains(lowerBody, `id="root"`) || strings.Contains(lowerBody, `id='root'`) || strings.Contains(lowerBody, `id="app"`) || strings.Contains(lowerBody, `id='app'`):
			return "spa"
		default:
			return "html_page"
		}
	case "text":
		return "text_page"
	default:
		return "unknown"
	}
}

func detectRegistryAPIHint(body string) bool {
	lowerBody := strings.ToLower(body)
	return strings.Contains(lowerBody, "/api/") ||
		strings.Contains(lowerBody, "swagger") ||
		strings.Contains(lowerBody, "openapi")
}

func extractRegistryHTMLTitle(body string) string {
	match := registryHTMLTitlePattern.FindStringSubmatch(body)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(html.UnescapeString(match[1]))
}

func buildRegistryTLSProbe(state *tls.ConnectionState, hostname string, now time.Time) *registryDomainTLSProbe {
	if state == nil || len(state.PeerCertificates) == 0 {
		return nil
	}
	cert := state.PeerCertificates[0]
	daysRemaining := int(cert.NotAfter.Sub(now).Hours() / 24)
	timeValid := !now.Before(cert.NotBefore) && !now.After(cert.NotAfter)
	serverNameMatched := cert.VerifyHostname(hostname) == nil

	status := "ok"
	switch {
	case !timeValid && now.After(cert.NotAfter):
		status = "expired"
	case !serverNameMatched:
		status = "hostname_mismatch"
	case daysRemaining <= 30:
		status = "expiring"
	}

	return &registryDomainTLSProbe{
		Status:            status,
		SubjectCommonName: cert.Subject.CommonName,
		IssuerCommonName:  cert.Issuer.CommonName,
		NotBefore:         cert.NotBefore.UTC().Format(time.RFC3339),
		NotAfter:          cert.NotAfter.UTC().Format(time.RFC3339),
		DaysRemaining:     daysRemaining,
		ServerNameMatched: serverNameMatched,
		TimeValid:         timeValid,
		DNSNames:          append([]string{}, cert.DNSNames...),
	}
}

func readRegistryResponsePreview(reader io.Reader) (string, error) {
	if reader == nil {
		return "", nil
	}
	data, err := io.ReadAll(io.LimitReader(reader, registryDomainProbeBodyLimit))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func isRegistryPublicIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	return !ip.IsLoopback() &&
		!ip.IsPrivate() &&
		!ip.IsLinkLocalMulticast() &&
		!ip.IsLinkLocalUnicast() &&
		!ip.IsMulticast() &&
		!ip.IsUnspecified()
}

func (h *handler) registryDomainProbe(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	domain := strings.TrimSpace(r.URL.Query().Get("domain"))
	if r.Method == http.MethodPost {
		var req struct {
			Domain string `json:"domain"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
			return
		}
		domain = strings.TrimSpace(req.Domain)
	}
	if domain == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "domain is required"})
		return
	}

	prober := h.domainProber
	if prober == nil {
		prober = newRegistryDomainProber(registryDomainProbeOptions{})
	}
	result, err := prober.Probe(r.Context(), domain)
	if err != nil {
		if errors.Is(err, errRegistryDomainProbeInvalidTarget) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"result": result,
	})
}
