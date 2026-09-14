package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/logging"
)

// ── 候选有效介绍向量与增量回补（improve-discovery-recommendations 4.6，design D6 / spec
// 「Effective Descriptions and Incremental Embeddings」）──
//
// 文本：从候选现有可用名称、网站、分类/语言/地区与有效描述构建（人工非空覆盖上游，
// EffectiveMetadata 复用），清除 Markdown/提示块等格式噪音后按 rune 限长，不用 LLM 编造
// 来源定位。指纹 = sha256(生成器版本 + 文本)，生成规则变化（版本常量升位）即视作全量 dirty。
//
// 回补：只处理「无向量行」或「指纹与已有行不一致」的候选，未变复用已有向量；每批有界
// （默认 20 条，调度每小时一批）；生成时携带候选 revision 与指纹，写库前复查二者，任何
// 变化即丢弃本次结果（旧请求不得覆盖新资料）；生成/写库失败保留旧向量、不改指纹 → 下一批
// 自动重试（无重试计数列，靠「指纹仍不匹配」这一天然 dirty 状态，见 D6）。
//
// 模型/维度：行以 (candidate_id, model, dimension) 唯一；模型切换后旧行不会被本回补删除，
// 检索侧（discovery_recall.checkCandidateVectorConsistency）以同模型同维校验阻断混算轮次
// （design D6：不同模型不得用旧向量兜底）——本回补只负责把当前配置下的向量补齐/覆盖。

// CandidateEmbeddingBatchSizeDefault 是单次回补的候选上限（design D9：初始全量回补限批）。
const CandidateEmbeddingBatchSizeDefault = 20

// candidateEmbeddingScanWindow 是「已有向量行」候选的扫描窗口：指纹只能逐行计算，
// 无法下推到 SQL，故按 updated_at 倒序扫描有限窗口——人工编辑/同步都会把候选推到窗口
// 最前，未变候选再多也只占扫描成本（不落库、不调模型）。
const candidateEmbeddingScanWindow = 200

// candidateEmbeddingGeneratorVersion 是生成规则版本（design D6「文本指纹包括有效元数据与
// 生成器版本」）：清洗规则、字段构成或限长变化时必须升位，使全部候选重新变为 dirty。
// v2：分段预算收紧到总量 ≤500 rune（旧 v1 文本最长 1800 rune 超供应商 512 token 上限，
// 部分候选永远嵌不上形成回填永败循环）——版本升位强制已嵌候选按短文本重嵌，保证同代
// 文本一致（同一向量表内不得混两代文本的向量）。
const candidateEmbeddingGeneratorVersion = "candidate-effective-text-v2"

// 分段 rune 上限：总量上限 500 rune（名称 80 + 网站 80 + 分类/语言/地区 80 + 说明 257
// + 3 个换行分隔符 = 500）。
//
// 为什么是 500：embedding 供应商（qwen3-embedding 经本地 openai_compatible 网关）单次
// 输入硬上限 512 token；CJK 最坏情况 ≈ 1 rune ≈ 1 token（实测日志 `input (794 tokens)
// is too large (batch size 512)` 即旧 1800 rune 预算触发），故预算必须留出安全余量。
// 一律按 rune 计数，禁止按字节截断中文。
const (
	candidateEmbeddingNameMaxRunes           = 80
	candidateEmbeddingWebsiteMaxRunes        = 80
	candidateEmbeddingClassificationMaxRunes = 80
	candidateEmbeddingDescriptionMaxRunes    = 257
)

// ── 文本清洗与拼装（纯函数，无 DB/网络/时间）──

