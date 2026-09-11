package repository

import (
	"syntopica-backend/internal/platform/database"
	tagging "syntopica-backend/internal/tagmanagement"
)

func init() {
	database.RegisterModels(
		&BoardDailyReport{},
		&DailyReportSection{},
		&DailyReportThread{},
		&SectionRelation{},
		&BoardPersistentTopic{},
		&BoardTopicWatch{},
		&TopicWatchHit{},
		&TopicLaneSnapshot{},
	)
	tagging.RegisterVectorDimEnsurer(ensureSectionEmbeddingDimension)
	tagging.RegisterVectorDimEnsurer(ensurePersistentTopicEmbeddingDimension)

	// Wire the prune migration to rebuild board relations after deleting
	// underqualified candidates, matching DeleteTopic's semantics.
	database.PruneRelationsRebuild = RebuildBoardRelations
}
