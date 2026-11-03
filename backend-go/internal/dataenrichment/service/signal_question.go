package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
)

// ── 问题驱动研究协议（add-question-driven-news-research D1/D3）───────────────
//
// question-v1 是候选 research_brief 的版本化研究协议：新发现把
// input_snapshot.research_contract 标为 question-v1，研究循环据此额外授权
// read_source / web_search / 受限 fetch_page 三个动作。空协议 = 旧冻结研究
// 协议（仅七源），未知协议拒绝研究而非默认放开工具（旧候选不自动升级）。

// SignalResearchContractQuestionV1 是当前唯一的新协议版本号。
const SignalResearchContractQuestionV1 = "question-v1"

// question-v1 追加预算（design D3）：单次研究外搜最多 3 次、网页读取最多
// 5 次；失败计入对应预算；重复/非法请求不执行只计决策轮（去重由共享循环
// 的 dedupKeyFor 承担）。这些是上限不是配额，40 轮/40 执行总预算不变。
const (
	SignalQuestionMaxWebSearches = 3
	SignalQuestionMaxWebPages    = 5
)

// question 协议授权的三个动作名（七源之外的唯一新增；calculate 仍是循环
// 原生动作，不是注册表工具）。
var signalQuestionToolNames = []string{"read_source", "web_search", "fetch_page"}

func isSignalQuestionTool(name string) bool {
	for _, n := range signalQuestionToolNames {
		if n == name {
			return true
		}
	}
	return false
}

// SignalResearchContractOf 从发现批次的 input_snapshot 读取研究协议（空=旧
// 协议；快照损坏返回空串，由研究入口的完整解析兜底报错）。
func SignalResearchContractOf(inputSnapshot json.RawMessage) string {
	var probe struct {
		ResearchContract string `json:"research_contract"`
	}
	if err := json.Unmarshal(inputSnapshot, &probe); err != nil {
		return ""
	}
	return probe.ResearchContract
}

// ── research_brief（候选 jsonb，D1）───────────────────────────────────────────

// SignalResearchBrief 是候选 research_brief 列的 JSON 形状：unknown（还不
// 清楚什么）与 verification（准备先查什么）来自校验后的 detect，source_ids
// 由代码按冻结快照核实填写（禁止信任模型自报）。旧行 NULL，不猜填。
type SignalResearchBrief struct {
	Unknown      string   `json:"unknown"`
	Verification string   `json:"verification"`
	SourceIDs    []string `json:"source_ids"`
}

// ToJSON serializes the brief for the candidate row（一次分析一行快照，写入
// 后不可变）。
func (b *SignalResearchBrief) ToJSON() (json.RawMessage, error) {
	if b == nil {
		return nil, errors.New("brief is nil")
	}
	bj, err := json.Marshal(b)
	if err != nil {
		return nil, fmt.Errorf("marshal research brief: %w", err)
	}
	return bj, nil
}

// ParseSignalResearchBrief decodes a candidate row's research_brief jsonb.
// 空/NULL → nil, nil（旧候选）；损坏 JSON → 错误（question-v1 缺 brief 或
// brief 损坏直接失败，不静默降级）。
func ParseSignalResearchBrief(raw json.RawMessage) (*SignalResearchBrief, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var b SignalResearchBrief
	if err := json.Unmarshal(raw, &b); err != nil {
		return nil, fmt.Errorf("parse research brief: %w", err)
	}
	return &b, nil
}

// ValidateSignalBriefText 校验 brief 文本字段：trim Unicode 空白后 1～600
// rune（V1 输入变体：空白/超长拒绝；作为非空文本校验，不冒充语义保证）。
// 返回 trim 后的值。
func ValidateSignalBriefText(field, v string) (string, error) {
	t := strings.TrimSpace(v)
	n := len([]rune(t))
	if n == 0 {
		return "", fmt.Errorf("%s 不能为空", field)
	}
	if n > signalBriefMaxRunes {
		return "", fmt.Errorf("%s 长度 %d 超过上限 %d 字符", field, n, signalBriefMaxRunes)
	}
	return t, nil
}

const signalBriefMaxRunes = 600

// ── 检索 query 校验（D3：只发公开主题，不拼全文/内部ID/凭证）────────────────