var (
	// HTML 注释（含 RSSHub 文档/摘要链路用的 <!-- form: ... --> 形态提示标记）先于标签剥离。
	effectiveHTMLCommentPattern = regexp.MustCompile(`(?s)<!--.*?-->`)
	// Markdown 图片 → 保留 alt 文本。
	effectiveMarkdownImagePattern = regexp.MustCompile(`!\[([^\]]*)\]\([^)]*\)`)
	// Markdown 链接 → 保留链接文字。
	effectiveMarkdownLinkPattern = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	// 代码围栏行（``` 或 ```lang）整行去掉，围栏内的正文保留。
	effectiveCodeFencePattern = regexp.MustCompile("(?m)^[ \t]*`{3,}[A-Za-z0-9_+-]*[ \t]*$")
	// 提示块容器标记行（VitePress 风格，含可选标题）：::: tip / ::: warning 标题 / :::，
	// 整行是格式噪音（标题同属该标记语法），去掉。
	effectivePromptBlockPattern = regexp.MustCompile(`(?m)^[ \t]*:::+[ \t]*.*$`)
	// 标题井号、引用箭头、列表符号等行首格式标记。
	effectiveHeadingPattern = regexp.MustCompile(`(?m)^[ \t]{0,3}#{1,6}[ \t]*`)
	effectiveQuotePattern   = regexp.MustCompile(`(?m)^[ \t]{0,3}>[ \t]?`)
	effectiveBulletPattern  = regexp.MustCompile(`(?m)^[ \t]{0,3}([-*+]|\d{1,3}[.)])[ \t]+`)
	// 成对强调标记与反引号；单个 * / _ 保留（可能是名称里的合法字符）。
	effectiveEmphasisPattern = regexp.MustCompile("[*_~]{2,}|`")
	// 简单 HTML 标签（保留标签内文字）。
	effectiveHTMLTagPattern = regexp.MustCompile(`</?[A-Za-z][^>]*>`)
	// 空白规范化：换行/tab/全角空格/窄空格等一律折叠为单空格。
	effectiveWhitespacePattern = regexp.MustCompile(`[\s\x{00a0}\x{1680}\x{2000}-\x{200a}\x{202f}\x{205f}\x{3000}]+`)
)

// SanitizeEffectiveText 清除格式噪音并规范空白（design D6「清除格式噪音但保留正文」）：
// HTML 注释/标签、Markdown 链接与图片语法、代码围栏与提示块标记行、标题/引用/列表符号、
// 成对强调符与反引号全部去掉，正文文字保留；空白折叠为单空格并去首尾。
func SanitizeEffectiveText(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	out := effectiveHTMLCommentPattern.ReplaceAllString(s, " ")
	out = effectiveMarkdownImagePattern.ReplaceAllString(out, "$1")
	out = effectiveMarkdownLinkPattern.ReplaceAllString(out, "$1")
	out = effectiveCodeFencePattern.ReplaceAllString(out, " ")
	out = effectivePromptBlockPattern.ReplaceAllString(out, " ")
	out = effectiveHeadingPattern.ReplaceAllString(out, "")
	out = effectiveQuotePattern.ReplaceAllString(out, "")
	out = effectiveBulletPattern.ReplaceAllString(out, "")
	out = effectiveEmphasisPattern.ReplaceAllString(out, "")
	out = effectiveHTMLTagPattern.ReplaceAllString(out, " ")
	return strings.TrimSpace(effectiveWhitespacePattern.ReplaceAllString(out, " "))
}

