package main

// Mechanical scorer (design D12). Joins gold labels with run records AFTER
// the run; the runner never sees gold. Every metric is recomputable from the
// two inputs alone.
//
// Denominator discipline:
//   - the fixed TOTAL sample N (non-disputed) is THE denominator for
//     coverage / error rates — conditional accuracy never substitutes for it;
//   - 既有身份侧 empty subject_key → counted as 缺身份 (missing identity),
//     entity side no decision → counted as 未归一 (unresolved); neither may
//     masquerade as a correct singleton group;
//   - pairwise 误合/漏合/重复身份组率 are computed ONLY on the comparable
//     subset (both systems produced a prediction);
//   - any zero denominator → 「不适用（N=0）」, never 0% or 100%;
//   - technical failures ≠ 0 → the segment is BLOCKED (no verdict).

import (
	"fmt"
)

// GoldEntry is one gold label row, blind-annotated from the article text
// BEFORE the run by an annotator isolated from model outputs.
type GoldEntry struct {
	SampleID   string `json:"sample_id"`
	GoldEntity string `json:"gold_entity"` // annotator-assigned entity label
	Disputed   bool   `json:"disputed"`
}

// MentionRecord is one runner output row (eval artifact; the entity layer
// holds the same decision). Attempts/Reason are the ACTUALS the budget report
// aggregates and the error-example list needs — both are serialized with the
// records so the report is recomputable from artifacts alone.
type MentionRecord struct {
	SampleID    string `json:"sample_id"`
	Mention     string `json:"mention"`
	SubjectKey  string `json:"subject_key"`  // 既有身份侧预测（空 = 缺身份）
	EntityID    uint   `json:"entity_id"`    // 实体侧预测（0 = 无决策/未归一）
	Status      string `json:"status"`       // created | reused | idempotent | unresolved
	TechFailure bool   `json:"tech_failure"` // true → 该提及技术失败
	// Attempts = 第二轮实际尝试次数（0 = 未发生二判：无候选直接 none / 幂等）。
	Attempts int `json:"attempts"`
	// Reason = 技术失败原因 / 新建原因（合法 none / 池内无候选），错例清单用。
	Reason string `json:"reason,omitempty"`
}

// Metric carries a rate plus its raw numerator/denominator; Text renders
// 「不适用（N=0）」 instead of a misleading 0%/100%.
type Metric struct {
	Num  int     `json:"num"`
	Den  int     `json:"den"`
	Rate float64 `json:"rate"`
}

func (m Metric) Text() string {
	if m.Den == 0 {
		return "不适用（N=0）"
	}
	return fmt.Sprintf("%.4f (%d/%d)", m.Rate, m.Num, m.Den)
}

func rate(num, den int) Metric {
	if den == 0 {
		return Metric{Num: num, Den: den}
	}
	return Metric{Num: num, Den: den, Rate: float64(num) / float64(den)}
}

// srow is one scoring row: gold label + both systems' predictions.
type srow struct {
	sampleID string
	gold     string
	subject  string // 既有身份侧预测组（空 = 缺身份，不进可比子集）
	entity   string // 实体侧预测组（空 = 未归一，不进可比子集）
	reused   bool   // entity decision was reuse/idempotent (wrong-reuse pool)
}