// validateSignalSearchQuery 校验外搜 query：trim 后 1～300 rune 且至少含一
// 个 Unicode 字母或数字（纯分隔符/标点拒绝，不做表达式拆分；单实体如
// "FEC" 接受；不强改大小写）。
func validateSignalSearchQuery(q string) (string, error) {
	t := strings.TrimSpace(q)
	n := len([]rune(t))
	if n == 0 {
		return "", errors.New("query 不能为空")
	}
	if n > signalSearchQueryMaxRunes {
		return "", fmt.Errorf("query 长度 %d 超过上限 %d 字符", n, signalSearchQueryMaxRunes)
	}
	for _, r := range t {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return t, nil
		}
	}
	return "", errors.New("query 必须至少包含一个字母或数字")
}

const signalSearchQueryMaxRunes = 300

// ── question-v1 研究运行时（先读门/白名单/预算/证据账本）────────────────────

// signalQuestionRuntime 承载一次 question-v1 研究运行的全部来源访问状态：
// 候选冻结白名单、先读门、搜索/抓页预算、已读证据与 URL 登记簿。单
// goroutine 顺序访问（循环体），无需加锁。
type signalQuestionRuntime struct {
	brief    *SignalResearchBrief
	material *SignalMaterial
	sources  map[string]*SignalFrozenSource // 本候选白名单（brief.SourceIDs → 冻结来源）
	readOK   map[string]bool                // 成功读取过的本地 source_id

	registered map[string]bool                        // 本 run 允许请求的归一化 URL（搜索登记 + 候选来源 URL）
	webSeq     int                                    // wN 证据序号
	webSources map[string]*signalResearchSourceRecord // 成功且时间合格的 wN 证据
	searchHits []WebSearchResult                      // 最近一次搜索的导航结果（不构成证据）

	searchesUsed int
	pagesUsed    int
	// missingLocalGap：白名单无可读本地正文时的缺口说明（bindLedger 落账一次）。
	missingLocalGap string

	searcher SignalWebSearchAdapter
	fetcher  SignalPageFetchAdapter
	ledger   *signalResearchLedger
	now      func() time.Time
}

// SignalWebSearchAdapter 是 question 研究的搜索后端面（生产实现=既有
// BochaWebSearcher；测试注入 fake；nil=未配置降级 typed gap）。
type SignalWebSearchAdapter interface {
	Search(ctx context.Context, query string) ([]WebSearchResult, error)
}

// SignalPageFetchAdapter 是 question 研究的安全抓页面（生产实现=
// SignalSafePageFetcher；测试注入 fake；nil=未配置降级 typed gap）。
type SignalPageFetchAdapter interface {
	FetchPage(ctx context.Context, url string) (*SignalPageFetchResult, error)
}

// newSignalQuestionRuntime 按材料协议构造 runtime：空协议 → (nil, nil)（旧
// 路径零变化）；question-v1 → runtime（brief 必须存在且合法）；未知协议 →
// 错误（拒绝研究，不默认放开工具）。协议只来自冻结快照，客户端无法覆写。
func newSignalQuestionRuntime(material *SignalMaterial, brief *SignalResearchBrief, searcher SignalWebSearchAdapter, fetcher SignalPageFetchAdapter) (*signalQuestionRuntime, error) {
	if material == nil {
		return nil, nil
	}
	switch material.ResearchContract {
	case "":
		return nil, nil
	case SignalResearchContractQuestionV1:
	default:
		return nil, fmt.Errorf("未知研究协议 %q：拒绝研究（不默认放开工具）", material.ResearchContract)
	}
	if brief == nil {
		return nil, errors.New("question-v1 候选缺少 research_brief，拒绝研究")
	}
	// brief 文本形状由唯一写入口（detect 冻结）校验；此处只要求存在（损坏
	// JSON 已在 ParseSignalResearchBrief 报错）。
	rt := &signalQuestionRuntime{
		brief:      brief,
		material:   material,
		sources:    make(map[string]*SignalFrozenSource, len(brief.SourceIDs)),
		readOK:     map[string]bool{},
		registered: map[string]bool{},
		webSources: map[string]*signalResearchSourceRecord{},
		searcher:   searcher,
		fetcher:    fetcher,
		now:        time.Now,
	}
	// 白名单只装 brief.SourceIDs 引用的冻结来源；候选来源 URL 预登记为可
	// 请求目标（fetch_page 只允许候选来源 URL 或本 run 搜索登记的 URL）。
	for _, id := range brief.SourceIDs {
		for i := range material.Sources {
			if material.Sources[i].SourceID == id {
				src := &material.Sources[i]
				rt.sources[id] = src
				if u, err := normalizeSignalWebURL(src.URL); err == nil {
					rt.registered[u] = true
				}
			}
		}
	}
	// M2：无可读本地正文 → 缺口可见（spec「先核前提再补证」：不死锁、记
	// gap；bindLedger 接线时落账）。
	if len(rt.sources) == 0 {
		rt.missingLocalGap = "本候选没有可读的冻结原文（依据切片无关联存文或原文为空）：先读门放行外搜/数值取证，缺口如实记录"
	}
	return rt, nil
}