// truncateRunes 按 rune 截断到 max（不按字节截断中文）；max <= 0 或已足够短时原样返回。
func truncateRunes(s string, max int) string {
	if max <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

// EffectiveDescriptionText 构造候选推荐文本（design D6）：名称、网站、分类组
// （分类/语言/地区合计 80 rune）、说明四段，逐段清洗后按 rune 限长（总量 ≤500 rune，
// 见分段上限常量注释的 token 数学），非空段以换行拼接；
// 四段皆空返回空串——调用方不得为无资料的候选编造向量（D6：没有资料保留未知）。
func EffectiveDescriptionText(name, website, classification, description string) string {
	parts := []string{
		truncateRunes(SanitizeEffectiveText(name), candidateEmbeddingNameMaxRunes),
		truncateRunes(SanitizeEffectiveText(website), candidateEmbeddingWebsiteMaxRunes),
		truncateRunes(SanitizeEffectiveText(classification), candidateEmbeddingClassificationMaxRunes),
		truncateRunes(SanitizeEffectiveText(description), candidateEmbeddingDescriptionMaxRunes),
	}
	nonEmpty := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	return strings.Join(nonEmpty, "\n")
}

// CandidateEmbeddingTextHash 计算文本指纹：生成器版本 + 文本，含版本使生成规则变化即全量
// 失效（design D6）。返回 32 位 hex（沿用 sha256Hex32 风格，列宽 size:64 有富余）。
func CandidateEmbeddingTextHash(text string) string {
	return sha256Hex32(candidateEmbeddingGeneratorVersion + "\n" + text)
}

// ── 回补服务 ──

// CandidateEmbedderFunc 生成单条文本的向量：返回向量、维度与模型名（模型为空视为失败——
// 唯一键含 model，空模型行会污染检索侧的同模型校验）。可注入以便测试与离线回补。
type CandidateEmbedderFunc func(ctx context.Context, text string) ([]float64, int, string, error)

// CandidateEmbeddingBackfillSummary 是一批回补的产出统计（job 的 Data/Summary 直接使用）。
type CandidateEmbeddingBackfillSummary struct {
	Generated int `json:"generated"` // 成功生成并写入向量的候选数
	Stale     int `json:"stale"`     // 写库前复查发现资料/revision 已变，丢弃本次结果
	Failed    int `json:"failed"`    // 生成或写库失败，保留旧向量待下批重试
	NoText    int `json:"no_text"`   // 无可用介绍文本，跳过（不编造资料）
}

// CandidateEmbeddingService 执行候选有效介绍的增量向量回补。
type CandidateEmbeddingService struct {
	db    *gorm.DB
	embed CandidateEmbedderFunc
}

// NewCandidateEmbeddingService 构造生产服务：向量走 airouter.Router.Embed（capability=embedding，
// operation=discovery.candidate_embedding，审计日志由 airouter 记录）。
func NewCandidateEmbeddingService(db *gorm.DB, router *airouter.Router) *CandidateEmbeddingService {
	return &CandidateEmbeddingService{db: db, embed: newAirouterCandidateEmbedder(router)}
}

// NewCandidateEmbeddingServiceWithEmbedder 用注入的生成函数构造（测试/离线回补）。
func NewCandidateEmbeddingServiceWithEmbedder(db *gorm.DB, embed CandidateEmbedderFunc) *CandidateEmbeddingService {
	svc := &CandidateEmbeddingService{db: db, embed: embed}
	if svc.embed == nil {
		svc.embed = newAirouterCandidateEmbedder(nil)
	}
	return svc
}

// newAirouterCandidateEmbedder 把 airouter 封装为 CandidateEmbedderFunc。
func newAirouterCandidateEmbedder(router *airouter.Router) CandidateEmbedderFunc {
	return func(ctx context.Context, text string) ([]float64, int, string, error) {
		if router == nil {
			return nil, 0, "", errors.New("candidate embedding: ai router is not configured")
		}
		res, err := router.Embed(ctx, airouter.EmbeddingRequest{
			Input:     []string{text},
			Operation: "discovery.candidate_embedding",
		}, airouter.CapabilityEmbedding)
		if err != nil {
			return nil, 0, "", err
		}
		if len(res.Embeddings) == 0 || len(res.Embeddings[0]) == 0 {
			return nil, 0, "", errors.New("candidate embedding: provider returned an empty vector")
		}
		vec := res.Embeddings[0]
		dim := res.Dimensions
		if dim <= 0 {
			dim = len(vec)
		}
		model := strings.TrimSpace(res.Model)
		if model == "" {
			return nil, 0, "", errors.New("candidate embedding: provider returned an empty model name")
		}
		return vec, dim, model, nil
	}
}

// candidateEmbeddingSource 是回补所需的候选快照（候选 + 上游路由资料 + 已有指纹）。
// StoredHash 为空表示尚无 candidate_embeddings 行。
type candidateEmbeddingSource struct {
	CandidateID uint
	Revision    uint
	Kind        string
	FeedURL     *string
	Manual      models.MetadataMap
	RouteName   *string
	RouteDesc   *string
	RouteURL    *string
	RouteNS     *string
	StoredHash  string
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// text 组装本候选的有效介绍文本（人工非空覆盖上游；上游资料仅 RSSHub 路由有）。
func (src candidateEmbeddingSource) text() string {
	eff := EffectiveMetadata(src.Manual, derefString(src.RouteName), derefString(src.RouteDesc), "", "")
	// 网站：RSSHub 路由的站点地址优先；原生 RSS 候选没有独立站点资料，退化为订阅地址。
	website := derefString(src.RouteURL)
	if website == "" {
		website = derefString(src.FeedURL)
	}
	// 分类组：RSSHub namespace 是上游目录的分类分组；语言/地区目前只有人工值。
	classification := strings.Join(nonEmptyStrings(
		derefString(src.RouteNS), eff.Language, eff.Region,
	), " ")
	return EffectiveDescriptionText(eff.Name, website, classification, eff.Description)
}

// fingerprint 本候选当前文本的指纹。
func (src candidateEmbeddingSource) fingerprint() string {
	return CandidateEmbeddingTextHash(src.text())
}

// isDirty 无向量行或指纹变化即为 dirty；未变复用已有向量（spec：未变化不重复生成）。
func (src candidateEmbeddingSource) isDirty() bool {
	text := src.text()
	if text == "" {
		return false // 无资料候选不为生成而排队（调用方按 NoText 跳过并留痕）
	}
	return src.StoredHash != CandidateEmbeddingTextHash(text)
}

// nonEmptyStrings 过滤空串（保持顺序）。
func nonEmptyStrings(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// candidateEmbeddingSourceSelect 是回补读取候选快照的统一 SELECT 前缀（列名与
// candidateEmbeddingSource 字段一一对应；sqlite 与 PostgreSQL 通用）。
const candidateEmbeddingSourceSelect = `
	SELECT fc.id AS candidate_id,
	       fc.revision AS revision,
	       fc.kind AS kind,
	       fc.feed_url AS feed_url,
	       fc.manual_metadata AS manual,
	       r.name AS route_name,
	       r.description AS route_desc,
	       r.url AS route_url,
	       r.namespace AS route_ns,
	       ce.text_hash AS stored_hash
	FROM feed_candidates fc
	LEFT JOIN rsshub_routes r ON r.id = fc.route_id
	LEFT JOIN candidate_embeddings ce ON ce.candidate_id = fc.id`

// candidateEmbeddingEligibleSQL 排除已从上游目录消失（gone）的路由：它们不再参与推荐，
// 不为其消耗 embedding 额度；路由复现（sync 清 gone）后自然重新成为 dirty。
const candidateEmbeddingEligibleSQL = ` WHERE (fc.route_id IS NULL OR r.status IS NULL OR r.status <> 'gone')`

// listSources 按附加条件读取候选快照（updated_at 倒序，最近变动的优先处理）。
func (s *CandidateEmbeddingService) listSources(ctx context.Context, extraWhere string, limit int) ([]candidateEmbeddingSource, error) {
	q := candidateEmbeddingSourceSelect + candidateEmbeddingEligibleSQL + extraWhere +
		" ORDER BY fc.updated_at DESC, fc.id DESC LIMIT ?"
	var rows []candidateEmbeddingSource
	if err := s.db.WithContext(ctx).Raw(q, limit).Scan(&rows).Error; err != nil {
		return nil, err
	}
	for i := range rows {
		if rows[i].Manual == nil {
			rows[i].Manual = models.MetadataMap{}
		}
	}
	return rows, nil
}

// loadSource 读取单个候选快照（写库前复查用）。
func (s *CandidateEmbeddingService) loadSource(ctx context.Context, tx *gorm.DB, candidateID uint, lock bool) (*candidateEmbeddingSource, error) {
	q := candidateEmbeddingSourceSelect + candidateEmbeddingEligibleSQL + " AND fc.id = ?"
	if lock && s.db.Name() == "postgres" {
		// 复查窗口内阻止其它回补并发改写该候选向量行（单机调度已互斥，这里只防外部进程）。
		q += " FOR UPDATE OF fc"
	}
	var rows []candidateEmbeddingSource
	if err := tx.WithContext(ctx).Raw(q, candidateID).Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	if rows[0].Manual == nil {
		rows[0].Manual = models.MetadataMap{}
	}
	return &rows[0], nil
}

// errStaleEmbeddingSource 表示写库前复查发现资料/revision 已变：本次结果作废，不写库。
var errStaleEmbeddingSource = errors.New("candidate embedding: source changed before write")

// DirtyCandidateEmbeddings 执行一批增量回补（limit<=0 取默认 20）：
// 无向量行或指纹变化的候选 → 生成 → 写库前复查 → upsert (candidate_id, model, dimension)。
// 单条候选失败不中断整批（保留旧向量，下批重试），只有 DB 层读取失败才返回错误。
func (s *CandidateEmbeddingService) DirtyCandidateEmbeddings(ctx context.Context, limit int) (*CandidateEmbeddingBackfillSummary, error) {
	if !LoadDiscoveryV2Enabled(s.db) {
		return nil, newCandidateConfigurationError(discoveryV2DisabledMessage)
	}
	if limit <= 0 {
		limit = CandidateEmbeddingBatchSizeDefault
	}
	summary := &CandidateEmbeddingBackfillSummary{}

	// 第一段：完全没有向量行的候选（初始全量回补的主要来源），SQL 层直接限批。
	missing, err := s.listSources(ctx, " AND NOT EXISTS (SELECT 1 FROM candidate_embeddings x WHERE x.candidate_id = fc.id)", limit)
	if err != nil {
		return nil, fmt.Errorf("list candidates without embedding: %w", err)
	}
	sources := missing
	seen := make(map[uint]struct{}, len(missing))
	for _, src := range missing {
		seen[src.CandidateID] = struct{}{}
	}
	// 第二段：已有向量行的候选按指纹判断是否变化（扫描窗口见常量注释），补足剩余名额。
	// 扫描窗口会带回无向量行的候选（它们也落在同一批候选里），必须按 ID 去重。
	if len(sources) < limit {
		scanned, scanErr := s.listSources(ctx, "", candidateEmbeddingScanWindow)
		if scanErr != nil {
			return nil, fmt.Errorf("scan candidates with embedding: %w", scanErr)
		}
		for _, src := range scanned {
			if len(sources) >= limit {
				break
			}
			if _, dup := seen[src.CandidateID]; dup {
				continue
			}
			if !src.isDirty() {
				continue
			}
			seen[src.CandidateID] = struct{}{}
			sources = append(sources, src)
		}
	}

	for _, src := range sources {
		text := src.text()
		if text == "" {
			summary.NoText++
			continue
		}
		vec, dim, model, embedErr := s.embed(ctx, text)
		if embedErr != nil {
			logging.Warnf("candidate embedding: generate for candidate %d failed (kept previous vector): %v", src.CandidateID, embedErr)
			summary.Failed++
			continue
		}
		written, writeErr := s.writeEmbedding(ctx, src, vec, dim, model)
		if writeErr != nil {
			logging.Warnf("candidate embedding: persist for candidate %d failed (kept previous vector): %v", src.CandidateID, writeErr)
			summary.Failed++
			continue
		}
		if !written {
			summary.Stale++
			continue
		}
		summary.Generated++
	}
	return summary, nil
}

// writeEmbedding 在一个事务里复查候选 revision 与指纹后 upsert 向量行。
// 返回 written=false 表示复查失败（旧请求作废，未写库）。
func (s *CandidateEmbeddingService) writeEmbedding(ctx context.Context, src candidateEmbeddingSource, vec []float64, dim int, model string) (bool, error) {
	written := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		fresh, err := s.loadSource(ctx, tx, src.CandidateID, true)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				// 候选在生成期间被删除或路由被标 gone：本次结果无处可用。
				return errStaleEmbeddingSource
			}
			return err
		}
		if fresh.Revision != src.Revision || fresh.fingerprint() != src.fingerprint() {
			return errStaleEmbeddingSource
		}
		row := models.CandidateEmbedding{
			CandidateID: src.CandidateID,
			Model:       model,
			Dimension:   dim,
			TextHash:    src.fingerprint(),
			// EmbeddingConfigID 留空：airouter 不回传 embedding_config 行 id，检索侧也不消费该列。
			EmbeddingVec: floatsToPgVector(vec),
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "candidate_id"}, {Name: "model"}, {Name: "dimension"},
			},
			DoUpdates: clause.AssignmentColumns([]string{"text_hash", "embedding", "updated_at"}),
		}).Create(&row).Error; err != nil {
			return err
		}
		written = true
		return nil
	})
	if errors.Is(err, errStaleEmbeddingSource) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return written, nil
}
