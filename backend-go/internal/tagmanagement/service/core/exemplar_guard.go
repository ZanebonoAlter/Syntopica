package core

import "strings"

// mergedPromptExemplarLabels lists the concrete tag labels that appear as
// few-shot exemplars in buildMergedExtractionPrompt. Small local models have
// been observed copying these exemplars verbatim into responses for articles
// that never mention them (guard-tag-exemplar-leakage: 69% of PostgreSQL tag
// links had zero mentions in the article text).
//
// 维护纪律：改 buildMergedExtractionPrompt 的示例词必须同步本集合——
// TestExemplarLabelsCoverPrompts 断言集合每个词原文出现在 prompt 文本中。
var mergedPromptExemplarLabels = []string{
	// keyword 定义举例 / description 示例 / 输出格式正例
	"PostgreSQL", "Transformer架构",
	// event 定义举例 / 输出格式正例
	"央行禁止比特币交易", "某国央行上调基准利率",
	// 正例 JSON 的 auxiliary_labels
	"央行", "基准利率", "利率上调",
}

// sectionPromptExemplarLabels is the same-source exemplar set for
// buildSectionExtractionPrompt (aggregate per-section extraction).
var sectionPromptExemplarLabels = []string{
	// event 正确示例
	"苹果WWDC 2024发布会", "央行禁止比特币交易",
	// keyword 正确示例
	"Transformer架构", "PostgreSQL", "Kubernetes",
	// 输出格式正例 event + auxiliary_labels
	"某国央行上调基准利率", "央行", "基准利率", "利率上调",
}

var (
	mergedExemplarSlugs  = exemplarSlugSet(mergedPromptExemplarLabels)
	sectionExemplarSlugs = exemplarSlugSet(sectionPromptExemplarLabels)
)

func exemplarSlugSet(labels []string) map[string]struct{} {
	m := make(map[string]struct{}, len(labels))
	for _, l := range labels {
		if s := Slugify(l); s != "" {
			m[s] = struct{}{}
		}
	}
	return m
}

// guardExemplarLeakage drops candidates that copy prompt exemplars without
// evidence in the model-visible input: a candidate whose label (Slugify-
// normalized) hits the exemplar slug set AND whose label text (case-folded)
// does not appear in visibleText is dropped before persistence. Kept tags have
// their auxiliary_labels filtered by the same rule, item by item. It returns
// the filtered tags and the labels of dropped main candidates for observability.
func guardExemplarLeakage(tags []ExtractedTag, visibleText string, exemplarSlugs map[string]struct{}) ([]ExtractedTag, []string) {
	if len(tags) == 0 {
		return tags, nil
	}
	visible := strings.ToLower(visibleText)
	kept := make([]ExtractedTag, 0, len(tags))
	var dropped []string
	for _, tag := range tags {
		if isExemplarWithoutEvidence(tag.Label, visible, exemplarSlugs) {
			dropped = append(dropped, tag.Label)
			continue
		}
		if len(tag.AuxiliaryLabels) > 0 {
			tag.AuxiliaryLabels = filterAuxiliaryExemplarLeakage(tag.AuxiliaryLabels, visible, exemplarSlugs)
		}
		kept = append(kept, tag)
	}
	return kept, dropped
}

func filterAuxiliaryExemplarLeakage(aux []AuxiliaryLabel, visible string, exemplarSlugs map[string]struct{}) []AuxiliaryLabel {
	kept := make([]AuxiliaryLabel, 0, len(aux))
	for _, a := range aux {
		if isExemplarWithoutEvidence(a.Label, visible, exemplarSlugs) {
			continue
		}
		kept = append(kept, a)
	}
	return kept
}

func isExemplarWithoutEvidence(label, visibleLower string, exemplarSlugs map[string]struct{}) bool {
	if _, hit := exemplarSlugs[Slugify(label)]; !hit {
		return false
	}
	return !strings.Contains(visibleLower, strings.ToLower(label))
}

// allExemplarSlugs is the union of both prompt exemplar sets. Used by one-shot
// cleanup tooling (cmd/admin tagguard-cleanup) which cannot know which
// extraction path produced a stored link, so it guards against the union.
var allExemplarSlugs = func() map[string]struct{} {
	m := make(map[string]struct{}, len(mergedExemplarSlugs)+len(sectionExemplarSlugs))
	for s := range mergedExemplarSlugs {
		m[s] = struct{}{}
	}
	for s := range sectionExemplarSlugs {
		m[s] = struct{}{}
	}
	return m
}()

// AllPromptExemplarLabels returns the union of both prompt exemplar label
// lists (original casing). Exported for one-shot cleanup tooling to build the
// SQL candidate filter; judgment stays in IsPromptExemplarWithoutEvidence.
func AllPromptExemplarLabels() []string {
	out := make([]string, 0, len(mergedPromptExemplarLabels)+len(sectionPromptExemplarLabels))
	out = append(out, mergedPromptExemplarLabels...)
	out = append(out, sectionPromptExemplarLabels...)
	return out
}

// IsPromptExemplarWithoutEvidence reports whether label hits any prompt
// exemplar set (union, Slugify-normalized) without case-insensitive evidence
// in visibleText. Exported for the one-shot cleanup tool so its judgment is
// same-source with the extraction-path guard.
func IsPromptExemplarWithoutEvidence(label, visibleText string) bool {
	return isExemplarWithoutEvidence(label, strings.ToLower(visibleText), allExemplarSlugs)
}
