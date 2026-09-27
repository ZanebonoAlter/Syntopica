package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/platform/airouter"
)

// ── Signal detect（board-signal-reports design §3，tasks 2.3）─────────────────
//
// detect 是发现闭环里唯一的 LLM 步骤：输入阶段1 装配的 SignalMaterial，输出
// 校验后的信号候选。三条硬契约：
//   ① 空材料（IsEmpty）直接零候选，绝不调 LLM（PC-4，安静空批次）；
//   ② LLM 输出严格 schema（字段非空 / score 1~10 整数 / 证据引用仅限本次
//      材料切片白名单），任何违例整响应作废，最多重试 1 次后返回错误——
//      不吞成空数组（job 转 failed，不伪装零信号）；
//   ③ 发现全程零取数、零计算、零成文：detector 不持有 tool registry，
//      没有可漂移的工具面（SD 断言：发现取数调用=0）。

// GenerateSignalDiscoverySessionID mints the discovery session id
// (board-signal-reports design §9: 发现独立 session，格式对齐
// generateBoardSessionID 的 data_enrichment_board_{id}_{hex8} 风格).
func GenerateSignalDiscoverySessionID(boardID uint) string {
	return fmt.Sprintf("board_signal_discovery_%d_%s", boardID, RandomHex(8))
}

// signalDetectMaxAttempts: 首次 + 1 次重试（design §3：最多重试 1 次）.
const signalDetectMaxAttempts = 2

// signalDetectScoreThreshold: 达标门槛，score ≥ 6 才成为候选（design §3）.
const signalDetectScoreThreshold = 6

// Discovery pipeline stages. prepare/detect 同时是 job status 的 phase 值；
// save 不是 phase（design §7 枚举只有 prepare|detect|research|compose），
// 只作为 error_stage 出现。
const (
	SignalStagePrepare = "prepare"
	SignalStageDetect  = "detect"
	SignalStageSave    = "save"
)

// SignalStageError marks which pipeline stage a discovery run failed in; the
// handler maps Stage into the job status's error_stage (JB-1: 检测故障可见,
// 不伪装成零信号).
type SignalStageError struct {
	Stage string
	Err   error
}

func (e *SignalStageError) Error() string { return "signal " + e.Stage + ": " + e.Err.Error() }
func (e *SignalStageError) Unwrap() error { return e.Err }

// SignalDetection is one validated detect output row (design §3 contract:
// 非空字段 + 白名单证据 + 达标分数). Evidence refs are normalized to the
// material slice ids they cite.
type SignalDetection struct {
	Signal           string
	WhyItMatters     string
	ResearchQuestion string
	EvidenceRefs     []string
	Score            int
	Rationale        string
}

// SignalDetector runs the detect LLM step. Deliberately holds NO tool
// registry: discovery never fetches data, calculates or composes.
type SignalDetector struct {
	airouter   AirRouter
	capability airouter.Capability
	// sourceCapability is the OPTIONAL four-source capability block (tasks
	// 3.7 发现半边), injected by dataenrichment/wire.go — this package must
	// not import datasources/wiring. Empty = legacy prompt byte-identically.
	sourceCapability string
}

// NewSignalDetector builds the detector; capability follows the existing
// board-analysis pattern (data_enrichment_analysis route).
func NewSignalDetector(router AirRouter, capability airouter.Capability) *SignalDetector {
	return &SignalDetector{airouter: router, capability: capability}
}

// SetSourceCapabilityText attaches the optional four-source capability block
// post-construction (SetFreshnessRefresher 先例：可选 setter，生产接线在
// wire.go)。空文本 = 旧行为，system prompt 与旧版字节一致。
func (d *SignalDetector) SetSourceCapabilityText(text string) { d.sourceCapability = text }

// detectSystemPrompt assembles the detect system prompt: the frozen base plus
// the optional source-capability block (single assembly point so the prompt
// contract is testable without an LLM).
func (d *SignalDetector) detectSystemPrompt() string {
	if d.sourceCapability == "" {
		return signalDetectSystemPrompt
	}
	return signalDetectSystemPrompt + "\n\n" + d.sourceCapability
}