// authorizesTool 报告 question 协议是否授权该动作（授权≠放行：先读门与
// 预算另由 gateTool 把守）。
func (q *signalQuestionRuntime) authorizesTool(tool string) bool {
	return isSignalQuestionTool(tool)
}

// hasReadableLocal 报告白名单是否存在至少一份可读本地正文。
func (q *signalQuestionRuntime) hasReadableLocal() bool {
	return len(q.sources) > 0
}

// localReadDone 报告是否已成功读取过至少一份本地正文。
func (q *signalQuestionRuntime) localReadDone() bool { return len(q.readOK) > 0 }

// gateTool 是先读门：存在可读本地来源且尚未成功读取一份时，外搜/数值源/
// calculate 一律拦截（先核前提再补证）；read_source 永不拦。无存文时不死
// 锁（放行，缺口由调用方记录）。
func (q *signalQuestionRuntime) gateTool(tool string) error {
	if q == nil || tool == "read_source" {
		return nil
	}
	if q.hasReadableLocal() && !q.localReadDone() {
		return fmt.Errorf("先读门：存在 %d 份冻结原文，请先 read_source 核实前提（至少成功读取一份），再进行外搜/取数/计算", len(q.sources))
	}
	return nil
}

// readSource 读取一份冻结本地来源：只认本候选白名单（brief.SourceIDs），
// 返回冻结片段；成功即解除先读门并把证据写入账本（n 前缀 kind=
// local_article）。重复读取由共享循环的 dedup 拦截（同参数不执行）。
func (q *signalQuestionRuntime) readSource(id string) (string, error) {
	src, ok := q.sources[id]
	if !ok {
		return "", fmt.Errorf("read_source：source_id %q 不在本候选冻结白名单内（白名单：%s）", id, strings.Join(q.brief.SourceIDs, "、"))
	}
	// 空正文不开门、不进证据宇宙（H3 防御位：freeze 已拒，此处兜底）。
	if strings.TrimSpace(src.Text) == "" {
		return "", fmt.Errorf("read_source：来源 %s 正文为空（原文不可用，已记缺口；请改读其它来源或基于缺口继续）", id)
	}
	q.readOK[id] = true
	if q.ledger != nil {
		q.ledger.addSourceRecord(&signalResearchSourceRecord{
			ID:            src.SourceID,
			Kind:          signalSourceKindLocal,
			Title:         src.Title,
			URL:           src.URL,
			PublishedAt:   src.PublishedAt,
			PublishedFrom: "local:frozen",
			CapturedAt:    &src.CapturedAt,
			Text:          src.Text,
			Hash:          src.ContentHash,
			Truncated:     src.Truncated,
			OriginalRunes: src.OriginalRunes,
			TemporalNote:  src.TemporalNote,
		})
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "来源 %s：%s", src.SourceID, src.Title)
	if src.URL != "" {
		sb.WriteString("\nURL：" + src.URL)
	}
	if src.PublishedAt != nil {
		sb.WriteString("\n发布时间：" + src.PublishedAt.In(src.CapturedAt.Location()).Format("2006-01-02"))
	} else {
		sb.WriteString("\n发布时间：未知")
	}
	sb.WriteString("\n截取说明：冻结片段 " + fmt.Sprint(len([]rune(src.Text))) + " 字符")
	if src.Truncated {
		fmt.Fprintf(&sb, "（原文 %d 字符，已截取）", src.OriginalRunes)
	}
	if src.TemporalNote != "" {
		sb.WriteString("；" + src.TemporalNote)
	}
	sb.WriteString("\n\n正文：\n" + src.Text)
	return sb.String(), nil
}

