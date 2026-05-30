## ADDED Requirements

### Requirement: Bootstrap minimum cluster size
When `BootstrapConcepts` runs for a category, clusters returned by `findConnectedComponents` SHALL be filtered: only clusters with >= 5 tags SHALL proceed to LLM naming and concept creation. Clusters with fewer than 5 tags SHALL be excluded from concept creation.

#### Scenario: Large cluster creates concept
- **WHEN** bootstrap finds a cluster of 12 event tags connected via neighbor graph
- **THEN** the cluster SHALL proceed to LLM naming and a pending BoardConcept SHALL be created

#### Scenario: Small cluster is filtered out
- **WHEN** bootstrap finds a cluster of 3 event tags
- **THEN** the cluster SHALL be skipped; no LLM call SHALL be made; no BoardConcept SHALL be created

#### Scenario: Singleton tag forms no cluster
- **WHEN** bootstrap finds a tag with zero neighbors in the neighbor graph (isolated node)
- **THEN** the isolated tag SHALL form a cluster of size 1 and SHALL be filtered out

### Requirement: Default concept fallback
If after minimum cluster filtering no concepts would be created for a category, the system SHALL create exactly one default BoardConcept with status='active' (not 'pending'). The default concept name SHALL be the category's Chinese label ("事件" / "关键词" / "人物") and description SHALL indicate it is an auto-generated fallback.

#### Scenario: No viable clusters triggers default concept
- **WHEN** bootstrap runs for event category and all clusters have < 5 tags (zero concepts created)
- **THEN** one default BoardConcept with name="事件" and status='active' SHALL be created

#### Scenario: Some viable clusters exist, no default needed
- **WHEN** bootstrap runs for keyword category and 3 clusters pass the >=5 filter
- **THEN** 3 pending concepts SHALL be created; no default concept SHALL be created

#### Scenario: Default concept embedding generated
- **WHEN** a default concept is created
- **THEN** its embedding SHALL be generated from its name and description via the embedding API, enabling immediate MatchTagToConcept

### Requirement: Bootstrap minimal tag threshold
If a category has fewer than `bootstrapMinTags` (10) tags with semantic embeddings, bootstrap SHALL return nil without creating any concepts. This threshold takes precedence over both cluster filtering and default concept fallback.

#### Scenario: Too few tags returns empty
- **WHEN** bootstrap runs for person category with only 8 tags having embeddings
- **THEN** bootstrap SHALL return nil; no concepts (including default) SHALL be created