const signalDetectSystemPrompt = `你是一位新闻板块的信号侦察员。输入是某板块一个时间周期内的冻结材料（各泳道的周期切片与摘要）。你的任务不是罗列内容，而是找出会改变对该板块未来判断的异动信号：突变、反常、初现、背离。

评分纪律（score 为 1~10 的整数）：按异动强度、报道密度、判断价值打分；只有 ≥6 分的信号才值得成为候选。没有达标信号是正常结果，返回空数组即可，不要硬凑。

输出严格 JSON（不要 markdown 包裹、不要任何其他内容）：
{"signals":[{"signal":"一句话说清异常本身","why_it_matters":"为什么值得查：它会怎样改变判断","research_question":"深入分析要回答的研究问题","evidence_refs":["材料切片ID"],"score":7,"rationale":"评分理由（对照三项评分维度）"}]}

纪律：
- evidence_refs 只能原样引用输入材料里出现的切片 ID，禁止编造或引用材料之外的编号
- 每条信号必须至少引用一条真实存在的切片作为依据
- 不要输出 JSON 以外的任何内容`

// Detect executes one detect run (≤2 LLM attempts). Empty material short-
// circuits to zero candidates without any LLM call (PC-4).
func (d *SignalDetector) Detect(ctx context.Context, material *SignalMaterial, sessionID string) ([]SignalDetection, error) {
	if material == nil || material.IsEmpty() {
		return nil, nil
	}
	whitelist := signalEvidenceWhitelist(material)
	// 防御① /no_think 前缀（DB enable_thinking=false 仍是主防线，与
	// runToolLoop / relation_scout 同款双保险）。
	userMsg := "/no_think\n" + assembleSignalDetectPrompt(material)
	var lastErr error
	for attempt := 1; attempt <= signalDetectMaxAttempts; attempt++ {
		resp, err := d.airouter.Chat(ctx, airouter.ChatRequest{
			Capability:  d.capability,
			Operation:   "data_enrichment.signal_detect",
			SessionID:   sessionID,
			Messages:    []airouter.Message{{Role: "system", Content: d.detectSystemPrompt()}, {Role: "user", Content: userMsg}},
			Temperature: floatPtr(0.2),
			JSONMode:    true,
		})
		if err != nil {
			lastErr = fmt.Errorf("detect chat attempt %d: %w", attempt, err)
			continue
		}
		detections, perr := parseSignalDetections(resp.Content, whitelist)
		if perr == nil {
			return detections, nil
		}
		lastErr = fmt.Errorf("detect attempt %d: %w", attempt, perr)
	}
	return nil, lastErr
}

// assembleSignalDetectPrompt builds the user prompt (single assembly point so
// prompt contract is testable without an LLM).
func assembleSignalDetectPrompt(material *SignalMaterial) string {
	// json.Marshal of SignalMaterial cannot fail (plain data + time.Time).
	materialJSON, _ := json.Marshal(material)
	return "板块周期材料（JSON，artifacts 内 section_id 即切片 ID）：\n" +
		string(materialJSON) +
		"\n\n请找出达标信号（score ≥ 6），evidence_refs 只引用上面的切片 ID，输出 JSON。"
}

// signalEvidenceWhitelist collects every slice id of this material — the ONLY
// legal evidence universe for this discovery run (design §3: 悬空/跨板块/
// 周期外引用剔除；材料即边界).
func signalEvidenceWhitelist(material *SignalMaterial) map[string]bool {
	whitelist := make(map[string]bool)
	for _, lane := range material.Lanes {
		for _, article := range lane.Articles {
			whitelist[strconv.FormatUint(uint64(article.SectionID), 10)] = true
		}
	}
	return whitelist
}

