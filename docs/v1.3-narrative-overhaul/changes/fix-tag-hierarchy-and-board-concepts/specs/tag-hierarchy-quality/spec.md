## ADDED Requirements

### Requirement: Cleanup cycle without template enforcement phases
The `tag_hierarchy_cleanup` scheduler SHALL NOT execute Phase 3d (template violation check), Phase 4 (adopt-narrower), Phase 5 (abstract-tag-update), or Phase 6 (template-aligned tree review). The cleanup cycle SHALL execute only: data cleanup phases, relation cleanup phases, flat merge, event keyword clustering, format cleanup, and description backfill.

#### Scenario: Phase 3d/4/5/6 removed from cleanup cycle
- **WHEN** `runCleanupCycle` executes
- **THEN** it SHALL NOT call `CleanupTemplateViolations`, `ProcessPendingAdoptNarrowerTasks`, `ProcessPendingAbstractTagUpdateTasks`, `GetTemplate`, `BuildTagForest`, `Phase6_CheckLevelAlignment`, `Phase6_DedupL1`, `Phase6_DedupL2`, `Phase6_SampleAuditLeaves`, or `ReviewHierarchyTrees`

#### Scenario: Scheduler description updated
- **WHEN** the scheduler registers or syncs its task
- **THEN** the description SHALL NOT contain "adopt narrower", "abstract update", or "tree review"

### Requirement: Phase 3 relation cleanup executes before merge and clustering
The cleanup cycle SHALL execute Phase 3 (orphaned relations, multi-parent conflicts, empty abstract nodes, single-child abstract nodes) BEFORE Phase 2 (flat merge) and Phase 2.5 (event keyword clustering), so that clustering operates on clean data.

#### Scenario: Phase 3 executes before Phase 2
- **WHEN** `runCleanupCycle` executes
- **THEN** `CleanupOrphanedRelations`, `CleanupMultiParentConflicts`, `CleanupEmptyAbstractNodes`, `CleanupSingleChildAbstractNodes` SHALL be called before `ExecuteFlatMerge` and `ClusterUnclassifiedTags`

### Requirement: Cleanup cycle summary fields remain aligned
The `TagHierarchyCleanupRunSummary` struct SHALL remove fields for removed phases (`TemplateDepthViolations`, `TemplateCrossCategory`, `AdoptNarrowerProcessed`, `AbstractUpdateProcessed`, `TreesReviewed`, `MovesApplied`, `GroupsCreated`, `GroupsReused`) and the `Reason` summary string SHALL not reference them.

#### Scenario: Summary struct reflects new phase set
- **WHEN** the cleanup cycle completes
- **THEN** the summary JSON SHALL contain `event_clusters_found`, `event_keyword_edges`, `flat_merges_applied` but SHALL NOT contain `template_depth_violations`, `template_cross_category`, `adopt_narrower_processed`, `abstract_update_processed`, `trees_reviewed`, `moves_applied`, `groups_created`, or `groups_reused`
