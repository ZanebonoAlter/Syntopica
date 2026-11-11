package entity

import (
	"context"
	"fmt"
	"sync"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/textutil"
	"syntopica-backend/internal/tagmanagement/repository"
	"syntopica-backend/internal/tagmanagement/service/core"
)

// Hook wiring (shadow integration, design D10). This file is the ONLY writer
// of core.EntitySubjectHook. Off mode short-circuits before ANY work — zero
// second-round calls, zero entity-layer writes, no entity_subject key.
// Shadow writes are limited to exactly three places: the entity layer,
// normal AI audit (written by the airouter router per attempt), and the
// topic_tags.metadata.entity_subject lazy key. Nothing else changes — not
// event_fingerprint, not tag business fields, not story/report/topic.

func init() {
	core.EntitySubjectHook = RunForTag
}

// routerChat adapts the production airouter router to ChatFunc. Every call
// carries the semantic operation; audit rows land per attempt (ai-summary
// red lines 1/7), success rows additionally record prompt+response_snippet.
func routerChat(ctx context.Context, req airouter.ChatRequest) (string, error) {
	result, err := airouter.NewRouter().Chat(ctx, req)
	if err != nil {
		return "", err
	}
	return result.Content, nil
}

// routerEmbed adapts the production router to EmbedFunc; the space triplet's
// Model/Version both carry the routed model string (a provider failover to a
// different embedding model naturally lands in a different space — exactly
// the cross-space guard D3 wants), Dim = len(vector).
func routerEmbed(ctx context.Context, text string) (QueryVector, error) {
	result, err := airouter.NewRouter().Embed(ctx, airouter.EmbeddingRequest{
		Input:     []string{text},
		Operation: "tagmanagement.entity_subject.query",
		Metadata:  map[string]any{"text": truncate(text)},
	}, airouter.CapabilityEmbedding)
	if err != nil {
		return QueryVector{}, err
	}
	if len(result.Embeddings) == 0 || len(result.Embeddings[0]) == 0 {
		return QueryVector{}, fmt.Errorf("empty embedding result")
	}
	vec := result.Embeddings[0]
	return QueryVector{
		Vec:     vec,
		Triplet: SpaceTriplet{Model: result.Model, Version: result.Model, Dim: len(vec)},
	}, nil
}

func truncate(s string) string {
	r := []rune(s)
	if len(r) > 200 {
		return string(r[:200])
	}
	return s
}

var (
	defaultResolverOnce sync.Once
	defaultResolver     *Resolver
	// hookResolverForTest lets tests substitute the resolver behind
	// RunForTag without touching the production singleton. nil = default.
	hookResolverForTest func() *Resolver
)

// SetHookResolverForTest swaps the hook's resolver source; pass nil to
// restore the production default. Test-only lever.
func SetHookResolverForTest(factory func() *Resolver) {
	hookResolverForTest = factory
}

func hookResolver() *Resolver {
	if hookResolverForTest != nil {
		return hookResolverForTest()
	}
	return DefaultResolver()
}

// DefaultResolver lazily builds the production resolver against the shared
// repository singleton.
func DefaultResolver() *Resolver {
	defaultResolverOnce.Do(func() {
		defaultResolver = NewResolver(
			repository.NewEntityRepository(repository.Repo.DB()),
			routerChat, routerEmbed, DefaultResolverConfig(),
		)
	})
	return defaultResolver
}

// RunForTag is the tag-pipeline hook: resolves ONE event tag's subject
// mention in shadow mode and persists the lazy metadata.entity_subject key.
// Panics, mode errors and resolution failures are all logged and swallowed —
// tag persistence NEVER blocks on entity resolution (spec「不阻断标签入库」).
func RunForTag(ctx context.Context, tag core.TopicTag, dbTag *models.TopicTag, articleID uint, articleContext string) {
	defer func() {
		if r := recover(); r != nil {
			logging.Warnf("entity subject hook: panic recovered for tag %d: %v", dbTag.ID, r)
		}
	}()

	mode, err := CurrentMode()
	if err != nil {
		// active → explicit rejection: log loudly, do NOT silently run shadow.
		logging.Errorf("entity subject hook: %v (tag %d skipped)", err, dbTag.ID)
		return
	}
	if mode != ModeShadow || tag.Category != "event" || tag.Fingerprint == nil {
		return // off / non-event / no fingerprint → zero writes
	}
	mentionText := tag.Fingerprint.Subject
	if mentionText == "" {
		return // no verbatim subject mention to resolve
	}

	resolver := hookResolver()
	req := MentionRequest{
		MentionText:     mentionText,
		SubjectDesc:     tag.SubjectDesc,
		ContextSnippet:  contextSnippet(tag, articleContext),
		SourceTagID:     &dbTag.ID,
		SourceArticleID: &articleID,
		AuxLabelIDs:     matchAuxProvenance(ctx, tag.AuxiliaryLabels, mentionText),
	}
	resolution := resolver.ResolveMention(ctx, req)

	// The lazy metadata key — read-modify-write of the SINGLE entity_subject
	// slot (same discipline as persistEventFingerprint; never blind-write).
	rec := &core.EntitySubjectRecord{
		Subject:     mentionText,
		SubjectKey:  tag.Fingerprint.SubjectKey,
		SubjectDesc: tag.SubjectDesc,
		EntityID:    resolution.EntityID,
		EntityKey:   resolution.EntityKey,
		Status:      resolution.Status,
		Reason:      resolution.Reason,
		Mode:        string(ModeShadow),
	}
	if err := core.PersistEntitySubject(dbTag, rec); err != nil {
		logging.Warnf("entity subject hook: persist metadata.entity_subject for tag %d failed: %v", dbTag.ID, err)
	}
	if resolution.Status == StatusUnresolved {
		logging.Warnf("entity subject hook: mention %q left 未归一 for tag %d: %s", mentionText, dbTag.ID, resolution.Reason)
	}
}

// contextSnippet gives the second-round question its context fragment: the
// tag label (the event phrase) plus a bounded article context tail.
func contextSnippet(tag core.TopicTag, articleContext string) string {
	snippet := tag.Label
	if articleContext != "" {
		snippet += "\n" + truncate(articleContext)
	}
	return snippet
}

// matchAuxProvenance implements the ONLY aux-linking rule (D1/spec): among
// THIS tag's auxiliary labels, keep exactly those whose normalized label
// text equals the normalized mention text, and return their semantic_labels
// ids (slug lookup). Unrelated aux rows stay pure material — never linked,
// never treated as entity options. Auto-aliases and similarity hits are not
// same-entity evidence and can never enter through here.
func matchAuxProvenance(ctx context.Context, auxLabels []core.AuxiliaryLabel, mentionText string) []uint {
	mentionKey := textutil.NormalizeEntityText(mentionText)
	if mentionKey == "" || len(auxLabels) == 0 {
		return nil
	}
	var ids []uint
	seen := map[uint]bool{}
	for _, aux := range auxLabels {
		if textutil.NormalizeEntityText(aux.Label) != mentionKey {
			continue // not the「规范化相等」row — provenance rule does not apply
		}
		var row models.SemanticLabel
		err := repository.Repo.DB().WithContext(ctx).
			Where("slug = ? AND label_type = 'auxiliary'", core.Slugify(aux.Label)).
			First(&row).Error
		if err != nil {
			continue // aux row not materialized (yet) — nothing to link
		}
		if !seen[row.ID] {
			seen[row.ID] = true
			ids = append(ids, row.ID)
		}
	}
	return ids
}