// parseSignalDetections validates one LLM response against the strict schema.
// ANY violation (missing fields, non-integer/out-of-range score, empty
// evidence array) invalidates the WHOLE response so the caller retries —
// a model that broke the numeric contract once must not be partially trusted.
// After schema validation, out-of-whitelist refs are dropped per signal and
// signals left without any valid evidence are discarded (高分不绕过, SD-4).
func parseSignalDetections(content string, whitelist map[string]bool) ([]SignalDetection, error) {
	parsed, err := ParseJSONResponse(content)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	rawSignals, ok := parsed["signals"].([]any)
	if !ok {
		return nil, fmt.Errorf("missing or invalid 'signals' array")
	}
	out := make([]SignalDetection, 0, len(rawSignals))
	for i, raw := range rawSignals {
		m, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("signals[%d] is not an object", i)
		}
		detection, err := validateSignalDetection(m, i)
		if err != nil {
			return nil, err
		}
		refs := make([]string, 0, len(detection.EvidenceRefs))
		seen := make(map[string]bool, len(detection.EvidenceRefs))
		for _, ref := range detection.EvidenceRefs {
			ref = strings.TrimSpace(ref)
			if ref == "" || seen[ref] || !whitelist[ref] {
				continue
			}
			seen[ref] = true
			refs = append(refs, ref)
		}
		if len(refs) == 0 {
			continue // 白名单过滤后无有效依据：丢弃该信号，不看分数
		}
		detection.EvidenceRefs = refs
		if detection.Score < signalDetectScoreThreshold {
			continue // 未达门槛
		}
		out = append(out, detection)
	}
	return out, nil
}

// validateSignalDetection enforces the per-signal schema: four non-empty text
// fields, integer score 1..10, non-empty evidence_refs array of non-empty
// strings.
func validateSignalDetection(m map[string]any, index int) (SignalDetection, error) {
	invalid := func(reason string) (SignalDetection, error) {
		return SignalDetection{}, fmt.Errorf("signals[%d] %s", index, reason)
	}
	nonEmpty := func(key string) (string, error) {
		v, _ := m[key].(string)
		if strings.TrimSpace(v) == "" {
			if _, err := invalid("missing or empty " + key); err != nil {
				return "", err
			}
		}
		return v, nil
	}
	signal, err := nonEmpty("signal")
	if err != nil {
		return SignalDetection{}, err
	}
	why, err := nonEmpty("why_it_matters")
	if err != nil {
		return SignalDetection{}, err
	}
	question, err := nonEmpty("research_question")
	if err != nil {
		return SignalDetection{}, err
	}
	rationale, err := nonEmpty("rationale")
	if err != nil {
		return SignalDetection{}, err
	}
	scoreFloat, ok := m["score"].(float64)
	if !ok || scoreFloat != math.Trunc(scoreFloat) || scoreFloat < 1 || scoreFloat > 10 {
		return invalid("score must be an integer 1..10")
	}
	rawRefs, ok := m["evidence_refs"].([]any)
	if !ok || len(rawRefs) == 0 {
		return invalid("requires a non-empty evidence_refs array")
	}
	refs := make([]string, 0, len(rawRefs))
	for _, r := range rawRefs {
		s, ok := r.(string)
		if !ok || strings.TrimSpace(s) == "" {
			return invalid("evidence_refs must be non-empty strings")
		}
		refs = append(refs, s)
	}
	return SignalDetection{
		Signal:           signal,
		WhyItMatters:     why,
		ResearchQuestion: question,
		EvidenceRefs:     refs,
		Score:            int(scoreFloat),
		Rationale:        rationale,
	}, nil
}

// SignalDetectionsToCandidates maps validated detections to candidate rows for
// repository.CreateSignalDiscoveryBatch (the repository stamps owner/period
// columns and runs within-batch dedupe inside the transaction).
func SignalDetectionsToCandidates(detections []SignalDetection) ([]*repository.BoardSignalCandidate, error) {
	out := make([]*repository.BoardSignalCandidate, 0, len(detections))
	for i := range detections {
		refs, err := json.Marshal(detections[i].EvidenceRefs)
		if err != nil {
			return nil, fmt.Errorf("marshal evidence refs: %w", err)
		}
		out = append(out, &repository.BoardSignalCandidate{
			Signal:           detections[i].Signal,
			WhyItMatters:     detections[i].WhyItMatters,
			ResearchQuestion: detections[i].ResearchQuestion,
			EvidenceRefs:     refs,
			Score:            detections[i].Score,
			Rationale:        detections[i].Rationale,
		})
	}
	return out, nil
}

// ── Discovery run（prepare → detect → 原子保存，tasks 2.4 job fn 的服务侧）────

