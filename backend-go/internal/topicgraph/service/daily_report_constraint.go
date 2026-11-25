package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/topicgraph/repository"
)

// 话题辅助标签约束（topic-lane-auxiliary-label-constraint）。
//
// 本文件只做两件事，都是可单测的纯/近纯逻辑：
//   - 约束锤向量的文本编码与计算（tasks 2.1~2.3，design D3/D8）；
//   - 约束覆盖判定（tasks 3.1~3.7，design D2/D7）——**不碰数据库**。
//
// 闸门接入（lane group 级）在 daily_report_lane.go；判定输入（section 的有效
// tag 集合 / tag 向量与模型标识）由 repository 提供，此处只消费。

// LaneConstraintVerdict is the three-state outcome of one coverage judgement
// (tasks 3.7). Only Violated blocks an anchor; Undecided means "无法判定" and
// SHALL NOT 参与拦截（spec「约束判定对不可判情况一律放行」）。
type LaneConstraintVerdict string

const (
	// LaneConstraintSatisfied — every required constraint is covered.
	LaneConstraintSatisfied LaneConstraintVerdict = "satisfied"
	// LaneConstraintViolated — at least one required constraint is decided
	// uncovered; the section SHALL NOT anchor the topic.
	LaneConstraintViolated LaneConstraintVerdict = "violated"
	// LaneConstraintUndecided — nothing violated, but at least one required
	// constraint could not be judged (cross-space tags / degenerate single
	// constraint without an absolute ceiling). Passes the gate.
	LaneConstraintUndecided LaneConstraintVerdict = "undecided"
)

// LaneConstraintCoverage is the per-constraint detail of one judgement. The
// verdict is the aggregate; the term lists feed the L2 rework prompt so the
// LLM sees which legs are missing (tasks 4.3「候选含各话题的约束状态」).
type LaneConstraintCoverage struct {
	Verdict LaneConstraintVerdict
	// Required is the number of required constraints on the topic.
	Required int
	// Covered / Uncovered / Undecided carry the terms of the required
	// constraints, partitioned by outcome.
	Covered   []string
	Uncovered []string
	Undecided []string
}

// Blocks reports whether the coverage forbids anchoring the topic.
func (c LaneConstraintCoverage) Blocks() bool { return c.Verdict == LaneConstraintViolated }

// laneConstraintAnchor is a constraint normalised for judgement: the anchor
// vector plus its *effective* margin / max_distance (row override, else the
// ai_settings global default — design D4).
type laneConstraintAnchor struct {
	term        string
	vec         []float64
	model       string
	margin      float64
	maxDistance float64
	// hasMaxDistance false = no absolute ceiling in play; a single-constraint
	// topic then degenerates to "无法判定"（task 3.4）.
	hasMaxDistance bool
}

// laneConstraintAnchorText builds the anchor text for a user-supplied short
// term: `term + " " + "event"`（design D3）.
//
// 为什么拼 `event`：锤与**被测对象**同构——被测的是 section 的 event 类标签，
// 而 tag 向量文本是 buildTagEmbeddingText 的 `label . description aliases
// category` 模板。本地空间实测分离度：V1_prod×event 64.4% > V1_prod×keyword
// 53.6% > V2_bare×keyword 46.8% > V1_prod×裸词 26.2%（裸词锤两锚间距仅 0.139，
// 判别力坦塌）。
func laneConstraintAnchorText(term string) string {
	return strings.TrimSpace(term) + " " + "event"
}

// EmbedFn is the embedding call signature the anchor computation depends on
// (airouter.Router.Embed). Declared as a func type so tests can stub it and
// assert the exact input text / model pass-through（tasks 2.2）.
type EmbedFn func(ctx context.Context, req airouter.EmbeddingRequest, capability airouter.Capability) (*airouter.EmbeddingResult, error)

// laneConstraintEmbedFn is the swappable embedding hook (default: airouter's
// current ordered-failover route — design D8，不在业务层重复实现顺位逻辑).
var laneConstraintEmbedFn EmbedFn = func(ctx context.Context, req airouter.EmbeddingRequest, capability airouter.Capability) (*airouter.EmbeddingResult, error) {
	return airouter.NewRouter().Embed(ctx, req, capability)
}

// ErrLaneConstraintAnchor is the sentinel for anchor computation failures
// (embedding unavailable / unparseable / dimension mismatch). The write path
// answers 5xx and MUST NOT persist a row (spec「计算失败不落库」).
var ErrLaneConstraintAnchor = errors.New("lane constraint anchor failed")

