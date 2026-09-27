package models

import "syntopica-backend/internal/platform/database"

// Register domain-owned models with AutoMigrate via the injection registry
// (pattern: discovery/models). topicgraph/models must not be imported by
// platform/database — that would cycle with this init.
func init() {
	database.RegisterModels(&TopicAnalysisCursor{})
}
