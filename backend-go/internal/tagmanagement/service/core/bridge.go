package core

import (
	"context"

	"gorm.io/gorm"

	"syntopica-backend/internal/models"
)

// ============================================================================
// Cross-package function bridges.
// These function variables break import cycles between core/ and sub-packages
// (auxlabel, board, merge). Sub-packages set these during init().
// ============================================================================

// AuxServiceFactory creates an auxiliary label service.
// Set by the auxlabel package during init.
var AuxServiceFactory func(db *gorm.DB, embedder interface{}) AuxService

// AuxService is the interface that auxlabel.AuxiliaryLabelService satisfies.
type AuxService interface {
	AttachAuxiliaryLabels(ctx context.Context, tagID uint, labels []AuxiliaryLabel) error
	RecountRefs(ctx context.Context, auxLabelIDs []uint) error
}

// BoardMatcherFactory returns a board matching service singleton.
// Set by the board package during init.
var BoardMatcherFactory func() BoardMatcher

// BoardMatcher is the interface that board.SemanticBoardMatchingService satisfies.
type BoardMatcher interface {
	MatchTopicTag(ctx context.Context, topicTagID uint) (interface{}, error)
}

// EntitySubjectHook resolves ONE event tag's subject mention against the
// global entity identity layer (normalize-event-subject-entities). Set by
// the tagmanagement/service/entity package during init(); nil when that
// package is not linked (tests, minimal binaries) — the tagging pipeline
// then skips entity resolution entirely. Off mode is a no-op INSIDE the hook
// as well, so the bridge stays wired in every deployment without changing
// off-mode behavior.
//
// The hook NEVER blocks or fails tag persistence: it owns its error path,
// logging and swallowing everything (technical failures leave the mention
// 未归一, the tag itself is unaffected — spec「不阻断标签入库」).
var EntitySubjectHook func(ctx context.Context, tag TopicTag, dbTag *models.TopicTag, articleID uint, articleContext string)