// LaneConstraintValidationError marks a client-side payload problem (400).
type LaneConstraintValidationError struct {
	Message string
}

func (e *LaneConstraintValidationError) Error() string { return e.Message }

func laneConstraintBadRequest(format string, args ...any) *LaneConstraintValidationError {
	return &LaneConstraintValidationError{Message: fmt.Sprintf(format, args...)}
}

// ComputeLaneConstraintAnchor computes one short term's anchor vector with the
// current embedding route and returns the model name reported by the provider
// (persisted as embedding_model so judgement can skip cross-space tags).
//
// 失败一律返回错误、SHALL NOT 返回零向量（spec「计算失败不落库」）：不可解析 /
// 空向量 / NaN / 模型名为空都归为失败——没有模型名的锤无法识别向量空间，留着
// 只会让判定默默失效。
func ComputeLaneConstraintAnchor(ctx context.Context, term string) (vector []float64, model string, err error) {
	text := laneConstraintAnchorText(term)
	result, err := laneConstraintEmbedFn(ctx, airouter.EmbeddingRequest{
		Input:     []string{text},
		Operation: "topicgraph.lane_constraint_anchor",
		SessionID: SessionIDFromContext(ctx),
		Metadata: map[string]any{
			"operation": "lane_constraint_anchor",
			"term":      term,
		},
	}, airouter.CapabilityEmbedding)
	if err != nil {
		return nil, "", fmt.Errorf("%w: embed %q: %v", ErrLaneConstraintAnchor, term, err)
	}
	if result == nil || len(result.Embeddings) == 0 || len(result.Embeddings[0]) == 0 {
		return nil, "", fmt.Errorf("%w: empty embedding for %q", ErrLaneConstraintAnchor, term)
	}
	vec := result.Embeddings[0]
	for i, v := range vec {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, "", fmt.Errorf("%w: non-finite component %d for %q", ErrLaneConstraintAnchor, i, term)
		}
	}
	if strings.TrimSpace(result.Model) == "" {
		return nil, "", fmt.Errorf("%w: provider returned no model name for %q", ErrLaneConstraintAnchor, term)
	}
	return vec, result.Model, nil
}

// laneConstraintEffectiveMargin resolves the relative-preference strictness:
// the row override when set, else the ai_settings global default (design D4).
func laneConstraintEffectiveMargin(margin *float64, cfg repository.PersistentTopicConfig) float64 {
	if margin != nil {
		return *margin
	}
	if cfg.ConstraintMarginDefault > 0 {
		return cfg.ConstraintMarginDefault
	}
	return repository.DefaultPersistentTopicConfig().ConstraintMarginDefault
}

// laneConstraintEffectiveMaxDistance resolves the absolute ceiling. The bool
// is false when neither the row nor the global default supplies one (a
// non-positive ceiling is meaningless: the condition is a strict "<" — so it
// means "no ceiling", which is what makes a single-constraint topic
// undecidable per task 3.4).
func laneConstraintEffectiveMaxDistance(maxDistance *float64, cfg repository.PersistentTopicConfig) (float64, bool) {
	if maxDistance != nil {
		if *maxDistance > 0 {
			return *maxDistance, true
		}
		return 0, false
	}
	if cfg.ConstraintMaxDistanceDefault > 0 {
		return cfg.ConstraintMaxDistanceDefault, true
	}
	return 0, false
}

// laneConstraintAnchors normalises the required constraints of one topic into
// judgement anchors, dropping the ones with an unusable anchor vector (an empty
// / unparseable vector cannot judge anything — the row is only reachable if the
// write path bypassed validation).
func laneConstraintAnchors(constraints []repository.TopicLaneConstraint, cfg repository.PersistentTopicConfig) []laneConstraintAnchor {
	anchors := make([]laneConstraintAnchor, 0, len(constraints))
	for _, c := range constraints {
		mode := c.Mode
		if mode == "" {
			mode = repository.LaneConstraintModeRequired
		}
		// preferred 约束 SHALL NOT 参与拦截判定（spec）：它既不拦截也不参与
		// 「相对偏好」的参照集——否则软约束会悄悄把硬门的判定口径收紧。
		if mode != repository.LaneConstraintModeRequired {
			continue
		}
		vec, err := parsePgVector(c.TermEmbedding)
		if err != nil || len(vec) == 0 {
			continue
		}
		margin := laneConstraintEffectiveMargin(c.Margin, cfg)
		maxDistance, hasMax := laneConstraintEffectiveMaxDistance(c.MaxDistance, cfg)
		anchors = append(anchors, laneConstraintAnchor{
			term:           c.Term,
			vec:            vec,
			model:          c.EmbeddingModel,
			margin:         margin,
			maxDistance:    maxDistance,
			hasMaxDistance: hasMax,
		})
	}
	return anchors
}

