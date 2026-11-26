package main

// entity-resolve-eval — bounded evaluation entry for the entity-subject
// resolution chain (normalize-event-subject-entities T4 / design D11/D12).
//
// The CLI is a protocol machine, not a credit card: every real model call
// only happens after (a) the protocol is FROZEN, (b) a budget plan is
// mechanically derived from the actual sample set, (c) a send-guard is
// registered in front of the provider clients, and (d) an explicit
// authorization file is presented (BUDGET-APPROVED) — without it the run
// subcommand refuses BEFORE loading config or connecting to any database.
// Nothing here writes tag business tables (runner output lives in eval
// artifacts + the entity layer only).

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// ---- protocol freeze (D12) ----

// Protocol captures every element that must be frozen BEFORE the run:
// sample list digest, article snapshot digest, grouping rules, model
// versions, budget plan and the scoring-rule fingerprint. After freezing,
// changing ANY element invalidates the run.
type Protocol struct {
	SampleDigest   string            `json:"sample_digest"`   // frozen sample list digest
	SnapshotDigest string            `json:"snapshot_digest"` // frozen article snapshot digest
	GroupRules     []string          `json:"group_rules"`     // e.g. same-name-diff-entity / cross-lang / none
	ModelVersions  map[string]string `json:"model_versions"`  // role → binding model (chat/embed)
	Budget         BudgetPlan        `json:"budget"`
	ScoringRules   string            `json:"scoring_rules"` // scoring-rule fingerprint
	DisputedCount  int               `json:"disputed_count"`
	FrozenAt       string            `json:"frozen_at"`
	FrozenDigest   string            `json:"frozen_digest"` // digest over all elements above
}

// digestOf serializes the mutable elements and hashes them. FrozenAt is
// included in the JSON but excluded from the digest computation input's
// FrozenDigest/FrozenAt fields themselves.
func digestOf(p Protocol) string {
	clone := p
	clone.FrozenDigest = ""
	clone.FrozenAt = ""
	b, err := json.Marshal(clone)
	if err != nil {
		return "marshal-error:" + err.Error()
	}
	return fnvHex(string(b))
}

// Freeze stamps the protocol as frozen (computes the digest over all
// elements, with FrozenAt recorded verbatim for audit).
func Freeze(p Protocol, frozenAt string) Protocol {
	p.FrozenAt = frozenAt
	p.FrozenDigest = digestOf(p)
	return p
}

// VerifyFrozen reports whether a (possibly edited) protocol still matches
// its frozen digest. The runner refuses to run against a drifted protocol —
// frozen means frozen: no post-hoc sample/rule/budget edits.
func VerifyFrozen(p Protocol) error {
	if p.FrozenDigest == "" {
		return fmt.Errorf("协议未冻结：先运行 freeze 子命令")
	}
	if got := digestOf(p); got != p.FrozenDigest {
		return fmt.Errorf("协议已漂移：冻结摘要 %s ≠ 当前 %s（冻结后不得改样本/题目/规则）", p.FrozenDigest, got)
	}
	return nil
}

// fnvHex is a small stable 64-bit FNV-1a (deterministic across runs — this
// is an integrity fingerprint, not a security hash).
func fnvHex(s string) string {
	const (
		offset64 uint64 = 14695981039346656037
		prime64  uint64 = 1099511628211
	)
	h := offset64
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= prime64
	}
	return fmt.Sprintf("%016x", h)
}

// ---- samples ----

// EvalSample is one evaluation item: one article snapshot + one subject
// mention extracted from it by the REAL first round (or replayed from a
// persisted shadow result). Gold labels NEVER travel with samples — the
// runner must not see them.
type EvalSample struct {
	SampleID     string `json:"sample_id"`
	ArticleTitle string `json:"article_title"`
	ContextText  string `json:"context_text"` // mention-bearing fragment (原文)
	Mention      string `json:"mention"`      // first-round verbatim mention
	SubjectDesc  string `json:"subject_desc"` // first-round stable short description (may be empty)
	SubjectKey   string `json:"subject_key"`  // 既有身份（评测对照侧输入，非金标）
	Group        string `json:"group"`        // grouping-rule tag
	Disputed     bool   `json:"disputed"`     // pre-listed disputed item — 不计身份分
}

// SamplesDigest fingerprints the sample list (id + mention + group + disputed
// + snapshot text) so any post-freeze edit is detectable.
func SamplesDigest(samples []EvalSample) string {
	var b strings.Builder
	for _, s := range samples {
		fmt.Fprintf(&b, "%s|%s|%s|%s|%s|%t\n", s.SampleID, s.Mention, s.SubjectDesc, s.ContextText, s.Group, s.Disputed)
	}
	return fnvHex(b.String())
}

// loadSamplesFile reads a samples JSON file (worksheets and runs read the
// SAME frozen artifact).
func loadSamplesFile(path string) ([]EvalSample, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // CLI 输入路径由运行者显式提供
	if err != nil {
		return nil, fmt.Errorf("read samples: %w", err)
	}
	var samples []EvalSample
	if err := json.Unmarshal(raw, &samples); err != nil {
		return nil, fmt.Errorf("parse samples: %w", err)
	}
	return samples, nil
}

// loadProtocolFile reads a frozen protocol JSON.
func loadProtocolFile(path string) (Protocol, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // CLI 输入路径由运行者显式提供
	if err != nil {
		return Protocol{}, fmt.Errorf("read protocol: %w", err)
	}
	var p Protocol
	if err := json.Unmarshal(raw, &p); err != nil {
		return Protocol{}, fmt.Errorf("parse protocol: %w", err)
	}
	return p, nil
}