// registerSearchHits 把一次搜索的命中登记进 URL 白名单（去掉 userinfo/
// fragment、host 归一、保留 query；不因归一化混合不同文档——查询串原样）。
// 搜索摘要只作导航：不产生任何可引用证据。
func (q *signalQuestionRuntime) registerSearchHits(hits []WebSearchResult) {
	q.searchHits = hits
	for _, h := range hits {
		if u, err := normalizeSignalWebURL(h.URL); err == nil {
			q.registered[u] = true
		}
	}
}

// webSearch 执行一次受限外搜：query 校验（公开主题，1～300rune 且含字母/
// 数字）→ 3 次预算（失败也计）→ 后端执行。命中只作导航登记；不上传原文/
// 设置/凭证，query 不含内部 ID。先读门由 policy CheckCall 统一把守。
func (q *signalQuestionRuntime) webSearch(ctx context.Context, query string) (string, error) {
	qTrimmed, err := validateSignalSearchQuery(query)
	if err != nil {
		return "", err
	}
	if q.searchesUsed >= SignalQuestionMaxWebSearches {
		return "", fmt.Errorf("外搜预算已用尽（%d/%d 次，含失败）；请基于已有证据继续或 finish", q.searchesUsed, SignalQuestionMaxWebSearches)
	}
	q.searchesUsed++
	recordCall := func(status, errMsg string) {
		if q.ledger != nil {
			q.ledger.recordTextCall("web_search", "query", qTrimmed, status, errMsg)
		}
	}
	if q.searcher == nil {
		gap := "web_search 未配置：外搜不可用，缺口如实记录，本地解释不受影响"
		recordCall("rejected", gap)
		if q.ledger != nil {
			q.ledger.appendGap(signalResearchGap{Tool: "web_search", Reason: gap})
		}
		return "", &SignalPageFetchError{Kind: signalPageGapNotConfigured, Detail: gap}
	}
	hits, serr := q.searcher.Search(ctx, qTrimmed)
	if serr != nil {
		recordCall("error", serr.Error())
		if q.ledger != nil {
			q.ledger.appendGap(signalResearchGap{Tool: "web_search", Reason: "外搜失败：" + serr.Error()})
		}
		return "", &SignalPageFetchError{Kind: signalPageGapFetchFailed, Detail: serr.Error()}
	}
	// ok 只在真实成功后记（M1：失败路径不再出现 ok+error 双行）。
	recordCall("ok", "")
	q.registerSearchHits(hits)
	var sb strings.Builder
	fmt.Fprintf(&sb, "搜索 %q 命中 %d 条（仅导航线索，摘要不是正文证据；可用 fetch_page 读取命中 URL）：\n", qTrimmed, len(hits))
	for i, h := range hits {
		fmt.Fprintf(&sb, "%d. %s\n   %s\n   摘要（不可引用）：%s\n", i+1, h.Title, h.URL, h.Snippet)
	}
	return sb.String(), nil
}

