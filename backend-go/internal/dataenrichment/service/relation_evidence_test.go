package service

import (
	"testing"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/dataenrichment/repository"
)

func TestRelationEvidenceQuoteVerification(t *testing.T) {
	raw := `{"query":"日债 原因","results":[{"title":"报道","url":"https://a.example/x","snippet":"中东局势推升油价与全球通胀预期，日债遭抛售"}]}`

	// Verbatim hit (with spacing / case / curly-quote normalization).
	require.True(t, verifyEvidenceQuote("中东局势推升油价与全球通胀预期，日债遭抛售", raw))
	require.True(t, verifyEvidenceQuote("  中东局势推升油价与全球通胀预期，日债遭抛售\t ", raw))
	// A space INSERTED inside the original text is NOT a verbatim quote —
	// whitespace folding collapses runs, it never deletes content.
	require.False(t, verifyEvidenceQuote("中东局势推升油价 与全球通胀预期", raw))
	// Ghost quote (model invention) never matches.
	require.False(t, verifyEvidenceQuote("美联储直接买入了日本国债", raw))
	// Empty / whitespace quotes never verify.
	require.False(t, verifyEvidenceQuote("", raw))
	require.False(t, verifyEvidenceQuote("   ", raw))
}

func TestRelationQualityGrade(t *testing.T) {
	ev1 := []repository.RelationEvidence{
		{URL: "https://a.example/1", Use: "support", Verified: true},
	}
	ev2SameDomain := []repository.RelationEvidence{
		{URL: "https://a.example/1", Use: "support", Verified: true},
		{URL: "https://www.a.example/2", Use: "support", Verified: true},
	}
	ev2Domains := []repository.RelationEvidence{
		{URL: "https://a.example/1", Use: "support", Verified: true},
		{URL: "https://b.example/2", Use: "support", Verified: true},
	}
	unverified := []repository.RelationEvidence{
		{URL: "https://a.example/1", Use: "support", Verified: false},
	}
	counterOnly := []repository.RelationEvidence{
		{URL: "https://a.example/1", Use: "counter", Verified: true},
	}

	require.Equal(t, repository.RelationGradeLow, relationQualityGrade(nil, true))
	require.Equal(t, repository.RelationGradeLow, relationQualityGrade(unverified, true))
	require.Equal(t, repository.RelationGradeLow, relationQualityGrade(counterOnly, true))
	require.Equal(t, repository.RelationGradeMedium, relationQualityGrade(ev1, false))
	require.Equal(t, repository.RelationGradeMedium, relationQualityGrade(ev2SameDomain, true), "single domain caps at medium")
	require.Equal(t, repository.RelationGradeHigh, relationQualityGrade(ev2Domains, true), "2 domains + counter searched = high")
	require.Equal(t, repository.RelationGradeMedium, relationQualityGrade(ev2Domains, false), "no counter search caps at medium")
}

func TestScrubRelationEvidenceDropsUnprovenanced(t *testing.T) {
	raw := map[string]string{
		"s1": `{"results":[{"snippet":"原文甲"}]}`,
	}
	entries := []repository.RelationEvidence{
		{Ref: "s1", URL: "https://a/1", Quote: "原文甲"},
		{Ref: "s1", URL: "https://a/2", Quote: "不存在的引用"},
		{Ref: "s9", URL: "https://a/3", Quote: "幽灵 ref"},
	}
	out, dropped := scrubRelationEvidence(entries, raw)
	require.Equal(t, 1, dropped, "entry without raw provenance is dropped")
	require.Len(t, out, 2)
	verified := 0
	for _, e := range out {
		if e.Verified {
			verified++
		}
	}
	require.Equal(t, 1, verified, "ghost quote stays but never verifies")
}

func TestRelationEvidenceVersionStability(t *testing.T) {
	ev := []repository.RelationEvidence{
		{URL: "https://a/1", Quote: "引文一", Verified: true},
		{URL: "https://b/2", Quote: "引文二", Verified: true},
	}
	v1 := relationEvidenceVersion(ev)
	v2 := relationEvidenceVersion(ev)
	require.Equal(t, v1, v2, "same verified evidence → same version")

	evChanged := []repository.RelationEvidence{
		{URL: "https://a/1", Quote: "引文一改", Verified: true},
		{URL: "https://b/2", Quote: "引文二", Verified: true},
	}
	require.NotEqual(t, v1, relationEvidenceVersion(evChanged), "changed evidence → new version")

	allUnverified := []repository.RelationEvidence{{URL: "https://a/1", Quote: "x", Verified: false}}
	require.Equal(t, "v0", relationEvidenceVersion(allUnverified))
	require.Equal(t, "v0", relationEvidenceVersion(nil))
}
