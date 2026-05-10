// 本文件用于知识库服务实现 将条目生命周期和检索能力集中在服务层管理

// 文件职责：实现当前模块的核心业务逻辑与数据流转
// 关键路径：入口参数先校验再执行业务处理 最后返回统一结果
// 边界与容错：异常场景显式返回错误 由上层决定重试或降级

package kb

import (
	"crypto/rand"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	_ "modernc.org/sqlite"
)

const (
	defaultDataDir      = "data/kb"
	defaultPageSize     = 20
	maxPageSize         = 100
	defaultReviewDays   = 90
	defaultChunkSize    = 420
	defaultChunkOverlap = 80
)

type Service struct {
	db           *sql.DB
	dbPath       string
	reviewDays   int
	chunkSize    int
	chunkOverlap int
}

type chunkSearchHit struct {
	article    Article
	heading    string
	chunkIndex int
	content    string
	snippet    string
	score      int
}

type articleChunk struct {
	index   int
	heading string
	content string
	hash    string
}

// NewService 统一负责知识库存储初始化
// 这里把目录创建 打开数据库 设置 WAL 和迁移收敛在一个入口
// 这样可以确保调用方拿到 Service 时已经处于可读写状态
func NewService(dataDir string) (*Service, error) {
	root := strings.TrimSpace(dataDir)
	if root == "" {
		if env := strings.TrimSpace(os.Getenv("KB_DATA_DIR")); env != "" {
			root = env
		}
	}
	if root == "" {
		root = defaultDataDir
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("create kb data dir failed: %w", err)
	}
	dbPath := filepath.Join(root, "knowledge.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open kb sqlite failed: %w", err)
	}
	if _, err := db.Exec(`PRAGMA journal_mode=WAL;`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set kb sqlite wal failed: %w", err)
	}
	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	service := &Service{
		db:           db,
		dbPath:       dbPath,
		reviewDays:   resolveReviewDays(),
		chunkSize:    resolveChunkSize(),
		chunkOverlap: resolveChunkOverlap(),
	}
	if err := service.syncCurrentVersionChunks(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return service, nil
}

func (s *Service) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Service) DBPath() string {
	if s == nil {
		return ""
	}
	return s.dbPath
}