// hasRequiredConstraints reports whether any row is a required constraint
// (empty mode = the column default, required).
func hasRequiredConstraints(constraints []repository.TopicLaneConstraint) bool {
	for _, c := range constraints {
		mode := c.Mode
		if mode == "" {
			mode = repository.LaneConstraintModeRequired
		}
		if mode == repository.LaneConstraintModeRequired {
			return true
		}
	}
	return false
}

// EvaluateLaneConstraintCoverage judges whether a tag set covers every
// required constraint of one topic. Pure — no DB, no clock, no network
// （task 3.7）.
//
// 判定语义（spec「约束覆盖判定语义」，design D2）：
//   - 约束 i 被覆盖 ⟺ 存在 tag t 使 d(t,A_i) + margin_i < d(t,A_j)（∀ 其它
//     required 约束 j）；max_distance 生效时还要求 d(t,A_i) < max_distance_i；
//   - 多条 required SHALL 全部被覆盖（AND），且不同约束可由**不同 tag** 分腿
//     满足；
//   - 一个 tag 无法独自满足两条 required（它的距离向量只在一个锤上占优）；
//   - 跨向量空间的 tag 跳过（既不判满足也不判违反，design D7）；
//   - 无标签的 section 跳过检查（不判违反）；
//   - 单约束退化（无其它 required 锤可比）只看 max_distance，无上限则无法判定。
func EvaluateLaneConstraintCoverage(constraints []repository.TopicLaneConstraint, labels []repository.LaneLabelVector, cfg repository.PersistentTopicConfig) LaneConstraintCoverage {
	anchors := laneConstraintAnchors(constraints, cfg)
	out := LaneConstraintCoverage{
		Verdict:  LaneConstraintSatisfied,
		Required: len(anchors),
	}
	if len(anchors) == 0 {
		// 话题确实绑了 required 约束、但锤向量读不出来（脏行）：无法判定，放行。
		// 没有 required 约束（或全是 preferred）才是真正的「满足」。
		if hasRequiredConstraints(constraints) {
			out.Verdict = LaneConstraintUndecided
		}
		return out
	}
	if len(labels) == 0 {
		return out // 无标签 section → 跳过检查
	}

	anyUndecided := false
	for i := range anchors {
		anchor := anchors[i]
		// Comparable peers: the *other required* anchors in the same vector
		// space (different model / dimension cannot be compared at all).
		peers := make([]int, 0, len(anchors))
		for j := range anchors {
			if j == i {
				continue
			}
			if anchors[j].model != anchor.model || len(anchors[j].vec) != len(anchor.vec) {
				continue
			}
			peers = append(peers, j)
		}
		// Candidate tags: same vector space as this anchor, usable vector.
		type candidate struct {
			toAnchor   float64
			toPeerBest []float64
		}
		cands := make([]candidate, 0, len(labels))
		for _, label := range labels {
			if label.Model == "" || label.Model != anchor.model || len(label.Vector) != len(anchor.vec) {
				continue // 跨向量空间 → 跳过（不判满足也不判违反）
			}
			d := cosineFloats(label.Vector, anchor.vec)
			if !isFiniteDistance(d) {
				continue
			}
			peersD := make([]float64, len(peers))
			usable := true
			for k, j := range peers {
				pd := cosineFloats(label.Vector, anchors[j].vec)
				if !isFiniteDistance(pd) {
					usable = false
					break
				}
				peersD[k] = pd
			}
			if !usable {
				continue
			}
			cands = append(cands, candidate{toAnchor: d, toPeerBest: peersD})
		}
		if len(cands) == 0 {
			// 这个锤在场上找不到可比 tag（跨空间 / 空向量）：无法判定，不拦截。
			out.Undecided = append(out.Undecided, anchor.term)
			anyUndecided = true
			continue
		}
		if len(peers) == 0 {
			// 单约束退化（task 3.4）：相对偏好无参照，只看绝对上限。
			if !anchor.hasMaxDistance {
				out.Undecided = append(out.Undecided, anchor.term)
				anyUndecided = true
				continue
			}
			covered := false
			for _, c := range cands {
				if c.toAnchor < anchor.maxDistance {
					covered = true
					break
				}
			}
			if covered {
				out.Covered = append(out.Covered, anchor.term)
			} else {
				out.Uncovered = append(out.Uncovered, anchor.term)
				out.Verdict = LaneConstraintViolated
			}
			continue
		}
		covered := false
		for _, c := range cands {
			if anchor.hasMaxDistance && c.toAnchor >= anchor.maxDistance {
				continue // 绝对上限：离锤太远，相对偏好的优势不具判别意义
			}
			better := true
			for _, pd := range c.toPeerBest {
				// 严格不等式：恰好等于 margin 不算覆盖（task 3.1 的边界不含）。
				if !(c.toAnchor+anchor.margin < pd) {
					better = false
					break
				}
			}
			if better {
				covered = true
				break
			}
		}
		if covered {
			out.Covered = append(out.Covered, anchor.term)
		} else {
			out.Uncovered = append(out.Uncovered, anchor.term)
			out.Verdict = LaneConstraintViolated
		}
	}
	if out.Verdict == LaneConstraintViolated {
		return out
	}
	if anyUndecided {
		out.Verdict = LaneConstraintUndecided
	}
	return out
}