// fetchPage 执行一次受限网页读取：URL 必须已登记（候选来源 URL 或本 run
// 搜索登记）、先读门通过、5 次预算（失败也计）。正文经安全适配取回后按
// cutoff 判定日期：合格才进入事实材料并产生 wN 证据；晚于 cutoff/未知日期
// 只返回 URL 与 typed 缺口，不把正文交给研究模型。
func (q *signalQuestionRuntime) fetchPage(ctx context.Context, rawURL string) (string, error) {
	norm, err := normalizeSignalWebURL(rawURL)
	if err != nil {
		return "", fmt.Errorf("fetch_page：%v", err)
	}
	if !q.registered[norm] {
		return "", fmt.Errorf("fetch_page：URL %s 未登记（只有候选来源 URL 或本次搜索登记的 URL 可请求，不抓网页内链接）", norm)
	}
	if q.pagesUsed >= SignalQuestionMaxWebPages {
		return "", fmt.Errorf("网页读取预算已用尽（%d/%d 次，含失败）；请基于已有证据继续或 finish", q.pagesUsed, SignalQuestionMaxWebPages)
	}
	q.pagesUsed++
	if q.fetcher == nil {
		gap := "fetch_page 未配置：网页读取不可用，缺口如实记录"
		if q.ledger != nil {
			q.ledger.recordTextCall("fetch_page", "", norm, "error", "not configured")
			q.ledger.appendGap(signalResearchGap{Tool: "fetch_page", Reason: gap})
		}
		return "", &SignalPageFetchError{Kind: signalPageGapNotConfigured, Detail: gap}
	}
	res, ferr := q.fetcher.FetchPage(ctx, norm)
	if ferr != nil {
		if q.ledger != nil {
			q.ledger.recordTextCall("fetch_page", "", norm, "error", ferr.Error())
			q.ledger.appendGap(signalResearchGap{Tool: "fetch_page", Reason: "网页读取失败：" + ferr.Error()})
		}
		return "", ferr
	}
	if !signalUsableWebText(res.Text) {
		gap := "网页正文为空"
		if q.ledger != nil {
			q.ledger.recordTextCall("fetch_page", "", norm, "error", "empty body")
			q.ledger.appendGap(signalResearchGap{Tool: "fetch_page", Reason: gap + "（URL 保留，正文不可用）"})
		}
		return "", &SignalPageFetchError{Kind: signalPageGapEmptyBody, Detail: gap}
	}
	// 时间边界（D5）：机器元数据日期取更晚者判边界；未知/晚于 cutoff 的正文
	// 不进入事实材料，只留 URL 与拒绝原因（取回时刻与事后核查如实标注）。
	fetchedAt := q.now()
	effective := signalWebEvidenceTime(res.PublishedAt, res.ModifiedAt)
	if effective == nil {
		gap := fmt.Sprintf("网页 %s 无可验证发布/更新日期：正文不进入事实材料（不猜日期），仅记录 URL 与取回时间 %s", res.FinalURL, fetchedAt.UTC().Format(time.RFC3339))
		if q.ledger != nil {
			q.ledger.recordTextCall("fetch_page", "", norm, "rejected", "unknown date")
			q.ledger.appendGap(signalResearchGap{Tool: "fetch_page", Reason: gap})
		}
		return "", &SignalPageFetchError{Kind: signalPageGapUnknownDate, Detail: gap}
	}
	if effective.After(q.material.Cutoff) {
		gap := fmt.Sprintf("网页 %s 声明日期 %s 晚于研究截止 %s：正文不进入事实材料（事后材料，不冒充当时已知）", res.FinalURL, effective.Format(time.RFC3339), q.material.Cutoff.Format(time.RFC3339))
		if q.ledger != nil {
			q.ledger.recordTextCall("fetch_page", "", norm, "rejected", "after cutoff")
			q.ledger.appendGap(signalResearchGap{Tool: "fetch_page", Reason: gap})
		}
		return "", &SignalPageFetchError{Kind: signalPageGapLateDate, Detail: gap}
	}
	// 合格：wN 证据入账本（可引用），正文进模型历史。
	q.webSeq++
	id := fmt.Sprintf("w%d", q.webSeq)
	note := "外部网页：事后核查材料（取回时间 " + fetchedAt.UTC().Format(time.RFC3339) + "，声明发布时间以来源为准）"
	rec := &signalResearchSourceRecord{
		ID:            id,
		Kind:          signalSourceKindWeb,
		Title:         res.Title,
		URL:           norm,
		FinalURL:      res.FinalURL,
		PublishedAt:   res.PublishedAt,
		PublishedFrom: res.PublishedFrom,
		FetchedAt:     &fetchedAt,
		Text:          res.Text,
		Hash:          res.ContentHash,
		Truncated:     res.Truncated,
		OriginalRunes: res.OriginalRunes,
		TemporalNote:  note,
	}
	if res.ModifiedAt != nil {
		rec.TemporalNote += fmt.Sprintf("；含更新时间 %s（取更晚者判界）", res.ModifiedAt.Format(time.RFC3339))
	}
	if q.ledger != nil {
		q.ledger.addSourceRecord(rec)
		q.ledger.recordTextCall("fetch_page", "", norm, "ok", "")
	}
	q.webSources[id] = rec
	var sb strings.Builder
	fmt.Fprintf(&sb, "来源 %s：%s\nURL：%s\n声明发布时间：%s（出处 %s）\n取回时间：%s（事后核查）\n截取说明：正文 %d 字符", id, res.Title, res.FinalURL, effective.Format(time.RFC3339), res.PublishedFrom, fetchedAt.UTC().Format(time.RFC3339), len([]rune(res.Text)))
	if res.Truncated {
		fmt.Fprintf(&sb, "（原文 %d 字符，已截取）", res.OriginalRunes)
	}
	sb.WriteString("\n\n正文：\n" + res.Text)
	return sb.String(), nil
}

