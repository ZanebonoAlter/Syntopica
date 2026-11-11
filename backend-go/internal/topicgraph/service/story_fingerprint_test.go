package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"syntopica-backend/internal/models"
)

// fingerprintTag builds a models.TopicTag carrying a nested event_fingerprint
// (the shape batch 2a's tagger persists) plus event_keywords / aliases.
func fingerprintTag(fingerprint map[string]any, keywords any, aliases string) models.TopicTag {
	meta := models.MetadataMap{}
	if fingerprint != nil {
		meta["event_fingerprint"] = fingerprint
	}
	if keywords != nil {
		meta["event_keywords"] = keywords
	}
	return models.TopicTag{ID: 7, Label: "阳光电源股价暴涨", Metadata: meta, Aliases: aliases}
}

func validFingerprintMap() map[string]any {
	return map[string]any{
		"subject":     "阳光电源",
		"subject_key": "company:sungrow",
		"action":      "暴涨",
		"object":      "股价",
		"occurred_at": "2026-11-05",
	}
}

func TestParseEventFingerprintComplete(t *testing.T) {
	tag := fingerprintTag(validFingerprintMap(), []any{"阳光电源", "股价暴涨", "业绩反转"}, `["Sungrow"]`)
	fp, ok := ParseEventFingerprint(tag)
	require.True(t, ok, "complete fingerprint must parse as complete")
	assert.Equal(t, uint(7), fp.TagID)
	assert.Equal(t, "阳光电源", fp.Subject)
	assert.Equal(t, "company:sungrow", fp.SubjectKey)
	assert.Equal(t, []string{"阳光电源", "股价暴涨", "业绩反转"}, fp.Keywords)
	assert.Equal(t, []string{"Sungrow"}, fp.Aliases)
	assert.True(t, fp.Trusted)
	assert.Equal(t, FingerprintReasonOK, fp.TrustReason)
}

func TestParseEventFingerprintNormalizesSubjectKey(t *testing.T) {
	m := validFingerprintMap()
	m["subject_key"] = " Person:Donald Trump "
	tag := fingerprintTag(m, []any{"特朗普", "访华"}, "")
	fp, ok := ParseEventFingerprint(tag)
	require.True(t, ok)
	assert.Equal(t, "person:donald_trump", fp.SubjectKey)
}

func TestParseEventFingerprintIncomplete(t *testing.T) {
	// subject missing → structurally incomplete (P2), untrusted, but still parsed.
	m := validFingerprintMap()
	delete(m, "subject")
	tag := fingerprintTag(m, []any{"阳光电源"}, "")
	fp, ok := ParseEventFingerprint(tag)
	assert.False(t, ok)
	assert.False(t, fp.Trusted)
	assert.Equal(t, FingerprintReasonMissingFields, fp.TrustReason)

	// No event_fingerprint at all → incomplete, keywords still surfaced.
	tag = fingerprintTag(nil, []string{"美国", "制裁"}, "")
	fp, ok = ParseEventFingerprint(tag)
	assert.False(t, ok)
	assert.False(t, fp.Trusted)
	assert.Equal(t, FingerprintReasonMissingFields, fp.TrustReason)
	assert.Equal(t, []string{"美国", "制裁"}, fp.Keywords)
}

func TestParseEventFingerprintAliasesDegradation(t *testing.T) {
	// Broken aliases JSON → empty aliases, never an error; keyword list
	// ([]any with non-string junk dropped) still parses.
	tag := fingerprintTag(validFingerprintMap(), []any{"阳光电源", 42, "股价"}, `not json`)
	fp, ok := ParseEventFingerprint(tag)
	require.True(t, ok)
	assert.Empty(t, fp.Aliases)
	assert.Equal(t, []string{"阳光电源", "股价"}, fp.Keywords)
	assert.True(t, fp.Trusted)
}

