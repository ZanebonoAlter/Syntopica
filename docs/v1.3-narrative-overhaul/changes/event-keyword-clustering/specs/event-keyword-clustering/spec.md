## ADDED Requirements

### Requirement: Keyword-overlap edge discovery
The system SHALL compute pairwise keyword overlap between unclassified event tags by counting shared elements in their `event_keywords` metadata arrays. Only pairs with shared keyword count >= `event_cluster_kw_min_overlap` (default 2) SHALL be retained as candidates.

#### Scenario: Two event tags share 2 keywords
- **WHEN** tag A has keywords ["特朗普", "北京", "车队", "抵达", "酒店"] and tag B has keywords ["特朗普", "北京", "欢迎仪式", "人民大会堂", "访华"]
- **THEN** the pair (A, B) SHALL be retained with shared_kws=2

#### Scenario: Two event tags share only 1 keyword
- **WHEN** tag A has keywords ["美国", "万斯", "伊朗", "核武器", "重申"] and tag B has keywords ["美国", "航空燃油", "出口量", "创新高", "EIA"]
- **THEN** the pair (A, B) SHALL be excluded (shared_kws=1 < threshold 2)

#### Scenario: Two event tags share no keywords
- **WHEN** tag A and tag B have no common keywords in their event_keywords arrays
- **THEN** the pair SHALL be excluded

### Requirement: Semantic filter on keyword candidates
The system SHALL filter keyword-overlap candidate pairs by computing semantic embedding cosine similarity. Only pairs with similarity >= `event_cluster_sem_threshold` (default 0.80) SHALL proceed to clustering.

#### Scenario: Keyword candidates pass semantic filter
- **WHEN** a keyword-overlap candidate pair has semantic similarity 0.85
- **THEN** the pair SHALL be retained for clustering

#### Scenario: Keyword candidates fail semantic filter
- **WHEN** a keyword-overlap candidate pair has semantic similarity 0.69
- **THEN** the pair SHALL be excluded

### Requirement: Event-only two-stage clustering
The system SHALL apply the keyword-overlap + semantic-filter two-stage clustering ONLY to tags with category "event". Other categories (keyword, person) SHALL continue using the existing semantic-only `FindSimilarTagsAmongSet` approach.

#### Scenario: Event category uses two-stage clustering
- **WHEN** `ClusterUnclassifiedTags` is called with category="event"
- **THEN** it SHALL use keyword-overlap edge discovery followed by semantic filter

#### Scenario: Non-event category uses semantic-only
- **WHEN** `ClusterUnclassifiedTags` is called with category="keyword"
- **THEN** it SHALL use the existing `FindSimilarTagsAmongSet` semantic-only approach

### Requirement: Connected component grouping
The system SHALL group filtered pairs into connected components (via BFS/DFS on the similarity graph) and submit each component of size >= 2 to `ExtractAbstractTag` for LLM judgment, following the existing cluster processing pipeline.

#### Scenario: Three tags form a connected component
- **WHEN** tag pairs (A,B), (B,C) pass both stages
- **THEN** tags A, B, C SHALL form one connected component and be submitted together to LLM judgment

#### Scenario: Isolated pair
- **WHEN** only pair (A,B) passes both stages with no other connections
- **THEN** tags A, B SHALL form a component of size 2 and be submitted to LLM judgment

### Requirement: Configurable thresholds
The system SHALL read clustering thresholds from `embedding_config`:
- `event_cluster_kw_min_overlap` (default 2): minimum shared keyword count
- `event_cluster_sem_threshold` (default 0.80): minimum semantic similarity for stage 2
- Existing `cluster_max_tags` and `cluster_max_size` SHALL apply unchanged.

#### Scenario: Custom thresholds from config
- **WHEN** `embedding_config` contains `event_cluster_kw_min_overlap`=3 and `event_cluster_sem_threshold`=0.85
- **THEN** the system SHALL use these values instead of defaults

#### Scenario: Missing config uses defaults
- **WHEN** `embedding_config` has no `event_cluster_kw_min_overlap` entry
- **THEN** the system SHALL use default value 2