// ScoreReport is the mechanical verdict.
type ScoreReport struct {
	TotalN          int    `json:"total_n"` // 固定总样本 N（非争议）
	DisputedCount   int    `json:"disputed_count"`
	CoverageSubject Metric `json:"coverage_subject"` // 既有身份覆盖率（非空 subject_key / N）
	CoverageEntity  Metric `json:"coverage_entity"`  // 实体侧覆盖率（有决策 / N）
	MissingIdentity int    `json:"missing_identity"` // 既有侧空 subject_key 单列
	Unresolved      int    `json:"unresolved"`       // 实体侧无决策单列
	TechFailures    int    `json:"tech_failures"`
	WrongReuse      int    `json:"wrong_reuse"`       // 错误复用数（复用了已有实体但金标不同）
	ErrorRateEntity Metric `json:"error_rate_entity"` // 错误复用 / N（固定总 N 口径）
	// Comparable-subset pairwise metrics, per system.
	ComparableN     int    `json:"comparable_n"`
	SubjectMerge    Metric `json:"subject_merge_rate"` // 误合（金标不同而预测同组）
	EntityMerge     Metric `json:"entity_merge_rate"`
	SubjectSplit    Metric `json:"subject_split_rate"` // 漏合（金标相同而预测不同组）
	EntitySplit     Metric `json:"entity_split_rate"`
	SubjectDupGroup Metric `json:"subject_dup_group_rate"` // 重复身份组率（金标组被拆 ≥2 预测组）
	EntityDupGroup  Metric `json:"entity_dup_group_rate"`
	Blocked         bool   `json:"blocked"` // 技术失败 ≠ 0 → 阻断该段
	BlockReason     string `json:"block_reason,omitempty"`
	MeetsBar        bool   `json:"meets_bar"`
	MeetsBarDetail  string `json:"meets_bar_detail"`
}

// Score joins gold with records and computes every line mechanically.
func Score(gold []GoldEntry, records []MentionRecord) ScoreReport {
	byID := make(map[string]MentionRecord, len(records))
	for _, r := range records {
		byID[r.SampleID] = r
	}

	var rep ScoreReport
	rows := make([]srow, 0, len(gold))
	for _, g := range gold {
		if g.Disputed {
			rep.DisputedCount++
			continue
		}
		rep.TotalN++
		r, ok := byID[g.SampleID]
		row := srow{sampleID: g.SampleID, gold: g.GoldEntity, subject: r.SubjectKey}
		if ok && !r.TechFailure && r.EntityID != 0 {
			row.entity = fmt.Sprintf("e%d", r.EntityID)
			row.reused = r.Status == "reused" || r.Status == "idempotent"
		} else if ok && r.TechFailure {
			rep.TechFailures++
		}
		if row.subject == "" {
			rep.MissingIdentity++
		}
		if row.entity == "" {
			rep.Unresolved++
		}
		rows = append(rows, row)
	}

	// Coverage over fixed N (a missing prediction is a MISS, never excluded).
	withSubject, withEntity := 0, 0
	for _, r := range rows {
		if r.subject != "" {
			withSubject++
		}
		if r.entity != "" {
			withEntity++
		}
	}
	rep.CoverageSubject = rate(withSubject, rep.TotalN)
	rep.CoverageEntity = rate(withEntity, rep.TotalN)

	// Comparable subset: BOTH systems produced a prediction.
	subset := make([]srow, 0, len(rows))
	for _, r := range rows {
		if r.subject != "" && r.entity != "" {
			subset = append(subset, r)
		}
	}
	rep.ComparableN = len(subset)
	rep.SubjectMerge = pairwise(subset, func(r srow) string { return r.subject }, true)
	rep.EntityMerge = pairwise(subset, func(r srow) string { return r.entity }, true)
	rep.SubjectSplit = pairwise(subset, func(r srow) string { return r.subject }, false)
	rep.EntitySplit = pairwise(subset, func(r srow) string { return r.entity }, false)
	rep.SubjectDupGroup = dupGroupRate(subset, func(r srow) string { return r.subject })
	rep.EntityDupGroup = dupGroupRate(subset, func(r srow) string { return r.entity })

	// Wrong reuse: reused an existing entity while a same-prediction sibling
	// carries a DIFFERENT gold label (the reuse fused distinct entities).
	for _, r := range rows {
		if !r.reused || r.entity == "" {
			continue
		}
		for _, o := range rows {
			if o.sampleID == r.sampleID || o.entity != r.entity {
				continue
			}
			if o.gold != r.gold {
				rep.WrongReuse++
				break
			}
		}
	}
	rep.ErrorRateEntity = rate(rep.WrongReuse, rep.TotalN)

	if rep.TechFailures != 0 {
		rep.Blocked = true
		rep.BlockReason = fmt.Sprintf("技术失败 %d ≠ 0：阻断该段，修复后重跑", rep.TechFailures)
		return rep
	}
	rep.MeetsBar, rep.MeetsBarDetail = evaluateBar(rep)
	return rep
}

