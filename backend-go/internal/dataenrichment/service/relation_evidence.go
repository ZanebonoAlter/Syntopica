package service

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strings"

	"syntopica-backend/internal/dataenrichment/repository"
)

// ── Evidence contract + conservative verification (spec: 可核查的外部证据契约) ──

// relationEvidenceGap is one structured gap recorded on runs/relations.
type relationEvidenceGap struct {
	Reason string `json:"reason"`
	Detail string `json:"detail,omitempty"`
}

// normalizeQuoteForCheck folds whitespace and unified quotes so a verbatim
// quote survives minor formatting differences while staying a strict substring
// probe (no fuzzy rewriting — a quote that cannot be found is NOT evidence).
func normalizeQuoteForCheck(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.Map(func(r rune) rune {
		switch r {
		case '　', '\t', '\n', '\r':
			return ' '
		case '“', '‘':
			return '"'
		case '”', '’':
			return '"'
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}

// verifyEvidenceQuote checks a model-supplied quote against the raw tool
// result text it claims to come from. Returns verified=true only when the
// normalized quote is a substring of the normalized raw text. Empty quotes
// never verify.
func verifyEvidenceQuote(quote, rawToolResult string) bool {
	q := normalizeQuoteForCheck(quote)
	if q == "" {
		return false
	}
	raw := normalizeQuoteForCheck(rawToolResult)
	return strings.Contains(raw, q)
}

// evidenceDomain extracts the registrable-ish host of a URL for independent-
// source counting ("example.com" from https://www.example.com/a).
func evidenceDomain(rawURL string) string {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Host == "" {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	host = strings.TrimPrefix(host, "www.")
	return host
}

// relationQualityGrade computes the program-only quality grade from mechanical
// signals (design D2: model self-reported confidence is ignored).
//
//	high:   ≥2 verified support quotes from ≥2 distinct domains AND a
//	        counter-evidence search was performed
//	medium: ≥1 verified support quote
//	low:    anything else
func relationQualityGrade(evidence []repository.RelationEvidence, counterSearched bool) string {
	verified := 0
	domains := map[string]bool{}
	for _, ev := range evidence {
		if ev.Use != "support" || !ev.Verified || ev.URL == "" {
			continue
		}
		verified++
		if d := evidenceDomain(ev.URL); d != "" {
			domains[d] = true
		}
	}
	switch {
	case verified >= 2 && len(domains) >= 2 && counterSearched:
		return repository.RelationGradeHigh
	case verified >= 1:
		return repository.RelationGradeMedium
	default:
		return repository.RelationGradeLow
	}
}

// scrubRelationEvidence mechanically verifies each evidence entry against the
// matching raw tool result (by ref) and drops unverifiable quotes: an entry
// whose quote fails the substring check is marked Verified=false and excluded
// from support counting; entries without any raw result to check against are
// dropped entirely (spec: 模型输出不存在的引用 MUST 被剔除).
func scrubRelationEvidence(entries []repository.RelationEvidence, rawByRef map[string]string) ([]repository.RelationEvidence, int) {
	out := make([]repository.RelationEvidence, 0, len(entries))
	dropped := 0
	for _, ev := range entries {
		raw, ok := rawByRef[ev.Ref]
		if !ok {
			dropped++ // no provenance → not verifiable → dropped
			continue
		}
		// A quote that cannot be found in the raw tool output stays on the
		// row for transparency (Verified=false) but never counts as support
		// in relationQualityGrade; dropping it entirely would hide the failure.
		ev.Verified = verifyEvidenceQuote(ev.Quote, raw)
		out = append(out, ev)
	}
	return out, dropped
}

// relationEvidenceVersion derives a stable evidence version string from the
// verified evidence set so the suggestion hash changes when the underlying
// evidence actually changes (same evidence → same hash → idempotent insert).
func relationEvidenceVersion(entries []repository.RelationEvidence) string {
	parts := make([]string, 0, len(entries))
	for _, ev := range entries {
		if ev.Verified {
			parts = append(parts, ev.URL+"|"+normalizeQuoteForCheck(ev.Quote))
		}
	}
	if len(parts) == 0 {
		return "v0"
	}
	return "v1-" + shortStableHash(strings.Join(parts, "\x1f"))
}

// shortStableHash returns a 16-hex stable digest (non-cryptographic use:
// evidence versioning only).
func shortStableHash(s string) string {
	digest := sha256.Sum256([]byte(s))
	return hex.EncodeToString(digest[:])[:16]
}
