package models

import "syntopica-backend/internal/platform/database"

// Register domain-owned models with AutoMigrate via the injection registry
// (pattern shared with topicgraph/repository and platform/scheduler): the
// static list in database/migrator.go cannot import domain packages without
// import cycles.
func init() {
	database.RegisterModels(
		&DiscoveryRun{},
		&DiscoveryRunItem{},
		&DiscoveryInterestEntry{},
		&FeedCandidate{},
		&FeedRecommendation{},
		&CandidateAvailability{},
		&CandidateEmbedding{},
		&CandidatePreference{},
		&RSSHubRoute{},
		&RouteParamOption{},
		&RouteEmbedding{},
		&PreferenceVector{},
	)
}