// pairwise walks all sample pairs in the subset. mergeMode=true counts
// pairs predicted into the SAME group while gold says DIFFERENT entities
// (误合); mergeMode=false counts pairs predicted into DIFFERENT groups while
// gold says the SAME entity (漏合).
func pairwise(rows []srow, key func(srow) string, mergeMode bool) Metric {
	den := len(rows) * (len(rows) - 1) / 2
	if den == 0 {
		return Metric{}
	}
	bad := 0
	for i := 0; i < len(rows); i++ {
		for j := i + 1; j < len(rows); j++ {
			samePred := key(rows[i]) == key(rows[j])
			sameGold := rows[i].gold == rows[j].gold
			if mergeMode && samePred && !sameGold {
				bad++
			}
			if !mergeMode && !samePred && sameGold {
				bad++
			}
		}
	}
	return rate(bad, den)
}

// dupGroupRate counts gold groups split into ≥2 distinct prediction groups;
// denominator = gold groups present in the subset.
func dupGroupRate(rows []srow, key func(srow) string) Metric {
	groups := map[string]map[string]bool{}
	for _, r := range rows {
		if groups[r.gold] == nil {
			groups[r.gold] = map[string]bool{}
		}
		groups[r.gold][key(r)] = true
	}
	if len(groups) == 0 {
		return Metric{}
	}
	dup := 0
	for _, preds := range groups {
		if len(preds) >= 2 {
			dup++
		}
	}
	return rate(dup, len(groups))
}

// evaluateBar applies the four mechanically-recomputable gates (design D12):
// on the same comparable subset — merge rate not higher than the legacy
// identity, duplicate-group rate lower, coverage not lower; plus zero
// technical failures (checked by the caller). Insufficient data (zero
// denominators) → NOT met, with the reason spelled out.
func evaluateBar(rep ScoreReport) (bool, string) {
	var reasons []string
	if rep.ComparableN == 0 {
		reasons = append(reasons, "可比子集为空（N=0）")
	}
	if rep.CoverageSubject.Den == 0 {
		reasons = append(reasons, "固定总 N 为 0")
	}
	if len(reasons) > 0 {
		return false, "数据不足，不判达标：" + join(reasons, "；")
	}
	if rep.EntityMerge.Rate > rep.SubjectMerge.Rate {
		reasons = append(reasons, fmt.Sprintf("误合率 %.4f 高于既有身份 %.4f", rep.EntityMerge.Rate, rep.SubjectMerge.Rate))
	}
	if rep.EntityDupGroup.Rate >= rep.SubjectDupGroup.Rate {
		reasons = append(reasons, fmt.Sprintf("重复身份组率 %.4f 未低于既有身份 %.4f", rep.EntityDupGroup.Rate, rep.SubjectDupGroup.Rate))
	}
	if rep.CoverageEntity.Rate < rep.CoverageSubject.Rate {
		reasons = append(reasons, fmt.Sprintf("覆盖率 %.4f 低于既有身份 %.4f", rep.CoverageEntity.Rate, rep.CoverageSubject.Rate))
	}
	if len(reasons) > 0 {
		return false, "未达标：" + join(reasons, "；")
	}
	return true, "同一可比子集上误合不增加、重复身份减少、覆盖率不低于基线、技术失败为 0"
}

func join(parts []string, sep string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += sep
		}
		out += p
	}
	return out
}