// CreateArticle 是知识库写入主路径
// 文章主表 版本表 标签 参考来源和评审记录必须在同一事务内提交
// 任一子步骤失败都会触发回滚 避免出现半成功数据
func (s *Service) CreateArticle(input CreateArticleInput) (*Article, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("kb service not ready")
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return nil, fmt.Errorf("%w: title is required", ErrInvalidInput)
	}
	createdBy := normalizeOperator(input.CreatedBy)
	createdAt := nowRFC3339()
	id := newID("kb")
	severity := normalizeSeverity(input.Severity)
	category := strings.TrimSpace(input.Category)
	if category == "" {
		category = "general"
	}
	summary := strings.TrimSpace(input.Summary)
	content := strings.TrimSpace(input.Content)
	changeNote := strings.TrimSpace(input.ChangeNote)
	if changeNote == "" {
		changeNote = "initial version"
	}
	sourceType := normalizeSourceType(input.SourceType)
	sourceRef := strings.TrimSpace(input.SourceRef)
	refTitle := strings.TrimSpace(input.RefTitle)
	if refTitle == "" {
		refTitle = title
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer rollbackTx(tx)

	_, err = tx.Exec(`
		INSERT INTO kb_articles (
			id, title, summary, category, severity, status, current_version,
			created_by, updated_by, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id, title, summary, category, severity, StatusDraft, 1, createdBy, createdBy, createdAt, createdAt)
	if err != nil {
		return nil, fmt.Errorf("create kb article failed: %w", err)
	}

	_, err = tx.Exec(`
		INSERT INTO kb_article_versions (
			article_id, version, content_markdown, change_note, source_type, source_ref, created_by, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, id, 1, content, changeNote, sourceType, sourceRef, createdBy, createdAt)
	if err != nil {
		return nil, fmt.Errorf("create kb article version failed: %w", err)
	}
	if err := s.rebuildChunksTx(tx, id, 1, content, createdAt); err != nil {
		return nil, err
	}

	if err := replaceTagsTx(tx, id, input.Tags); err != nil {
		return nil, err
	}
	if sourceRef != "" {
		if err := upsertReferenceTx(tx, id, sourceType, sourceRef, refTitle); err != nil {
			return nil, err
		}
	}
	if err := insertReviewTx(tx, id, 1, "create", changeNote, createdBy, createdAt); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetArticle(id)
}

// UpdateArticle 采用追加版本而不是覆盖旧版本
// 这样可以保留完整变更历史 也为后续 rollback 提供稳定依据
func (s *Service) UpdateArticle(id string, input UpdateArticleInput) (*Article, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("kb service not ready")
	}
	articleID := strings.TrimSpace(id)
	if articleID == "" {
		return nil, fmt.Errorf("%w: article id is required", ErrInvalidInput)
	}
	updatedBy := normalizeOperator(input.UpdatedBy)
	updatedAt := nowRFC3339()

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer rollbackTx(tx)

	current, err := queryArticleCoreTx(tx, articleID)
	if err != nil {
		return nil, err
	}

	title := firstNonEmpty(strings.TrimSpace(input.Title), current.Title)
	if title == "" {
		return nil, fmt.Errorf("%w: title is required", ErrInvalidInput)
	}
	summary := firstNonEmpty(strings.TrimSpace(input.Summary), current.Summary)
	category := firstNonEmpty(strings.TrimSpace(input.Category), current.Category)
	if category == "" {
		category = "general"
	}
	severity := normalizeSeverity(firstNonEmpty(strings.TrimSpace(input.Severity), current.Severity))
	content := strings.TrimSpace(input.Content)
	if content == "" {
		content = current.Content
	}
	nextVersion := current.CurrentVersion + 1
	changeNote := strings.TrimSpace(input.ChangeNote)
	if changeNote == "" {
		changeNote = "update content"
	}
	sourceType := normalizeSourceType(input.SourceType)
	sourceRef := strings.TrimSpace(input.SourceRef)
	refTitle := strings.TrimSpace(input.RefTitle)
	if refTitle == "" {
		refTitle = title
	}

	_, err = tx.Exec(`
		UPDATE kb_articles
		SET title = ?, summary = ?, category = ?, severity = ?, current_version = ?, updated_by = ?, updated_at = ?
		WHERE id = ?
	`, title, summary, category, severity, nextVersion, updatedBy, updatedAt, articleID)
	if err != nil {
		return nil, fmt.Errorf("update kb article failed: %w", err)
	}

	_, err = tx.Exec(`
		INSERT INTO kb_article_versions (
			article_id, version, content_markdown, change_note, source_type, source_ref, created_by, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, articleID, nextVersion, content, changeNote, sourceType, sourceRef, updatedBy, updatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert kb version failed: %w", err)
	}
	if err := s.rebuildChunksTx(tx, articleID, nextVersion, content, updatedAt); err != nil {
		return nil, err
	}

	if len(input.Tags) > 0 {
		if err := replaceTagsTx(tx, articleID, input.Tags); err != nil {
			return nil, err
		}
	}
	if sourceRef != "" {
		if err := upsertReferenceTx(tx, articleID, sourceType, sourceRef, refTitle); err != nil {
			return nil, err
		}
	}
	if err := insertReviewTx(tx, articleID, nextVersion, "update", changeNote, updatedBy, updatedAt); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetArticle(articleID)
}

func (s *Service) GetArticle(id string) (*Article, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("kb service not ready")
	}
	articleID := strings.TrimSpace(id)
	if articleID == "" {
		return nil, fmt.Errorf("%w: article id is required", ErrInvalidInput)
	}
	core, err := queryArticleCore(s.db, articleID)
	if err != nil {
		return nil, err
	}
	tags, err := queryArticleTags(s.db, articleID)
	if err != nil {
		return nil, err
	}
	refs, err := queryArticleRefs(s.db, articleID)
	if err != nil {
		return nil, err
	}
	versions, err := queryArticleVersions(s.db, articleID)
	if err != nil {
		return nil, err
	}
	reviews, err := queryArticleReviews(s.db, articleID)
	if err != nil {
		return nil, err
	}
	core.Tags = tags
	core.References = refs
	core.Versions = versions
	core.Reviews = reviews
	return core, nil
}

func (s *Service) ListArticles(query ListQuery) ([]Article, int, error) {
	if s == nil || s.db == nil {
		return nil, 0, fmt.Errorf("kb service not ready")
	}
	page := query.Page
	if page <= 0 {
		page = 1
	}
	pageSize := query.PageSize
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	offset := (page - 1) * pageSize

	whereSQL, args := buildListWhere(query)
	countSQL := "SELECT COUNT(1) FROM kb_articles a" + whereSQL
	var total int
	if err := s.db.QueryRow(countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, pageSize, offset)

	rows, err := s.db.Query(`
		SELECT
			a.id,
			a.title,
			a.summary,
			a.category,
			a.severity,
			a.status,
			a.current_version,
			a.created_by,
			a.updated_by,
			a.created_at,
			a.updated_at,
			IFNULL(v.content_markdown, ''),
			IFNULL(v.change_note, '')
		FROM kb_articles a
		LEFT JOIN kb_article_versions v
			ON v.article_id = a.id AND v.version = a.current_version
	`+whereSQL+`
		ORDER BY a.updated_at DESC
		LIMIT ? OFFSET ?
	`, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]Article, 0, pageSize)
	for rows.Next() {
		var item Article
		if err := rows.Scan(
			&item.ID,
			&item.Title,
			&item.Summary,
			&item.Category,
			&item.Severity,
			&item.Status,
			&item.CurrentVersion,
			&item.CreatedBy,
			&item.UpdatedBy,
			&item.CreatedAt,
			&item.UpdatedAt,
			&item.Content,
			&item.ChangeNote,
		); err != nil {
			return nil, 0, err
		}
		item.Status = normalizeArticleStatusOrDraft(item.Status)
		item.NeedsReview = s.isNeedsReview(item.Status, item.UpdatedAt)
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	if len(out) == 0 {
		return out, total, nil
	}
	tagsByArticle, err := queryTagsByArticleIDs(s.db, collectArticleIDs(out))
	if err != nil {
		return nil, 0, err
	}
	for i := range out {
		out[i].Tags = tagsByArticle[out[i].ID]
	}
	return out, total, nil
}

func (s *Service) PendingReviews(limit int) ([]Article, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > maxPageSize {
		limit = maxPageSize
	}
	reviewing, _, err := s.ListArticles(ListQuery{
		Status:   StatusReviewing,
		Page:     1,
		PageSize: limit,
	})
	if err != nil {
		return nil, err
	}
	if len(reviewing) >= limit {
		return reviewing[:limit], nil
	}
	remain := limit - len(reviewing)
	published, _, err := s.ListArticles(ListQuery{
		Status:   StatusPublished,
		Page:     1,
		PageSize: limit * 2,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Article, 0, limit)
	out = append(out, reviewing...)
	for _, item := range published {
		if !item.NeedsReview {
			continue
		}
		out = append(out, item)
		if len(out) >= limit || len(out)-len(reviewing) >= remain {
			break
		}
	}
	return out, nil
}

// ApplyAction 统一处理 submit approve reject archive 等状态迁移
// 这里显式限制可执行动作并记录评审轨迹 保证状态机可审计可回溯
func (s *Service) ApplyAction(id, action, operator, comment string) (*Article, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("kb service not ready")
	}
	articleID := strings.TrimSpace(id)
	if articleID == "" {
		return nil, fmt.Errorf("%w: article id is required", ErrInvalidInput)
	}
	normalizedAction := strings.ToLower(strings.TrimSpace(action))
	switch normalizedAction {
	case "submit", "approve", "reject", "archive":
	default:
		return nil, fmt.Errorf("%w: unsupported action %s", ErrInvalidInput, normalizedAction)
	}
	updatedAt := nowRFC3339()
	operator = normalizeOperator(operator)
	comment = strings.TrimSpace(comment)

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer rollbackTx(tx)

	current, err := queryArticleCoreTx(tx, articleID)
	if err != nil {
		return nil, err
	}
	currentStatus := normalizeArticleStatusOrDraft(current.Status)
	if !isKBActionAllowed(currentStatus, normalizedAction) {
		return nil, fmt.Errorf("%w: action %s is not allowed when status is %s", ErrInvalidInput, normalizedAction, currentStatus)
	}
	nextStatus := currentStatus
	switch normalizedAction {
	case "submit":
		nextStatus = StatusReviewing
	case "approve":
		nextStatus = StatusPublished
	case "reject":
		nextStatus = StatusDraft
	case "archive":
		nextStatus = StatusArchived
	}
	if nextStatus != currentStatus {
		if _, err := tx.Exec(`
			UPDATE kb_articles
			SET status = ?, updated_by = ?, updated_at = ?
			WHERE id = ?
		`, nextStatus, operator, updatedAt, articleID); err != nil {
			return nil, err
		}
	}
	if err := insertReviewTx(tx, articleID, current.CurrentVersion, normalizedAction, comment, operator, updatedAt); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetArticle(articleID)
}

// RollbackArticle 通过“生成新版本”完成回滚
// 不直接改写历史版本内容 这样能同时满足可恢复和可审计
func (s *Service) RollbackArticle(id string, targetVersion int, operator, comment string) (*Article, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("kb service not ready")
	}
	articleID := strings.TrimSpace(id)
	if articleID == "" {
		return nil, fmt.Errorf("%w: article id is required", ErrInvalidInput)
	}
	if targetVersion <= 0 {
		return nil, fmt.Errorf("%w: targetVersion must be greater than zero", ErrInvalidInput)
	}
	operator = normalizeOperator(operator)
	updatedAt := nowRFC3339()
	comment = strings.TrimSpace(comment)
	if comment == "" {
		comment = fmt.Sprintf("rollback to version %d", targetVersion)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer rollbackTx(tx)

	current, err := queryArticleCoreTx(tx, articleID)
	if err != nil {
		return nil, err
	}
	var rollbackContent string
	if err := tx.QueryRow(`
		SELECT content_markdown
		FROM kb_article_versions
		WHERE article_id = ? AND version = ?
	`, articleID, targetVersion).Scan(&rollbackContent); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: target version %d not found", ErrInvalidInput, targetVersion)
		}
		return nil, err
	}
	nextVersion := current.CurrentVersion + 1
	if _, err := tx.Exec(`
		INSERT INTO kb_article_versions (
			article_id, version, content_markdown, change_note, source_type, source_ref, created_by, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, articleID, nextVersion, rollbackContent, comment, "rollback", fmt.Sprintf("version:%d", targetVersion), operator, updatedAt); err != nil {
		return nil, err
	}
	if err := s.rebuildChunksTx(tx, articleID, nextVersion, rollbackContent, updatedAt); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`
		UPDATE kb_articles
		SET current_version = ?, updated_by = ?, updated_at = ?
		WHERE id = ?
	`, nextVersion, operator, updatedAt, articleID); err != nil {
		return nil, err
	}
	if err := insertReviewTx(tx, articleID, nextVersion, "rollback", comment, operator, updatedAt); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetArticle(articleID)
}

