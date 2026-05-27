// 本文件用于知识库问答接口与 RAG 降级流程。
// 文件职责：把检索问答、AI 增强和降级元数据从知识库通用 handler 中拆出。

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/kb"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/metrics"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/models"
)

// kbAsk 是知识问答入口
// 先走检索问答基线 再按配置决定是否叠加 AI 生成 避免强依赖外部模型
func (h *handler) kbAsk(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if h.kb == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "knowledge base is not ready"})
		return
	}
	var req struct {
		Question string `json:"question"`
		Limit    int    `json:"limit"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	result, meta, err := h.askKnowledge(req.Question, req.Limit)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	metrics.Global().ObserveKBAsk(len(result.Citations))
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"answer":     result.Answer,
		"citations":  result.Citations,
		"confidence": result.Confidence,
		"meta":       meta,
	})
}

type kbAskMeta struct {
	Degraded       bool   `json:"degraded"`
	ErrorClass     string `json:"errorClass,omitempty"`
	FallbackReason string `json:"fallbackReason,omitempty"`
}

// askKnowledge 统一封装“检索问答 + 可选 AI 增强”的双阶段流程
// 任何阶段失败都要保留可用的检索结果 保障问答能力可降级
func (h *handler) legacyAskKnowledge(question string, limit int) (kb.AskResult, kbAskMeta, error) {
	trimmedQuestion := strings.TrimSpace(question)
	if trimmedQuestion == "" {
		return kb.AskResult{}, kbAskMeta{}, fmt.Errorf("question is required")
	}
	if limit <= 0 {
		limit = 3
	}
	if h == nil || h.kb == nil {
		return kb.AskResult{}, kbAskMeta{}, fmt.Errorf("knowledge service not ready")
	}
	items := make([]kb.Article, 0, limit)
	for _, candidate := range buildQuestionCandidates(trimmedQuestion) {
		found, err := h.kb.Search(candidate, limit, false)
		if err != nil {
			return kb.AskResult{}, kbAskMeta{}, err
		}
		if len(found) == 0 {
			continue
		}
		items = found
		break
	}
	if len(items) == 0 {
		return kb.AskResult{}, kbAskMeta{}, fmt.Errorf("知识库中未找到可引用条目")
	}

	citations := make([]kb.Citation, 0, len(items))
	for _, item := range items {
		citations = append(citations, kb.Citation{
			ArticleID: item.ID,
			Title:     item.Title,
			Version:   item.CurrentVersion,
		})
	}

	fallback, _ := h.kb.Ask(trimmedQuestion, limit)
	if len(fallback.Citations) == 0 {
		fallback.Citations = citations
	}
	if strings.TrimSpace(fallback.Answer) == "" {
		fallback.Answer = fmt.Sprintf("可先参考《%s》并根据建议动作执行排查。", items[0].Title)
	}

	if !h.isKnowledgeAIReady() {
		fallback.Confidence = 0.72
		return fallback, kbAskMeta{
			Degraded:       true,
			ErrorClass:     "ai_disabled",
			FallbackReason: "ai_disabled_or_unconfigured",
		}, nil
	}

	answer, confidence, err := h.callAIForKnowledgeAnswer(ragPayload{
		Question:  trimmedQuestion,
		Citations: citations,
		Chunks:    []kb.RetrievedChunk{},
	})
	if err != nil {
		// AI 调用失败时降级回本地回答，但仍返回引用
		fallback.Confidence = 0.65
		return fallback, kbAskMeta{
			Degraded:       true,
			ErrorClass:     classifyKnowledgeAIError(err),
			FallbackReason: "ai_request_failed",
		}, nil
	}
	answer = strings.TrimSpace(answer)
	if answer == "" {
		fallback.Confidence = 0.65
		return fallback, kbAskMeta{
			Degraded:       true,
			ErrorClass:     "empty_answer",
			FallbackReason: "ai_response_empty",
		}, nil
	}
	return kb.AskResult{
		Answer:     answer,
		Citations:  citations,
		Confidence: confidence,
	}, kbAskMeta{}, nil
}

func (h *handler) askKnowledge(question string, limit int) (kb.AskResult, kbAskMeta, error) {
	trimmedQuestion := strings.TrimSpace(question)
	if trimmedQuestion == "" {
		return kb.AskResult{}, kbAskMeta{}, fmt.Errorf("question is required")
	}
	if limit <= 0 {
		limit = 3
	}
	if h == nil || h.kb == nil {
		return kb.AskResult{}, kbAskMeta{}, fmt.Errorf("knowledge service not ready")
	}
	chunks := make([]kb.RetrievedChunk, 0, limit)
	for _, candidate := range buildQuestionCandidates(trimmedQuestion) {
		found, err := h.kb.Retrieve(candidate, limit, false)
		if err != nil {
			return kb.AskResult{}, kbAskMeta{}, err
		}
		if len(found) == 0 {
			continue
		}
		chunks = found
		break
	}
	if len(chunks) == 0 {
		return kb.AskResult{}, kbAskMeta{}, fmt.Errorf("知识库中未找到可引用条目")
	}

	citations := make([]kb.Citation, 0, len(chunks))
	for _, item := range chunks {
		citations = append(citations, kb.Citation{
			ArticleID:  item.ArticleID,
			Title:      item.Title,
			Version:    item.Version,
			Heading:    item.Heading,
			Snippet:    item.Snippet,
			ChunkIndex: item.ChunkIndex,
		})
	}

	fallback, _ := h.kb.Ask(trimmedQuestion, limit)
	if len(fallback.Citations) == 0 {
		fallback.Citations = citations
	}
	if strings.TrimSpace(fallback.Answer) == "" {
		fallback.Answer = fmt.Sprintf("可先参考《%s》并根据引用片段继续排查。", chunks[0].Title)
	}

	if !h.isKnowledgeAIReady() {
		fallback.Confidence = 0.72
		return fallback, kbAskMeta{
			Degraded:       true,
			ErrorClass:     "ai_disabled",
			FallbackReason: "ai_disabled_or_unconfigured",
		}, nil
	}

	answer, confidence, err := h.callAIForKnowledgeAnswer(ragPayload{
		Question:  trimmedQuestion,
		Citations: citations,
		Chunks:    chunks,
	})
	if err != nil {
		fallback.Confidence = 0.65
		return fallback, kbAskMeta{
			Degraded:       true,
			ErrorClass:     classifyKnowledgeAIError(err),
			FallbackReason: "ai_request_failed",
		}, nil
	}
	answer = strings.TrimSpace(answer)
	if answer == "" {
		fallback.Confidence = 0.65
		return fallback, kbAskMeta{
			Degraded:       true,
			ErrorClass:     "empty_answer",
			FallbackReason: "ai_response_empty",
		}, nil
	}
	return kb.AskResult{
		Answer:     answer,
		Citations:  citations,
		Confidence: confidence,
	}, kbAskMeta{}, nil
}

func classifyKnowledgeAIError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(msg, "deadline exceeded"), strings.Contains(msg, "timeout"):
		return "timeout"
	case strings.Contains(msg, "429"), strings.Contains(msg, "rate limit"):
		return "rate_limit"
	case strings.Contains(msg, "connection refused"),
		strings.Contains(msg, "no such host"),
		strings.Contains(msg, "dial tcp"),
		strings.Contains(msg, "request failed"):
		return "network"
	case strings.Contains(msg, "status 5"), strings.Contains(msg, "bad gateway"), strings.Contains(msg, "service unavailable"):
		return "upstream"
	default:
		return "request_error"
	}
}

type legacyRAGPayload struct {
	Question  string
	Citations []kb.Citation
	Articles  []kb.Article
}

type ragPayload struct {
	Question  string
	Citations []kb.Citation
	Chunks    []kb.RetrievedChunk
}

func (h *handler) isKnowledgeAIReady() bool {
	cfg := h.resolveKBConfig()
	if cfg == nil {
		return false
	}
	if !cfg.AIEnabled {
		return false
	}
	return strings.TrimSpace(cfg.AIBaseURL) != "" &&
		strings.TrimSpace(cfg.AIAPIKey) != "" &&
		strings.TrimSpace(cfg.AIModel) != ""
}

// callAIForKnowledgeAnswer 只负责调用模型并返回结构化结果
// 业务侧是否采用该结果由上层决策 避免网络波动直接污染问答主流程
func (h *handler) callAIForKnowledgeAnswer(payload ragPayload) (string, float64, error) {
	cfg := h.resolveKBConfig()
	if cfg == nil {
		return "", 0, fmt.Errorf("config not loaded")
	}
	endpoint, err := buildChatCompletionURL(cfg.AIBaseURL)
	if err != nil {
		return "", 0, err
	}

	systemPrompt := `
你是资深运维知识库助手。
你必须只根据提供的知识条目回答，不允许编造。
输出 JSON 对象：
answer: 中文回答，最多 5 句，强调可执行动作
confidence: 0 到 1 的小数
禁止输出 Markdown 和多余字段。
`
	userContent := buildKnowledgeRAGContent(payload)
	requestBody := openAIChatRequest{
		Model: cfg.AIModel,
		Messages: []openAIChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userContent},
		},
		Temperature: 0.2,
	}
	body, err := json.Marshal(requestBody)
	if err != nil {
		return "", 0, fmt.Errorf("marshal request failed: %w", err)
	}

	timeout := parseAITimeout(cfg.AITimeout)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBuffer(body))
	if err != nil {
		return "", 0, fmt.Errorf("build request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.AIAPIKey)

	client := &http.Client{Timeout: timeout + 2*time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, fmt.Errorf("read response failed: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", 0, fmt.Errorf("ai response status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var parsed openAIChatResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", 0, fmt.Errorf("parse response failed: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", 0, fmt.Errorf("empty ai response")
	}
	content := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if content == "" {
		return "", 0, fmt.Errorf("empty ai content")
	}
	return parseKnowledgeAIResponse(content)
}

func (h *handler) resolveKBConfig() *models.Config {
	if h == nil {
		return nil
	}
	if h.fs != nil {
		if runtime := h.fs.Config(); runtime != nil {
			return runtime
		}
	}
	return h.cfg
}

func legacyBuildKnowledgeRAGContent(payload legacyRAGPayload) string {
	builder := strings.Builder{}
	builder.WriteString("问题:\n")
	builder.WriteString(payload.Question)
	builder.WriteString("\n\n候选知识条目:\n")
	for idx, article := range payload.Articles {
		builder.WriteString(fmt.Sprintf("[%d] %s (id=%s, version=%d, severity=%s)\n",
			idx+1, article.Title, article.ID, article.CurrentVersion, article.Severity))
		if summary := strings.TrimSpace(article.Summary); summary != "" {
			builder.WriteString("摘要: ")
			builder.WriteString(summary)
			builder.WriteString("\n")
		}
		content := strings.TrimSpace(article.Content)
		if content != "" {
			content = trimRunes(content, 800)
			builder.WriteString("正文片段: ")
			builder.WriteString(content)
			builder.WriteString("\n")
		}
		if len(article.Tags) > 0 {
			builder.WriteString("标签: ")
			builder.WriteString(strings.Join(article.Tags, ", "))
			builder.WriteString("\n")
		}
		builder.WriteString("\n")
	}
	builder.WriteString("请基于以上条目给出答案。\n")
	builder.WriteString("注意：最终引用列表由系统追加，你只输出 answer/confidence。")
	return builder.String()
}

// parseKnowledgeAIResponse 对模型输出做强约束解析
// 解析失败直接上抛 由上层触发降级路径 保证返回字段稳定
func buildKnowledgeRAGContent(payload ragPayload) string {
	builder := strings.Builder{}
	builder.WriteString("问题:\n")
	builder.WriteString(payload.Question)
	builder.WriteString("\n\n候选知识片段:\n")
	for idx, chunk := range payload.Chunks {
		builder.WriteString(fmt.Sprintf("[%d] %s (id=%s, version=%d, severity=%s, chunk=%d)\n",
			idx+1, chunk.Title, chunk.ArticleID, chunk.Version, chunk.Severity, chunk.ChunkIndex))
		if heading := strings.TrimSpace(chunk.Heading); heading != "" {
			builder.WriteString("章节: ")
			builder.WriteString(heading)
			builder.WriteString("\n")
		}
		if snippet := strings.TrimSpace(chunk.Snippet); snippet != "" {
			builder.WriteString("命中摘要: ")
			builder.WriteString(snippet)
			builder.WriteString("\n")
		}
		if content := strings.TrimSpace(chunk.Content); content != "" {
			builder.WriteString("正文片段: ")
			builder.WriteString(trimRunes(content, 800))
			builder.WriteString("\n")
		}
		builder.WriteString("\n")
	}
	builder.WriteString("请只基于以上知识片段回答，并给出可执行建议。\n")
	builder.WriteString("注意：最终引用列表由系统追加，你只输出 answer/confidence。")
	return builder.String()
}

func parseKnowledgeAIResponse(raw string) (string, float64, error) {
	clean := strings.TrimSpace(raw)
	if clean == "" {
		return "", 0, fmt.Errorf("empty ai response")
	}
	var payload struct {
		Answer     string   `json:"answer"`
		Confidence *float64 `json:"confidence,omitempty"`
	}
	if err := json.Unmarshal([]byte(clean), &payload); err != nil {
		extracted := extractJSONObject(clean)
		if extracted == "" {
			return "", 0, err
		}
		if err := json.Unmarshal([]byte(extracted), &payload); err != nil {
			return "", 0, err
		}
	}
	answer := strings.TrimSpace(payload.Answer)
	if answer == "" {
		return "", 0, fmt.Errorf("ai answer is empty")
	}
	confidence := 0.78
	if payload.Confidence != nil {
		if *payload.Confidence >= 0 && *payload.Confidence <= 1 {
			confidence = *payload.Confidence
		}
	}
	return answer, confidence, nil
}

func trimRunes(input string, max int) string {
	if max <= 0 {
		return strings.TrimSpace(input)
	}
	r := []rune(strings.TrimSpace(input))
	if len(r) <= max {
		return string(r)
	}
	return string(r[:max]) + "..."
}

func buildQuestionCandidates(question string) []string {
	clean := strings.TrimSpace(question)
	if clean == "" {
		return nil
	}
	candidates := []string{clean}
	normalized := strings.NewReplacer(
		"？", " ",
		"?", " ",
		"，", " ",
		",", " ",
		"。", " ",
		".", " ",
		"！", " ",
		"!", " ",
		"；", " ",
		";", " ",
	).Replace(clean)
	for _, token := range strings.Fields(normalized) {
		if len([]rune(token)) >= 2 {
			candidates = append(candidates, token)
		}
	}
	runes := []rune(clean)
	for _, n := range []int{8, 6, 4} {
		if len(runes) >= n {
			candidates = append(candidates, string(runes[:n]))
		}
	}
	out := make([]string, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		c := strings.TrimSpace(candidate)
		if c == "" {
			continue
		}
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		out = append(out, c)
	}
	return out
}