// isFiniteDistance guards against the MaxFloat64 sentinel cosineFloats returns
// for mismatched / zero-norm vectors.
func isFiniteDistance(d float64) bool {
	return !math.IsNaN(d) && !math.IsInf(d, 0) && d < math.MaxFloat64
}

// LaneConstraintRequest is one entry of the PUT body. The client supplies only
// the short term (and optionally mode / margin / max_distance) — SHALL NOT be
// asked for a description（spec「不要求用户补充描述」）.
type LaneConstraintRequest struct {
	Term        string   `json:"term"`
	Mode        string   `json:"mode"`
	Margin      *float64 `json:"margin"`
	MaxDistance *float64 `json:"max_distance"`
}

// ReplaceTopicLaneConstraints performs the full-replacement PUT workflow:
// normalise + validate the payload (400 on any invalid entry, no partial
// write), reuse stored anchors for unchanged terms, compute anchors for new
// terms with the current embedding route (5xx on failure, nothing written),
// then commit the whole set in one transaction.
func ReplaceTopicLaneConstraints(ctx context.Context, topicID uint, reqs []LaneConstraintRequest) ([]repository.TopicLaneConstraint, error) {
	inputs, err := laneConstraintInputsFromRequests(reqs)
	if err != nil {
		return nil, err
	}
	existing, err := repository.Repo.ListTopicLaneConstraints(topicID)
	if err != nil {
		return nil, err
	}
	stored := make(map[string]repository.TopicLaneConstraint, len(existing))
	for _, row := range existing {
		stored[row.Term] = row
	}

	for i := range inputs {
		in := &inputs[i]
		if row, ok := stored[in.Term]; ok && row.TermEmbedding != "" {
			// 短词未变 → 复用已落库的锤。锤是短词的纯函数，复用让「同一请求重提」
			// 幂等且不依赖 embedding 可用性，同时保住既有向量的空间一致性（远程
			// 降级到本地算出的锤不能与既有 tag 向量比对，必须人工重算才换空间）。
			in.TermEmbedding = row.TermEmbedding
			in.EmbeddingModel = row.EmbeddingModel
			continue
		}
		vec, model, aErr := ComputeLaneConstraintAnchor(ctx, in.Term)
		if aErr != nil {
			return nil, aErr
		}
		expectedDim, dErr := repository.Repo.SampleLiveTagEmbeddingDimension(model)
		if dErr != nil {
			logging.Warnf("daily-report: sample tag embedding dimension for %q failed (skipping dimension check): %v", model, dErr)
		} else if expectedDim > 0 && expectedDim != len(vec) {
			return nil, fmt.Errorf("%w: term %q produced a %d-dim vector but the %q space stores %d-dim vectors",
				ErrLaneConstraintAnchor, in.Term, len(vec), model, expectedDim)
		}
		in.TermEmbedding = repository.FloatsToPgVector(vec)
		in.EmbeddingModel = model
	}

	if err := repository.Repo.ReplaceTopicLaneConstraints(topicID, inputs); err != nil {
		return nil, err
	}
	return repository.Repo.ListTopicLaneConstraints(topicID)
}