// Search 先走数据库层条件查询 再在结果不足时补充分词兜底
// 这个设计用于提升真实文本噪声场景下的召回稳定性
func (s *Service) legacySearch(query string, limit int, includeArchived bool) ([]Article, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return []Article{}, nil
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > maxPageSize {
		limit = maxPageSize
	}
	status := StatusPublished
	if includeArchived {
		status = ""
	}
	items, _, err := s.ListArticles(ListQuery{
		Query:    q,
		Status:   status,
		Page:     1,
		PageSize: limit,
	})
	if err != nil {
		return nil, err
	}
	if len(items) > 0 {
		return items, nil
	}
	// Fallback: when full sentence match misses, use token scoring for better recall.
	return s.searchByTokens(q, limit, includeArchived)
}

// Ask 在检索结果基础上组装引用上下文
// 即使 AI 未参与生成也会返回可追踪的引用条目 保障答案可验证
func (s *Service) legacyAsk(question string, limit int) (AskResult, error) {
	trimmed := strings.TrimSpace(question)
	if trimmed == "" {
		return AskResult{}, fmt.Errorf("%w: question is required", ErrInvalidInput)
	}
	if limit <= 0 {
		limit = 3
	}
	items, err := s.Search(trimmed, limit, false)
	if err != nil {
		return AskResult{}, err
	}
	if len(items) == 0 {
		return AskResult{
			Answer:     "知识库中未检索到相关条目，请补充更具体的关键词。",
			Citations:  []Citation{},
			Confidence: 0.2,
		}, nil
	}
	citations := make([]Citation, 0, len(items))
	for _, item := range items {
		citations = append(citations, Citation{
			ArticleID: item.ID,
			Title:     item.Title,
			Version:   item.CurrentVersion,
		})
	}
	top := items[0]
	snippet := strings.TrimSpace(top.Summary)
	if snippet == "" {
		snippet = snippetFromContent(top.Content, 180)
	}
	if snippet == "" {
		snippet = "建议先查看该条目的处置步骤与历史变更。"
	}
	answer := fmt.Sprintf("基于知识库条目《%s》：%s", top.Title, snippet)
	return AskResult{
		Answer:     answer,
		Citations:  citations,
		Confidence: 0.75,
	}, nil
}

func (s *Service) legacyRecommendations(query string, limit int) ([]Article, error) {
	return s.legacySearch(query, limit, false)
}

// searchByTokens 是 Search 的召回兜底
// 它不依赖全文索引 在分词后按简单打分排序 保证低依赖场景也能工作
func (s *Service) searchByTokens(query string, limit int, includeArchived bool) ([]Article, error) {
	if s == nil {
		return []Article{}, nil
	}
	tokens := tokenizeSearchQuery(query)
	if len(tokens) == 0 {
		return []Article{}, nil
	}
	status := StatusPublished
	if includeArchived {
		status = ""
	}
	candidates := make([]Article, 0, maxPageSize)
	page := 1
	for {
		items, total, err := s.ListArticles(ListQuery{
			Status:          status,
			IncludeArchived: includeArchived,
			Page:            page,
			PageSize:        maxPageSize,
		})
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, items...)
		if len(items) == 0 || len(candidates) >= total || page >= 10 {
			break
		}
		page++
	}
	type scoredArticle struct {
		article Article
		score   int
	}
	scored := make([]scoredArticle, 0, len(candidates))
	for _, item := range candidates {
		score := scoreArticleByTokens(item, tokens)
		if score <= 0 {
			continue
		}
		scored = append(scored, scoredArticle{article: item, score: score})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})
	if len(scored) == 0 {
		return []Article{}, nil
	}
	if limit > len(scored) {
		limit = len(scored)
	}
	out := make([]Article, 0, limit)
	for _, item := range scored[:limit] {
		out = append(out, item.article)
	}
	return out, nil
}

// Search 先做片段级召回，再把最佳片段折叠回文章结果。
// 这样既能兼容现有文章列表接口，又能让检索真正走 RAG 的 chunk 底座。
func (s *Service) Search(query string, limit int, includeArchived bool) ([]Article, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return []Article{}, nil
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > maxPageSize {
		limit = maxPageSize
	}
	hits, err := s.retrieveChunkHits(q, limit*4, includeArchived)
	if err != nil {
		return nil, err
	}
	if len(hits) == 0 {
		return s.searchByTokens(q, limit, includeArchived)
	}
	out := make([]Article, 0, limit)
	seen := make(map[string]struct{}, limit)
	for _, hit := range hits {
		if _, ok := seen[hit.article.ID]; ok {
			continue
		}
		item := hit.article
		item.Content = hit.content
		item.MatchSnippet = hit.snippet
		item.MatchHeading = hit.heading
		item.MatchScore = hit.score
		out = append(out, item)
		seen[item.ID] = struct{}{}
		if len(out) >= limit {
			break
		}
	}
	if len(out) == 0 {
		return s.searchByTokens(q, limit, includeArchived)
	}
	tagsByArticle, err := queryTagsByArticleIDs(s.db, collectArticleIDs(out))
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Tags = tagsByArticle[out[i].ID]
	}
	return out, nil
}

// Retrieve 返回问答阶段直接可用的片段命中结果。
func (s *Service) Retrieve(query string, limit int, includeArchived bool) ([]RetrievedChunk, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return []RetrievedChunk{}, nil
	}
	if limit <= 0 {
		limit = 3
	}
	if limit > maxPageSize {
		limit = maxPageSize
	}
	hits, err := s.retrieveChunkHits(q, limit, includeArchived)
	if err != nil {
		return nil, err
	}
	out := make([]RetrievedChunk, 0, len(hits))
	for _, hit := range hits {
		out = append(out, RetrievedChunk{
			ArticleID:  hit.article.ID,
			Title:      hit.article.Title,
			Version:    hit.article.CurrentVersion,
			Severity:   hit.article.Severity,
			Heading:    hit.heading,
			Snippet:    hit.snippet,
			Content:    hit.content,
			ChunkIndex: hit.chunkIndex,
			Score:      hit.score,
		})
	}
	return out, nil
}