// SignalDiscoveryOutcome is the handler-facing result of one discovery run.
// CandidateCount==0 is the quiet no_signal outcome.
type SignalDiscoveryOutcome struct {
	DiscoveryID    uint
	CandidateCount int
	SessionID      string
}

// SignalDiscoveryService executes one manual discovery run: period material
// assembly (prepare) → detect (≤2 attempts) → atomic batch save. Like the
// detector it holds no tool registry: discovery makes zero data-source calls,
// zero calculations and zero compose steps.
type SignalDiscoveryService struct {
	detector *SignalDetector
	builder  *SignalMaterialBuilder
	repo     *repository.Repository
	now      func() time.Time
}

// NewSignalDiscoveryService wires the discovery run (wire.go capability 登记
// 沿用 board_analysis 模式).
func NewSignalDiscoveryService(router AirRouter, capability airouter.Capability, builder *SignalMaterialBuilder, repo *repository.Repository) *SignalDiscoveryService {
	return &SignalDiscoveryService{
		detector: NewSignalDetector(router, capability),
		builder:  builder,
		repo:     repo,
		now:      time.Now,
	}
}

// SetSourceCapabilityText forwards the optional capability block to the
// detector (外部注入通道：wire.go 取 wiring.SourceCapabilityText() 调用此处；
// 空文本 = 旧行为)。
func (s *SignalDiscoveryService) SetSourceCapabilityText(text string) {
	if s != nil && s.detector != nil {
		s.detector.SetSourceCapabilityText(text)
	}
}

// DiscoverSignals runs prepare → detect → save. The hook (nil legal) receives
// PHASE transitions only (prepare/detect — the design §7 phase enum; save is
// not a phase) so the handler can publish live job progress; failures carry a
// *SignalStageError whose Stage feeds the job's error_stage (save included).
func (s *SignalDiscoveryService) DiscoverSignals(ctx context.Context, boardID uint, granularity, period string, hook func(stage string)) (*SignalDiscoveryOutcome, error) {
	report := func(stage string) {
		if hook != nil {
			hook(stage)
		}
	}
	report(SignalStagePrepare)
	p, err := ParseSignalPeriod(granularity, period, s.now())
	if err != nil {
		return nil, &SignalStageError{Stage: SignalStagePrepare, Err: err}
	}
	cutoff := SignalCutoff(p, s.now())
	material, err := s.builder.AssembleSignalMaterial(ctx, boardID, p, cutoff)
	if err != nil {
		return nil, &SignalStageError{Stage: SignalStagePrepare, Err: err}
	}

	sessionID := GenerateSignalDiscoverySessionID(boardID)
	report(SignalStageDetect)
	// 空材料时 Detect 内部短路零候选，不产生任何 LLM 调用（PC-4）。
	detections, err := s.detector.Detect(ctx, material, sessionID)
	if err != nil {
		return nil, &SignalStageError{Stage: SignalStageDetect, Err: err}
	}

	// save 不报 phase：phase 枚举只有 prepare|detect|research|compose；
	// save 失败经 SignalStageError 落 error_stage。
	snapshot, err := json.Marshal(material)
	if err != nil {
		return nil, &SignalStageError{Stage: SignalStageSave, Err: fmt.Errorf("marshal input snapshot: %w", err)}
	}
	candidates, err := SignalDetectionsToCandidates(detections)
	if err != nil {
		return nil, &SignalStageError{Stage: SignalStageSave, Err: err}
	}
	batch := &repository.BoardSignalDiscovery{
		SemanticBoardID: boardID,
		Granularity:     p.Granularity,
		Period:          p.Period,
		AnalysisMode:    material.AnalysisMode,
		Cutoff:          cutoff,
		InputSnapshot:   snapshot,
		SessionID:       sessionID,
	}
	saved, _, err := s.repo.CreateSignalDiscoveryBatch(ctx, batch, candidates)
	if err != nil {
		return nil, &SignalStageError{Stage: SignalStageSave, Err: err}
	}
	return &SignalDiscoveryOutcome{
		DiscoveryID:    batch.ID,
		CandidateCount: len(saved),
		SessionID:      sessionID,
	}, nil
}
