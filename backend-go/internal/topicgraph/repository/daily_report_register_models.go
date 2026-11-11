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
		&TopicKeyEventMarker{},
		&KeyEventRecommendationRun{},
		// add-event-story-heat：事件线与热度证据（specs/event-story、
		// specs/story-heat-evidence）。CHECK/FK/部分索引由迁移 20261105_0001 固化。
		&BoardStory{},
		&StorySignal{},
	)
	tagging.RegisterVectorDimEnsurer(ensureSectionEmbeddingDimension)
	tagging.RegisterVectorDimEnsurer(ensurePersistentTopicEmbeddingDimension)

	// Wire the prune migration to rebuild board relations after deleting
	// underqualified candidates, matching DeleteTopic's semantics.
	database.PruneRelationsRebuild = RebuildBoardRelations
}