// Ask 基于片段级召回生成本地回答与引用。
// 即使外部 AI 不可用，也能返回“基于哪一段知识得出的建议”。
func (s *Service) Ask(question string, limit int) (AskResult, error) {
	trimmed := strings.TrimSpace(question)
	if trimmed == "" {
		return AskResult{}, fmt.Errorf("%w: question is required", ErrInvalidInput)
	}
	if limit <= 0 {
		limit = 3
	}
	hits, err := s.retrieveChunkHits(trimmed, limit, false)
	if err != nil {
		return AskResult{}, err
	}
	if len(hits) == 0 {
		return AskResult{
			Answer:     "知识库中未检索到相关条目，请补充更具体的关键词。",
			Citations:  []Citation{},
			Confidence: 0.2,
		}, nil
	}
	citations := make([]Citation, 0, len(hits))
	for _, hit := range hits {
		citations = append(citations, Citation{
			ArticleID:  hit.article.ID,
			Title:      hit.article.Title,
			Version:    hit.article.CurrentVersion,
			Heading:    hit.heading,
			Snippet:    hit.snippet,
			ChunkIndex: hit.chunkIndex,
		})
	}
	confidence := 0.72
	if len(hits) > 1 {
		confidence = 0.78
	}
	return AskResult{
		Answer:     buildFallbackChunkAnswer(hits[0]),
		Citations:  citations,
		Confidence: confidence,
	}, nil
}

func (s *Service) Recommendations(query string, limit int) ([]Article, error) {
	return s.Search(query, limit, false)
}

func (s *Service) retrieveChunkHits(query string, limit int, includeArchived bool) ([]chunkSearchHit, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("kb service not ready")
	}
	normalized := strings.ToLower(strings.TrimSpace(query))
	if normalized == "" {
		return []chunkSearchHit{}, nil
	}
	if limit <= 0 {
		limit = 3
	}
	if limit > maxPageSize {
		limit = maxPageSize
	}
	candidates, err := s.loadChunkCandidates(includeArchived)
	if err != nil {
		return nil, err
	}
	tokens := tokenizeSearchQuery(normalized)
	scored := make([]chunkSearchHit, 0, len(candidates))
	for _, item := range candidates {
		item.score = scoreChunkHit(item, normalized, tokens)
		if item.score <= 0 {
			continue
		}
		item.snippet = snippetFromContent(item.content, 180)
		if item.snippet == "" {
			item.snippet = firstNonEmpty(strings.TrimSpace(item.article.Summary), strings.TrimSpace(item.content))
		}
		scored = append(scored, item)
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		if scored[i].article.UpdatedAt != scored[j].article.UpdatedAt {
			return scored[i].article.UpdatedAt > scored[j].article.UpdatedAt
		}
		if scored[i].article.ID != scored[j].article.ID {
			return scored[i].article.ID < scored[j].article.ID
		}
		return scored[i].chunkIndex < scored[j].chunkIndex
	})
	if len(scored) == 0 {
		return []chunkSearchHit{}, nil
	}
	if limit > len(scored) {
		limit = len(scored)
	}
	return scored[:limit], nil
}