// PrepareManualLaneConstraints resolves the constraints of a manual lane creation
// request **before** the create transaction.
//
// 三个要点（design D12）：
//  1. 校验复用 `laneConstraintInputsFromRequests`（与 PUT 端点同一真相源，规则不漂移）；
//  2. 锚向量在这里（事务外）算——embedding 是网络调用，不能进 DB 事务；
//  3. 维度按当前标签向量空间校对一次，与 PUT 端点同口径（换空间时不静默落库）。
//
// 无约束时返回 nil（存量调用零变化）。
func PrepareManualLaneConstraints(ctx context.Context, reqs []LaneConstraintRequest) ([]repository.ManualTopicConstraint, error) {
	if len(reqs) == 0 {
		return nil, nil
	}
	inputs, err := laneConstraintInputsFromRequests(reqs)
	if err != nil {
		return nil, err
	}
	out := make([]repository.ManualTopicConstraint, 0, len(inputs))
	for _, in := range inputs {
		vec, model, aErr := ComputeLaneConstraintAnchor(ctx, in.Term)
		if aErr != nil {
			return nil, aErr
		}
		expectedDim, dErr := repository.Repo.SampleLiveTagEmbeddingDimension(model)
		if dErr != nil {
			logging.Warnf("daily-report: sample tag embedding dimension for %q failed (skipping dimension check): %v", model, dErr)
		} else if expectedDim > 0 && expectedDim != len(vec) {
			return nil, fmt.Errorf("%w: term %q produced a %d-dim vector but the %q space stores %d-dim vectors",
				ErrLaneConstraintAnchor, in.Term, len(vec), model, expectedDim)
		}
		out = append(out, repository.ManualTopicConstraint{
			Term:        in.Term,
			Mode:        in.Mode,
			Anchor:      repository.FloatsToPgVector(vec),
			Model:       model,
			Margin:      in.Margin,
			MaxDistance: in.MaxDistance,
		})
	}
	return out, nil
}

// laneConstraintInputsFromRequests normalises the PUT payload into repository
// inputs and validates it (400 on failure). Validation is delegated to the
// repository's single source of truth so the API and the DB rules cannot drift.
func laneConstraintInputsFromRequests(reqs []LaneConstraintRequest) ([]repository.LaneConstraintInput, error) {
	inputs := make([]repository.LaneConstraintInput, 0, len(reqs))
	for i, req := range reqs {
		term := strings.TrimSpace(req.Term)
		if term == "" {
			return nil, laneConstraintBadRequest("constraint #%d: term is required", i+1)
		}
		mode := strings.TrimSpace(req.Mode)
		if mode == "" {
			mode = repository.LaneConstraintModeRequired
		}
		inputs = append(inputs, repository.LaneConstraintInput{
			Term:        term,
			Mode:        mode,
			Margin:      req.Margin,
			MaxDistance: req.MaxDistance,
		})
	}
	if err := repository.ValidateLaneConstraintInputs(inputs); err != nil {
		return nil, laneConstraintBadRequest("%s", err.Error())
	}
	return inputs, nil
}

// LoadLaneConstraintOption loads the gate context for one report run: the
// constraint rows of every anchorable topic plus the day's tag vectors (with
// their embedding model, resolved through article_topic_tags — NOT
// cluster_tag_ids, which is 92.8% dangling on historical sections).
//
// 任何一步失败都降级为「无约束」并记日志：约束不可读时回落到引入约束前的行为
// （不是「拦下全部」），因为读不到约束就等于不知道该拦什么；这与存量话题零约束
// 的路径完全一致，行为可预期。
func LoadLaneConstraintOption(topics []repository.BoardPersistentTopic, tagIDs []uint) LaneConstraintOption {
	if len(topics) == 0 || len(tagIDs) == 0 {
		return LaneConstraintOption{}
	}
	topicIDs := make([]uint, 0, len(topics))
	for _, t := range topics {
		topicIDs = append(topicIDs, t.ID)
	}
	constraints, err := repository.Repo.ListTopicLaneConstraintsByTopicIDs(topicIDs)
	if err != nil {
		logging.Warnf("daily-report: load lane constraints failed (gate disabled this run): %v", err)
		return LaneConstraintOption{}
	}
	if len(constraints) == 0 {
		return LaneConstraintOption{}
	}
	vectors, vErr := repository.Repo.ListLaneConstraintLabelVectors(tagIDs)
	if vErr != nil {
		logging.Warnf("daily-report: load lane constraint label vectors failed (gate disabled this run): %v", vErr)
		return LaneConstraintOption{}
	}
	return WithLaneConstraints(constraints, vectors)
}
