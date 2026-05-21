## ADDED Requirements

### Requirement: FindSimilarTagsByKeywordOverlap function
The system SHALL provide a function `FindSimilarTagsByKeywordOverlap` in the tagging domain that accepts a set of tag IDs and returns similarity edges based on two-stage filtering (keyword overlap + semantic embedding). This function SHALL be used by `ClusterUnclassifiedTagsWithConfig` for event category tags.

#### Scenario: Event tags processed with keyword overlap
- **WHEN** `ClusterUnclassifiedTagsWithConfig` is called with category="event" and 244 unclassified tag IDs
- **THEN** the system SHALL call `FindSimilarTagsByKeywordOverlap` instead of `FindSimilarTagsAmongSet`

#### Scenario: Config loading for event clustering
- **WHEN** `ClusterUnclassifiedTagsWithConfig` is called with category="event"
- **THEN** the system SHALL load `event_cluster_kw_min_overlap` and `event_cluster_sem_threshold` from `embedding_config`