func (s *Service) loadChunkCandidates(includeArchived bool) ([]chunkSearchHit, error) {
	whereParts := []string{"a.status = ?"}
	args := []any{StatusPublished}
	if includeArchived {
		whereParts = []string{}
		args = []any{}
	}
	whereSQL := ""
	if len(whereParts) > 0 {
		whereSQL = "WHERE " + strings.Join(whereParts, " AND ")
	}
	rows, err := s.db.Query(`
		SELECT
			a.id,
			a.title,
			a.summary,
			a.category,
			a.severity,
			a.status,
			a.current_version,
			a.created_by,
			a.updated_by,
			a.created_at,
			a.updated_at,
			IFNULL(c.heading, ''),
			IFNULL(c.content, ''),
			c.chunk_index
		FROM kb_articles a
		JOIN kb_article_chunks c
			ON c.article_id = a.id AND c.version = a.current_version
		`+whereSQL+`
		ORDER BY a.updated_at DESC, c.chunk_index ASC
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]chunkSearchHit, 0, 64)
	for rows.Next() {
		var item chunkSearchHit
		if err := rows.Scan(
			&item.article.ID,
			&item.article.Title,
			&item.article.Summary,
			&item.article.Category,
			&item.article.Severity,
			&item.article.Status,
			&item.article.CurrentVersion,
			&item.article.CreatedBy,
			&item.article.UpdatedBy,
			&item.article.CreatedAt,
			&item.article.UpdatedAt,
			&item.heading,
			&item.content,
			&item.chunkIndex,
		); err != nil {
			return nil, err
		}
		item.article.Status = normalizeArticleStatusOrDraft(item.article.Status)
		item.article.NeedsReview = s.isNeedsReview(item.article.Status, item.article.UpdatedAt)
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func scoreChunkHit(hit chunkSearchHit, normalized string, tokens []string) int {
	title := strings.ToLower(strings.TrimSpace(hit.article.Title))
	summary := strings.ToLower(strings.TrimSpace(hit.article.Summary))
	heading := strings.ToLower(strings.TrimSpace(hit.heading))
	content := strings.ToLower(strings.TrimSpace(hit.content))
	score := 0
	if normalized != "" {
		if strings.Contains(title, normalized) {
			score += 36
		}
		if strings.Contains(summary, normalized) {
			score += 20
		}
		if strings.Contains(heading, normalized) {
			score += 24
		}
		if strings.Contains(content, normalized) {
			score += 12
		}
	}
	hitCount := 0
	for _, token := range tokens {
		matched := false
		if token != "" && strings.Contains(title, token) {
			score += 10
			matched = true
		}
		if token != "" && strings.Contains(heading, token) {
			score += 8
			matched = true
		}
		if token != "" && strings.Contains(summary, token) {
			score += 6
			matched = true
		}
		if token != "" && strings.Contains(content, token) {
			score += 4
			matched = true
		}
		if matched {
			hitCount++
		}
	}
	return score + hitCount
}

func buildFallbackChunkAnswer(hit chunkSearchHit) string {
	snippet := strings.TrimSpace(hit.snippet)
	if snippet == "" {
		snippet = snippetFromContent(hit.content, 180)
	}
	if snippet == "" {
		snippet = "建议先查看该条目的适用条件、处置步骤与最近一次变更说明。"
	}
	if strings.TrimSpace(hit.heading) != "" {
		return fmt.Sprintf("基于知识库条目《%s》中的“%s”片段：%s", hit.article.Title, hit.heading, snippet)
	}
	return fmt.Sprintf("基于知识库条目《%s》：%s", hit.article.Title, snippet)
}

func tokenizeSearchQuery(query string) []string {
	normalized := strings.ToLower(strings.TrimSpace(query))
	if normalized == "" {
		return nil
	}
	tokens := make([]string, 0, 16)
	seen := make(map[string]struct{}, 32)
	push := func(token string) {
		token = strings.TrimSpace(token)
		if token == "" {
			return
		}
		if _, ok := seen[token]; ok {
			return
		}
		seen[token] = struct{}{}
		tokens = append(tokens, token)
	}
	parts := strings.FieldsFunc(normalized, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-')
	})
	for _, part := range parts {
		part = strings.Trim(part, "-_")
		if part == "" {
			continue
		}
		runes := []rune(part)
		if len(runes) >= minTokenLen(part) {
			push(part)
		}
		if isASCIIWord(part) || len(runes) < 4 {
			continue
		}
		for n := 2; n <= 3; n++ {
			if len(runes) < n {
				continue
			}
			for i := 0; i+n <= len(runes); i++ {
				push(string(runes[i : i+n]))
				if len(tokens) >= 32 {
					return tokens
				}
			}
		}
	}
	compact := strings.Builder{}
	compact.Grow(len(normalized))
	for _, r := range normalized {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			compact.WriteRune(r)
		}
	}
	compactStr := compact.String()
	if len([]rune(compactStr)) >= 4 {
		push(compactStr)
	}
	if len(tokens) > 32 {
		return tokens[:32]
	}
	return tokens
}

func minTokenLen(token string) int {
	if isASCIIWord(token) {
		return 3
	}
	return 2
}

func isASCIIWord(token string) bool {
	for _, r := range token {
		if r > unicode.MaxASCII {
			return false
		}
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-') {
			return false
		}
	}
	return token != ""
}

func scoreArticleByTokens(item Article, tokens []string) int {
	if len(tokens) == 0 {
		return 0
	}
	title := strings.ToLower(item.Title)
	summary := strings.ToLower(item.Summary)
	content := strings.ToLower(item.Content)
	tags := make([]string, 0, len(item.Tags))
	for _, tag := range item.Tags {
		tags = append(tags, strings.ToLower(tag))
	}
	score := 0
	for _, token := range tokens {
		hit := false
		if token != "" && strings.Contains(title, token) {
			score += 8
			hit = true
		}
		if token != "" && strings.Contains(summary, token) {
			score += 5
			hit = true
		}
		for _, tag := range tags {
			if token != "" && strings.Contains(tag, token) {
				score += 4
				hit = true
				break
			}
		}
		if token != "" && strings.Contains(content, token) {
			score += 2
			hit = true
		}
		if hit {
			score++
		}
	}
	return score
}

// ImportDocs 负责把目录中的 Markdown 批量导入知识库
// 导入策略是路径去重并增量更新 避免重复导入造成版本膨胀
func (s *Service) syncCurrentVersionChunks() error {
	if s == nil || s.db == nil {
		return nil
	}
	rows, err := s.db.Query(`
		SELECT
			a.id,
			a.current_version,
			IFNULL(v.content_markdown, ''),
			IFNULL(v.created_at, a.updated_at)
		FROM kb_articles a
		LEFT JOIN kb_article_versions v
			ON v.article_id = a.id AND v.version = a.current_version
		ORDER BY a.updated_at DESC
	`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var articleID string
		var version int
		var content string
		var createdAt string
		if err := rows.Scan(&articleID, &version, &content, &createdAt); err != nil {
			return err
		}
		var count int
		if err := s.db.QueryRow(`
			SELECT COUNT(1)
			FROM kb_article_chunks
			WHERE article_id = ? AND version = ?
		`, articleID, version).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			continue
		}
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		if err := s.rebuildChunksTx(tx, articleID, version, content, createdAt); err != nil {
			rollbackTx(tx)
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (s *Service) rebuildChunksTx(tx *sql.Tx, articleID string, version int, content, createdAt string) error {
	if tx == nil {
		return fmt.Errorf("kb chunk tx is nil")
	}
	if strings.TrimSpace(articleID) == "" || version <= 0 {
		return fmt.Errorf("%w: invalid article chunk target", ErrInvalidInput)
	}
	if _, err := tx.Exec(`
		DELETE FROM kb_article_chunks
		WHERE article_id = ? AND version = ?
	`, articleID, version); err != nil {
		return fmt.Errorf("delete kb chunks failed: %w", err)
	}
	chunks := s.buildArticleChunks(content)
	if len(chunks) == 0 {
		return nil
	}
	chunkCreatedAt := strings.TrimSpace(createdAt)
	if chunkCreatedAt == "" {
		chunkCreatedAt = nowRFC3339()
	}
	for _, chunk := range chunks {
		if _, err := tx.Exec(`
			INSERT INTO kb_article_chunks (
				id, article_id, version, chunk_index, heading, content, chunk_hash, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, newID("kbc"), articleID, version, chunk.index, chunk.heading, chunk.content, chunk.hash, chunkCreatedAt); err != nil {
			return fmt.Errorf("insert kb chunk failed: %w", err)
		}
	}
	return nil
}

func (s *Service) buildArticleChunks(content string) []articleChunk {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	normalized = strings.TrimSpace(normalized)
	if normalized == "" {
		return nil
	}
	type chunkLine struct {
		heading string
		text    string
	}
	lines := make([]chunkLine, 0, 64)
	currentHeading := ""
	for _, raw := range strings.Split(normalized, "\n") {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			currentHeading = strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
			if currentHeading != "" {
				lines = append(lines, chunkLine{
					heading: currentHeading,
					text:    currentHeading,
				})
			}
			continue
		}
		lines = append(lines, chunkLine{
			heading: currentHeading,
			text:    normalizeChunkLine(trimmed),
		})
	}
	if len(lines) == 0 {
		return []articleChunk{newArticleChunk(1, "", normalized)}
	}
	chunkSize := s.chunkSize
	if chunkSize <= 0 {
		chunkSize = defaultChunkSize
	}
	chunkOverlap := s.chunkOverlap
	if chunkOverlap < 0 {
		chunkOverlap = 0
	}
	chunks := make([]articleChunk, 0, 8)
	start := 0
	for start < len(lines) {
		var builder strings.Builder
		chunkHeading := lines[start].heading
		runeCount := 0
		end := start
		for end < len(lines) {
			line := strings.TrimSpace(lines[end].text)
			if line == "" {
				end++
				continue
			}
			lineRunes := len([]rune(line))
			if runeCount > 0 && runeCount+1+lineRunes > chunkSize && end > start {
				break
			}
			if builder.Len() > 0 {
				builder.WriteByte('\n')
			}
			builder.WriteString(line)
			runeCount += lineRunes + 1
			if chunkHeading == "" && lines[end].heading != "" {
				chunkHeading = lines[end].heading
			}
			end++
		}
		chunkContent := strings.TrimSpace(builder.String())
		if chunkContent != "" {
			chunks = append(chunks, newArticleChunk(len(chunks)+1, chunkHeading, chunkContent))
		}
		if end >= len(lines) {
			break
		}
		nextStart := end
		overlapRunes := 0
		for i := end - 1; i > start; i-- {
			overlapRunes += len([]rune(lines[i].text)) + 1
			if overlapRunes >= chunkOverlap {
				nextStart = i
				break
			}
		}
		if nextStart <= start {
			nextStart = end
		}
		start = nextStart
	}
	if len(chunks) == 0 {
		return []articleChunk{newArticleChunk(1, "", normalized)}
	}
	return chunks
}