// sourceCatalog 渲染研究 prompt 的来源目录：只含 ID/标题/URL/日期/截取说
// 明，不含正文（D3：不提前注入 sources 正文，不泄漏其它候选专属原文）。
func (q *signalQuestionRuntime) sourceCatalog() string {
	var sb strings.Builder
	if len(q.sources) == 0 {
		sb.WriteString("（本候选无可读的冻结原文；无法核实前提时如实记缺口，可定向补证）")
		return sb.String()
	}
	fmt.Fprintf(&sb, "可读来源 %d 份（read_source 白名单；正文经 read_source 进入研究，此处只列目录）：\n", len(q.sources))
	for _, id := range q.brief.SourceIDs {
		src, ok := q.sources[id]
		if !ok {
			continue
		}
		date := "未知"
		if src.PublishedAt != nil {
			date = src.PublishedAt.Format("2006-01-02")
		}
		trunc := ""
		if src.Truncated {
			trunc = fmt.Sprintf("（原文 %d 字符已截取）", src.OriginalRunes)
		}
		fmt.Fprintf(&sb, "- %s《%s》%s 发布:%s 片段%d字%s\n", src.SourceID, src.Title, src.URL, date, len([]rune(src.Text)), trunc)
	}
	return sb.String()
}

// bindLedger 绑定研究账本（Evidence 记录与 text call 流水写入处）。
func (q *signalQuestionRuntime) bindLedger(l *signalResearchLedger) {
	q.ledger = l
	if l != nil && q.missingLocalGap != "" {
		l.appendGap(signalResearchGap{Tool: "read_source", Reason: q.missingLocalGap})
	}
}

// questionPromptSection 渲染 question-v1 的研究 prompt 增量段（目录 + 工具
// 纪律），由 assembleSignalResearchPrompt 追加（材料 marshal 时剥离 Sources
// 正文，见 signalQuestionMaterialView）。
func (q *signalQuestionRuntime) questionPromptSection() string {
	var sb strings.Builder
	sb.WriteString("\n---\n冻结原文目录：\n")
	sb.WriteString(q.sourceCatalog())
	sb.WriteString(`
来源工具纪律（question-v1）：
1. read_source(source_id)：只读上表白名单；先读门——存在可读原文时，必须先成功 read_source 至少一份核实前提，才能外搜/取数/计算。
2. web_search(query)：公开主题短查询（≤300字）；最多 ` + fmt.Sprint(SignalQuestionMaxWebSearches) + ` 次（含失败）；结果只作导航，摘要不是正文证据。
3. fetch_page(url)：只可请求候选来源 URL 或本次搜索登记的 URL，最多 ` + fmt.Sprint(SignalQuestionMaxWebPages) + ` 次（含失败）；正文经安全取回并按研究截止时间核验，日期不明或晚于截止的网页不会进入正文证据。
4. 引用文字证据用 [[source:n1]] / [[source:w1]]（成文时）；文字 ID 不能进 calculate。
`)
	return sb.String()
}

// signalQuestionMaterialView 剥离材料中的冻结正文（研究 prompt 只携带目录，
// 不 marshal 整个含正文快照——D3 结构保证）。
func signalQuestionMaterialView(material *SignalMaterial) SignalMaterial {
	view := *material
	view.Sources = nil
	return view
}

// hashSignalText 统一 sha256 hex（冻结来源与网页证据共用）。
func hashSignalText(text string) string {
	digest := sha256.Sum256([]byte(text))
	return hex.EncodeToString(digest[:])
}
