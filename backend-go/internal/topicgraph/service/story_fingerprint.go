package service

import (
	"encoding/json"
	"strings"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/textutil"
)

// Event-fingerprint consistency validation (add-event-story-heat design §3.3,
// specs/event-story「事件指纹结构化产出」, task 2.3). Pure string comparison —
// no DB, no AI. The fingerprint is produced inside the extraction call
// (batch 2a) and stored nested under topic_tags.metadata.event_fingerprint;
// this file re-parses it from the persisted tag shape and validates it
// against the tag's own metadata.event_keywords (99.7% coverage) plus
// topic_tags.aliases. An untrusted fingerprint never blocks the tag — the
// assignment path degrades to the LLM tier (task 3.1) instead.
//
// Persistence of fingerprint_trusted is batch 2c's job (task 2.4 wrote the
// nested object; this batch only computes trust).

// EventTagFingerprint is a persisted event tag's fingerprint plus its
// keyword-consistency verdict. Keywords/Aliases are carried along so callers
// (2c) can log why a tag was trusted or not without re-reading the tag.
type EventTagFingerprint struct {
	TagID   uint
	Label   string
	Subject string
	// SubjectKey is textutil.NormalizeSubjectKey-ed on parse.
	SubjectKey string
	Action     string
	Object     string
	OccurredAt string
	// Trusted / TrustReason carry ValidateEventFingerprint's verdict.
	Trusted     bool
	TrustReason string
	// Keywords mirrors metadata.event_keywords ([]any / []string, non-string
	// elements dropped); Aliases mirrors the topic_tags.aliases JSON array
	// (parse failure → empty).
	Keywords []string
	Aliases  []string
}

// Fingerprint trust reasons — short english enums, stable for logging.
const (
	FingerprintReasonOK                = "ok"
	FingerprintReasonMissingFields     = "missing_fields"
	FingerprintReasonInvalidSubjectKey = "invalid_subject_key"
	FingerprintReasonNoKeywords        = "no_keywords"
	FingerprintReasonSubjectMiss       = "subject_not_in_keywords"
	FingerprintReasonObjectMiss        = "object_not_in_keywords"
)

// validSubjectKeyPrefixes are the subject_key type prefixes accepted by the
// spec (person/company/org/product/other).
var validSubjectKeyPrefixes = map[string]bool{
	"person":  true,
	"company": true,
	"org":     true,
	"product": true,
	"other":   true,
}

// ParseEventFingerprint loads a tag's structured fingerprint out of
// topic_tags.metadata (nested event_fingerprint object), normalizes the
// subject key, and fills the keyword/alias side of the struct. The bool
// return reports structural completeness (subject/subject_key/action/object
// all non-empty — occurred_at optional); an incomplete fingerprint parses
// fine but is never trusted. Works with both map[string]any and
// models.MetadataMap nested values (MetadataMap IS a map[string]any).
func ParseEventFingerprint(tag models.TopicTag) (EventTagFingerprint, bool) {
	fp := EventTagFingerprint{TagID: tag.ID, Label: tag.Label}
	if nested, ok := tag.Metadata["event_fingerprint"].(map[string]any); ok {
		fp.Subject = strings.TrimSpace(stringValue(nested, "subject"))
		fp.SubjectKey = textutil.NormalizeSubjectKey(stringValue(nested, "subject_key"))
		fp.Action = strings.TrimSpace(stringValue(nested, "action"))
		fp.Object = strings.TrimSpace(stringValue(nested, "object"))
		fp.OccurredAt = strings.TrimSpace(stringValue(nested, "occurred_at"))
	}
	fp.Keywords = parseMetadataKeywordStrings(tag.Metadata["event_keywords"])
	fp.Aliases = parseTagAliases(tag.Aliases)
	fp.Trusted, fp.TrustReason = ValidateEventFingerprint(fp, fp.Keywords, fp.Aliases)
	complete := fp.Subject != "" && fp.SubjectKey != "" && fp.Action != "" && fp.Object != ""
	return fp, complete
}

// ValidateEventFingerprint checks a fingerprint against the tag's event
// keywords and aliases (design §3.3):
//  1. all four identity fields present;
//  2. subject_key shaped 类型:规范名 with a whitelisted type prefix;
//  3. keywords non-empty (no validation basis → untrusted);
//  4. subject AND object each findable in keywords+aliases after
//     textutil.NormalizeEntityText, with bidirectional containment
//     (keyword contains value or value contains keyword — “股价” hits the
//     keyword “股价暴涨”).
//
// Returns (trusted, reason); reasons are the FingerprintReason* enums.
func ValidateEventFingerprint(fp EventTagFingerprint, keywords []string, aliases []string) (bool, string) {
	if strings.TrimSpace(fp.Subject) == "" || strings.TrimSpace(fp.SubjectKey) == "" ||
		strings.TrimSpace(fp.Action) == "" || strings.TrimSpace(fp.Object) == "" {
		return false, FingerprintReasonMissingFields
	}
	if !validSubjectKeyShape(fp.SubjectKey) {
		return false, FingerprintReasonInvalidSubjectKey
	}

	usable := make([]string, 0, len(keywords)+len(aliases))
	for _, k := range keywords {
		if n := textutil.NormalizeEntityText(k); n != "" {
			usable = append(usable, n)
		}
	}
	if len(usable) == 0 {
		// Only aliases left is not a validation basis: aliases are tag-level
		// (not per-event) so a keyword-less tag cannot be cross-checked.
		return false, FingerprintReasonNoKeywords
	}
	for _, a := range aliases {
		if n := textutil.NormalizeEntityText(a); n != "" {
			usable = append(usable, n)
		}
	}

	if !entityInList(fp.Subject, usable) {
		return false, FingerprintReasonSubjectMiss
	}
	if !entityInList(fp.Object, usable) {
		return false, FingerprintReasonObjectMiss
	}
	return true, FingerprintReasonOK
}

// validSubjectKeyShape reports whether key normalizes to 类型:规范名 with a
// whitelisted prefix and a non-empty name part.
func validSubjectKeyShape(key string) bool {
	normalized := textutil.NormalizeSubjectKey(strings.TrimSpace(key))
	idx := strings.IndexByte(normalized, ':')
	if idx <= 0 || idx == len(normalized)-1 {
		return false
	}
	name := normalized[idx+1:]
	// A canonical name carries no further colons ("person::x" is garbage).
	return validSubjectKeyPrefixes[normalized[:idx]] && name != "" && !strings.ContainsRune(name, ':')
}

// entityInList reports whether value (normalized) is bidirectionally
// contained in any normalized keyword/alias.
func entityInList(value string, normalizedList []string) bool {
	n := textutil.NormalizeEntityText(value)
	if n == "" {
		return false
	}
	for _, k := range normalizedList {
		if strings.Contains(k, n) || strings.Contains(n, k) {
			return true
		}
	}
	return false
}

// stringValue reads a trimmed string field out of a nested metadata map.
func stringValue(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return strings.TrimSpace(s)
}

// parseMetadataKeywordStrings extracts metadata.event_keywords, accepting
// both []any (jsonb scan / MetadataMap default) and []string; non-string
// elements are dropped (same tolerance as core.getEventKeywords).
func parseMetadataKeywordStrings(raw any) []string {
	switch list := raw.(type) {
	case []any:
		out := make([]string, 0, len(list))
		for _, item := range list {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	case []string:
		out := make([]string, 0, len(list))
		for _, s := range list {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// parseTagAliases parses the topic_tags.aliases JSON string array; a parse
// failure (or NULL) degrades to empty — alias expansion is best-effort.
func parseTagAliases(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}