func newArticleChunk(index int, heading, content string) articleChunk {
	cleanHeading := strings.TrimSpace(heading)
	cleanContent := strings.TrimSpace(content)
	sum := sha1.Sum([]byte(cleanHeading + "\n" + cleanContent))
	return articleChunk{
		index:   index,
		heading: cleanHeading,
		content: cleanContent,
		hash:    hex.EncodeToString(sum[:]),
	}
}

func normalizeChunkLine(line string) string {
	trimmed := strings.TrimSpace(line)
	trimmed = strings.TrimPrefix(trimmed, "- ")
	trimmed = strings.TrimPrefix(trimmed, "* ")
	trimmed = strings.TrimPrefix(trimmed, "> ")
	return strings.TrimSpace(trimmed)
}

func (s *Service) ImportDocs(rootPath, operator string) (ImportResult, error) {
	if s == nil || s.db == nil {
		return ImportResult{}, fmt.Errorf("kb service not ready")
	}
	root := strings.TrimSpace(rootPath)
	if root == "" {
		root = "docs"
	}
	operator = normalizeOperator(operator)
	result := ImportResult{Files: []string{}}

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if strings.ToLower(filepath.Ext(path)) != ".md" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			result.Skipped++
			return nil
		}
		content := strings.TrimSpace(string(data))
		if content == "" {
			result.Skipped++
			return nil
		}
		title := parseTitle(content, filepath.Base(path))
		summary := parseSummary(content)
		refPath := filepath.ToSlash(path)
		refTitle := title
		tags := tagsFromPath(refPath)

		articleID, exists, err := s.findArticleIDByReference("import", refPath)
		if err != nil {
			result.Skipped++
			return nil
		}
		if exists {
			_, err = s.UpdateArticle(articleID, UpdateArticleInput{
				Title:      title,
				Summary:    summary,
				Category:   "docs",
				Severity:   SeverityMedium,
				Content:    content,
				Tags:       tags,
				UpdatedBy:  operator,
				ChangeNote: "sync docs import",
				SourceType: "import",
				SourceRef:  refPath,
				RefTitle:   refTitle,
			})
			if err != nil {
				result.Skipped++
				return nil
			}
			result.Updated++
			result.Files = append(result.Files, refPath)
			return nil
		}
		_, err = s.CreateArticle(CreateArticleInput{
			Title:      title,
			Summary:    summary,
			Category:   "docs",
			Severity:   SeverityMedium,
			Content:    content,
			Tags:       tags,
			CreatedBy:  operator,
			ChangeNote: "initial docs import",
			SourceType: "import",
			SourceRef:  refPath,
			RefTitle:   refTitle,
		})
		if err != nil {
			result.Skipped++
			return nil
		}
		result.Imported++
		result.Files = append(result.Files, refPath)
		return nil
	})
	if err != nil {
		return result, err
	}
	sort.Strings(result.Files)
	return result, nil
}