func TestValidateEventFingerprintMatrix(t *testing.T) {
	valid := func() EventTagFingerprint {
		return EventTagFingerprint{
			Subject: "阳光电源", SubjectKey: "company:sungrow",
			Action: "暴涨", Object: "股价",
		}
	}

	t.Run("consistent", func(t *testing.T) {
		ok, reason := ValidateEventFingerprint(valid(), []string{"阳光电源", "股价暴涨"}, nil)
		assert.True(t, ok)
		assert.Equal(t, FingerprintReasonOK, reason)
	})

	t.Run("subject mismatch", func(t *testing.T) {
		ok, reason := ValidateEventFingerprint(valid(), []string{"隆基绿能", "股价"}, nil)
		assert.False(t, ok)
		assert.Equal(t, FingerprintReasonSubjectMiss, reason)
	})

	t.Run("object mismatch", func(t *testing.T) {
		ok, reason := ValidateEventFingerprint(valid(), []string{"阳光电源", "回购"}, nil)
		assert.False(t, ok)
		assert.Equal(t, FingerprintReasonObjectMiss, reason)
	})

	t.Run("no keywords", func(t *testing.T) {
		ok, reason := ValidateEventFingerprint(valid(), nil, []string{"阳光电源"})
		assert.False(t, ok)
		assert.Equal(t, FingerprintReasonNoKeywords, reason)
	})

	t.Run("alias expansion hit", func(t *testing.T) {
		fp := valid()
		fp.Subject = "Sungrow"
		// Subject only findable via the tag alias, not the keywords.
		ok, reason := ValidateEventFingerprint(fp, []string{"股价", "暴涨"}, []string{"Sungrow 阳光电源"})
		assert.True(t, ok)
		assert.Equal(t, FingerprintReasonOK, reason)
	})

	t.Run("full-width and case variants", func(t *testing.T) {
		fp := valid()
		fp.Subject = "ＳＵＮＧＲＯＷ" // full-width upper → NFKC + lower → sungrow
		fp.Object = "股 价"      // whitespace stripped by normalization
		ok, reason := ValidateEventFingerprint(fp, []string{"Sungrow Power", "股价暴涨"}, nil)
		assert.True(t, ok)
		assert.Equal(t, FingerprintReasonOK, reason)
	})

	t.Run("bidirectional containment", func(t *testing.T) {
		fp := valid()
		fp.Object = "股价暴涨创历史新高" // object longer than the keyword
		ok, reason := ValidateEventFingerprint(fp, []string{"阳光电源", "股价暴涨"}, nil)
		assert.True(t, ok)
		assert.Equal(t, FingerprintReasonOK, reason)
	})

	t.Run("missing fields", func(t *testing.T) {
		fp := valid()
		fp.Action = "  "
		ok, reason := ValidateEventFingerprint(fp, []string{"阳光电源"}, nil)
		assert.False(t, ok)
		assert.Equal(t, FingerprintReasonMissingFields, reason)
	})

	t.Run("invalid subject key shapes", func(t *testing.T) {
		for _, key := range []string{"trump", "foo:trump", "person:", ":trump", "person: :x"} {
			fp := valid()
			fp.SubjectKey = key
			ok, reason := ValidateEventFingerprint(fp, []string{"阳光电源", "股价"}, nil)
			assert.False(t, ok, "key %q must be rejected", key)
			assert.Equal(t, FingerprintReasonInvalidSubjectKey, reason, "key %q", key)
		}
		// All whitelisted prefixes pass shape check.
		for _, key := range []string{"person:trump", "company:bytedance", "org:pboc", "product:gpt-5", "other:市场情绪"} {
			fp := valid()
			fp.SubjectKey = key
			ok, reason := ValidateEventFingerprint(fp, []string{"阳光电源", "股价"}, nil)
			assert.True(t, ok, "key %q must pass", key)
			assert.Equal(t, FingerprintReasonOK, reason)
		}
	})
}