func (s *Service) findArticleIDByReference(refType, refPath string) (string, bool, error) {
	var articleID string
	err := s.db.QueryRow(`
		SELECT article_id
		FROM kb_references
		WHERE ref_type = ? AND ref_path = ?
		LIMIT 1
	`, strings.TrimSpace(refType), strings.TrimSpace(refPath)).Scan(&articleID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	return articleID, true, nil
}

// migrate 只做幂等结构迁移 不掺杂业务写入逻辑
// 这样迁移失败时影响范围可控 且重试不会破坏已有数据
func migrate(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS kb_articles (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			summary TEXT NOT NULL DEFAULT '',
			category TEXT NOT NULL DEFAULT '',
			severity TEXT NOT NULL DEFAULT 'medium',
			status TEXT NOT NULL DEFAULT 'draft',
			current_version INTEGER NOT NULL DEFAULT 1,
			created_by TEXT NOT NULL DEFAULT 'system',
			updated_by TEXT NOT NULL DEFAULT 'system',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS kb_article_versions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			article_id TEXT NOT NULL,
			version INTEGER NOT NULL,
			content_markdown TEXT NOT NULL,
			change_note TEXT NOT NULL DEFAULT '',
			source_type TEXT NOT NULL DEFAULT 'manual',
			source_ref TEXT NOT NULL DEFAULT '',
			created_by TEXT NOT NULL DEFAULT 'system',
			created_at TEXT NOT NULL,
			UNIQUE(article_id, version)
		);`,
		`CREATE TABLE IF NOT EXISTS kb_tags (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			type TEXT NOT NULL DEFAULT 'custom'
		);`,
		`CREATE TABLE IF NOT EXISTS kb_article_tags (
			article_id TEXT NOT NULL,
			tag_id TEXT NOT NULL,
			UNIQUE(article_id, tag_id)
		);`,
		`CREATE TABLE IF NOT EXISTS kb_reviews (
			id TEXT PRIMARY KEY,
			article_id TEXT NOT NULL,
			target_version INTEGER NOT NULL,
			action TEXT NOT NULL,
			comment TEXT NOT NULL DEFAULT '',
			operator TEXT NOT NULL DEFAULT 'system',
			created_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS kb_references (
			id TEXT PRIMARY KEY,
			article_id TEXT NOT NULL,
			ref_type TEXT NOT NULL,
			ref_path TEXT NOT NULL,
			ref_title TEXT NOT NULL DEFAULT ''
		);`,
		`CREATE TABLE IF NOT EXISTS kb_article_chunks (
			id TEXT PRIMARY KEY,
			article_id TEXT NOT NULL,
			version INTEGER NOT NULL,
			chunk_index INTEGER NOT NULL,
			heading TEXT NOT NULL DEFAULT '',
			content TEXT NOT NULL,
			chunk_hash TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			UNIQUE(article_id, version, chunk_index)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_kb_articles_status_updated ON kb_articles(status, updated_at);`,
		`CREATE INDEX IF NOT EXISTS idx_kb_versions_article_version ON kb_article_versions(article_id, version);`,
		`CREATE INDEX IF NOT EXISTS idx_kb_reviews_article_created ON kb_reviews(article_id, created_at);`,
		`CREATE INDEX IF NOT EXISTS idx_kb_references_type_path ON kb_references(ref_type, ref_path);`,
		`CREATE INDEX IF NOT EXISTS idx_kb_chunks_article_version ON kb_article_chunks(article_id, version, chunk_index);`,
		`CREATE INDEX IF NOT EXISTS idx_kb_chunks_hash ON kb_article_chunks(chunk_hash);`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("kb migrate failed: %w", err)
		}
	}
	return nil
}

func buildListWhere(query ListQuery) (string, []any) {
	parts := make([]string, 0, 6)
	args := make([]any, 0, 10)
	q := strings.ToLower(strings.TrimSpace(query.Query))
	if q != "" {
		pattern := "%" + q + "%"
		parts = append(parts, `(
			lower(a.title) LIKE ?
			OR lower(a.summary) LIKE ?
			OR EXISTS (
				SELECT 1 FROM kb_article_versions v
				WHERE v.article_id = a.id AND v.version = a.current_version
				AND lower(v.content_markdown) LIKE ?
			)
			OR EXISTS (
				SELECT 1
				FROM kb_article_tags at
				JOIN kb_tags t ON t.id = at.tag_id
				WHERE at.article_id = a.id AND lower(t.name) LIKE ?
			)
		)`)
		args = append(args, pattern, pattern, pattern, pattern)
	}
	status := normalizeArticleStatus(query.Status)
	if status != "" {
		parts = append(parts, "a.status = ?")
		args = append(args, status)
	} else if !query.IncludeArchived {
		parts = append(parts, "a.status != ?")
		args = append(args, StatusArchived)
	}
	severity := strings.ToLower(strings.TrimSpace(query.Severity))
	if severity != "" {
		parts = append(parts, "a.severity = ?")
		args = append(args, severity)
	}
	tag := strings.ToLower(strings.TrimSpace(query.Tag))
	if tag != "" {
		parts = append(parts, `EXISTS (
			SELECT 1
			FROM kb_article_tags at
			JOIN kb_tags t ON t.id = at.tag_id
			WHERE at.article_id = a.id AND lower(t.name) = ?
		)`)
		args = append(args, tag)
	}
	if len(parts) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(parts, " AND "), args
}

func queryArticleCore(db queryer, id string) (*Article, error) {
	return queryArticleCoreTx(db, id)
}

func queryArticleCoreTx(q queryer, id string) (*Article, error) {
	var item Article
	err := q.QueryRow(`
		SELECT
			a.id,
			a.title,
			a.summary,
			a.category,
			a.severity,
			a.status,
			a.current_version,
			a.created_by,
			a.updated_by,
			a.created_at,
			a.updated_at,
			IFNULL(v.content_markdown, ''),
			IFNULL(v.change_note, '')
		FROM kb_articles a
		LEFT JOIN kb_article_versions v
			ON v.article_id = a.id AND v.version = a.current_version
		WHERE a.id = ?
		LIMIT 1
	`, id).Scan(
		&item.ID,
		&item.Title,
		&item.Summary,
		&item.Category,
		&item.Severity,
		&item.Status,
		&item.CurrentVersion,
		&item.CreatedBy,
		&item.UpdatedBy,
		&item.CreatedAt,
		&item.UpdatedAt,
		&item.Content,
		&item.ChangeNote,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	item.Status = normalizeArticleStatusOrDraft(item.Status)
	item.NeedsReview = needsReviewByDays(item.Status, item.UpdatedAt, resolveReviewDays())
	return &item, nil
}

func queryArticleTags(db queryer, articleID string) ([]string, error) {
	rows, err := db.Query(`
		SELECT t.name
		FROM kb_tags t
		JOIN kb_article_tags at ON at.tag_id = t.id
		WHERE at.article_id = ?
		ORDER BY t.name ASC
	`, articleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]string, 0, 6)
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, err
		}
		out = append(out, tag)
	}
	return out, rows.Err()
}

func queryArticleRefs(db queryer, articleID string) ([]ArticleRef, error) {
	rows, err := db.Query(`
		SELECT ref_type, ref_path, ref_title
		FROM kb_references
		WHERE article_id = ?
		ORDER BY ref_type ASC, ref_path ASC
	`, articleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ArticleRef, 0, 4)
	for rows.Next() {
		var item ArticleRef
		if err := rows.Scan(&item.RefType, &item.RefPath, &item.RefTitle); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func queryArticleVersions(db queryer, articleID string) ([]ArticleVersion, error) {
	rows, err := db.Query(`
		SELECT version, content_markdown, change_note, source_type, source_ref, created_by, created_at
		FROM kb_article_versions
		WHERE article_id = ?
		ORDER BY version DESC
	`, articleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ArticleVersion, 0, 8)
	for rows.Next() {
		var item ArticleVersion
		if err := rows.Scan(
			&item.Version,
			&item.Content,
			&item.ChangeNote,
			&item.SourceType,
			&item.SourceRef,
			&item.CreatedBy,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func queryArticleReviews(db queryer, articleID string) ([]ReviewRecord, error) {
	rows, err := db.Query(`
		SELECT action, comment, operator, created_at
		FROM kb_reviews
		WHERE article_id = ?
		ORDER BY created_at DESC
	`, articleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ReviewRecord, 0, 8)
	for rows.Next() {
		var item ReviewRecord
		if err := rows.Scan(&item.Action, &item.Comment, &item.Operator, &item.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func queryTagsByArticleIDs(db queryer, articleIDs []string) (map[string][]string, error) {
	out := make(map[string][]string, len(articleIDs))
	if len(articleIDs) == 0 {
		return out, nil
	}
	for _, id := range articleIDs {
		tags, err := queryArticleTags(db, id)
		if err != nil {
			return nil, err
		}
		out[id] = tags
	}
	return out, nil
}

func replaceTagsTx(tx *sql.Tx, articleID string, tags []string) error {
	if _, err := tx.Exec(`DELETE FROM kb_article_tags WHERE article_id = ?`, articleID); err != nil {
		return err
	}
	normalized := normalizeTags(tags)
	for _, tag := range normalized {
		tagID, err := getOrCreateTagTx(tx, tag)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`
			INSERT OR IGNORE INTO kb_article_tags(article_id, tag_id)
			VALUES(?, ?)
		`, articleID, tagID); err != nil {
			return err
		}
	}
	return nil
}

func getOrCreateTagTx(tx *sql.Tx, tagName string) (string, error) {
	var id string
	err := tx.QueryRow(`SELECT id FROM kb_tags WHERE name = ? LIMIT 1`, tagName).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	id = newID("tag")
	if _, err := tx.Exec(`
		INSERT INTO kb_tags(id, name, type)
		VALUES(?, ?, ?)
	`, id, tagName, "custom"); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			if err := tx.QueryRow(`SELECT id FROM kb_tags WHERE name = ? LIMIT 1`, tagName).Scan(&id); err == nil {
				return id, nil
			}
		}
		return "", err
	}
	return id, nil
}

func upsertReferenceTx(tx *sql.Tx, articleID, refType, refPath, refTitle string) error {
	normalizedType := normalizeSourceType(refType)
	normalizedPath := strings.TrimSpace(refPath)
	if normalizedPath == "" {
		return nil
	}
	normalizedTitle := strings.TrimSpace(refTitle)
	if normalizedTitle == "" {
		normalizedTitle = normalizedPath
	}
	var refID string
	err := tx.QueryRow(`
		SELECT id
		FROM kb_references
		WHERE ref_type = ? AND ref_path = ?
		LIMIT 1
	`, normalizedType, normalizedPath).Scan(&refID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			refID = newID("ref")
			_, err = tx.Exec(`
				INSERT INTO kb_references(id, article_id, ref_type, ref_path, ref_title)
				VALUES(?, ?, ?, ?, ?)
			`, refID, articleID, normalizedType, normalizedPath, normalizedTitle)
			return err
		}
		return err
	}
	_, err = tx.Exec(`
		UPDATE kb_references
		SET article_id = ?, ref_title = ?
		WHERE id = ?
	`, articleID, normalizedTitle, refID)
	return err
}

func insertReviewTx(tx *sql.Tx, articleID string, targetVersion int, action, comment, operator, createdAt string) error {
	_, err := tx.Exec(`
		INSERT INTO kb_reviews(id, article_id, target_version, action, comment, operator, created_at)
		VALUES(?, ?, ?, ?, ?, ?, ?)
	`, newID("review"), articleID, targetVersion, action, comment, operator, createdAt)
	return err
}

func rollbackTx(tx *sql.Tx) {
	if tx != nil {
		_ = tx.Rollback()
	}
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func normalizeTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	out := make([]string, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		t := strings.ToLower(strings.TrimSpace(tag))
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

func normalizeSeverity(raw string) string {
	val := strings.ToLower(strings.TrimSpace(raw))
	switch val {
	case SeverityLow, SeverityMedium, SeverityHigh:
		return val
	case "低":
		return SeverityLow
	case "中":
		return SeverityMedium
	case "高":
		return SeverityHigh
	default:
		return SeverityMedium
	}
}

func normalizeOperator(raw string) string {
	val := strings.TrimSpace(raw)
	if val == "" {
		return "system"
	}
	return val
}

func normalizeSourceType(raw string) string {
	val := strings.ToLower(strings.TrimSpace(raw))
	switch val {
	case "manual", "import", "ai-generated", "rollback", "incident", "alert", "change", "postmortem":
		return val
	default:
		return "manual"
	}
}

func newID(prefix string) string {
	buf := make([]byte, 4)
	_, _ = rand.Read(buf)
	hexPart := hex.EncodeToString(buf)
	return fmt.Sprintf("%s_%d_%s", prefix, time.Now().UnixNano(), hexPart)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func collectArticleIDs(items []Article) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item.ID != "" {
			out = append(out, item.ID)
		}
	}
	return out
}

func snippetFromContent(content string, max int) string {
	val := strings.TrimSpace(content)
	if val == "" {
		return ""
	}
	if max <= 0 {
		max = 180
	}
	lines := strings.Split(val, "\n")
	for _, line := range lines {
		clean := strings.TrimSpace(strings.TrimPrefix(line, "#"))
		if clean == "" {
			continue
		}
		if len([]rune(clean)) <= max {
			return clean
		}
		r := []rune(clean)
		return string(r[:max]) + "..."
	}
	if len([]rune(val)) <= max {
		return val
	}
	r := []rune(val)
	return string(r[:max]) + "..."
}

func parseTitle(content, fallback string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			title := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
			if title != "" {
				return title
			}
		}
	}
	name := strings.TrimSuffix(fallback, filepath.Ext(fallback))
	name = strings.TrimSpace(name)
	if name == "" {
		return "Untitled"
	}
	return name
}

func parseSummary(content string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if len([]rune(trimmed)) > 160 {
			r := []rune(trimmed)
			return string(r[:160]) + "..."
		}
		return trimmed
	}
	return ""
}

func tagsFromPath(path string) []string {
	clean := filepath.ToSlash(strings.TrimSpace(path))
	if clean == "" {
		return nil
	}
	parts := strings.Split(clean, "/")
	if len(parts) <= 1 {
		return []string{"docs"}
	}
	tags := []string{"docs"}
	for _, part := range parts[:len(parts)-1] {
		p := strings.ToLower(strings.TrimSpace(part))
		if p == "" || p == "." {
			continue
		}
		tags = append(tags, p)
	}
	return normalizeTags(tags)
}

type queryer interface {
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

func normalizeArticleStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return ""
	case StatusDraft:
		return StatusDraft
	case StatusReviewing, "pending_review":
		return StatusReviewing
	case StatusPublished:
		return StatusPublished
	case StatusArchived:
		return StatusArchived
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

func normalizeArticleStatusOrDraft(raw string) string {
	normalized := normalizeArticleStatus(raw)
	if normalized == "" {
		return StatusDraft
	}
	return normalized
}

// isKBActionAllowed 统一约束知识条目状态迁移
// 原因：submit/approve/reject 必须依赖明确状态，否则“草稿”和“待审核”会被混用
// 边界：archive 允许从 draft/reviewing/published 进入 archived，方便清理历史草稿或中止审核
func isKBActionAllowed(status, action string) bool {
	normalizedStatus := normalizeArticleStatusOrDraft(status)
	normalizedAction := strings.ToLower(strings.TrimSpace(action))
	switch normalizedAction {
	case "submit":
		return normalizedStatus == StatusDraft
	case "approve", "reject":
		return normalizedStatus == StatusReviewing
	case "archive":
		return normalizedStatus == StatusDraft || normalizedStatus == StatusReviewing || normalizedStatus == StatusPublished
	default:
		return false
	}
}

func resolveChunkSize() int {
	raw := strings.TrimSpace(os.Getenv("KB_RAG_CHUNK_SIZE"))
	if raw == "" {
		return defaultChunkSize
	}
	val, err := strconv.Atoi(raw)
	if err != nil || val < 120 {
		return defaultChunkSize
	}
	return val
}

func resolveChunkOverlap() int {
	raw := strings.TrimSpace(os.Getenv("KB_RAG_CHUNK_OVERLAP"))
	if raw == "" {
		return defaultChunkOverlap
	}
	val, err := strconv.Atoi(raw)
	if err != nil || val < 0 {
		return defaultChunkOverlap
	}
	if val >= resolveChunkSize() {
		return defaultChunkOverlap
	}
	return val
}

func resolveReviewDays() int {
	raw := strings.TrimSpace(os.Getenv("KB_REVIEW_DAYS"))
	if raw == "" {
		return defaultReviewDays
	}
	val, err := strconv.Atoi(raw)
	if err != nil || val <= 0 {
		return defaultReviewDays
	}
	return val
}

func (s *Service) isNeedsReview(status, updatedAt string) bool {
	days := s.reviewDays
	if days <= 0 {
		days = defaultReviewDays
	}
	return needsReviewByDays(status, updatedAt, days)
}

func needsReviewByDays(status, updatedAt string, days int) bool {
	if strings.ToLower(strings.TrimSpace(status)) != StatusPublished {
		return false
	}
	if days <= 0 {
		return false
	}
	updatedTime, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(updatedAt))
	if err != nil {
		if fallback, errFallback := time.Parse(time.RFC3339, strings.TrimSpace(updatedAt)); errFallback == nil {
			updatedTime = fallback
		} else {
			return false
		}
	}
	expireAt := updatedTime.Add(time.Duration(days) * 24 * time.Hour)
	return time.Now().After(expireAt)
}
