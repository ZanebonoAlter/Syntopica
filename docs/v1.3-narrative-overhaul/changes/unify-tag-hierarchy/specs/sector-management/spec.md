## ADDED Requirements

### Requirement: Sector generation — auto mode
The system SHALL automatically generate Sector (board_concept) entries when a category's unplaced Tag count exceeds `auto_sector_threshold` (configurable, default 15). The system SHALL collect all unplaced Tags' embeddings, invoke LLM to propose Sector labels and descriptions via structured output (JSON schema passed as parameter, not prompt text), and create Sectors whose embedding similarity to any existing active Sector is below 0.85.

#### Scenario: Auto generation triggered by unplaced threshold
- **WHEN** TagHierarchyPlacementScheduler runs and finds 20 unplaced Tags in category "event" with auto_sector_threshold=15
- **THEN** the system SHALL invoke LLM to propose Sectors and create any that pass the 0.85 dedup threshold

#### Scenario: Duplicate proposal filtered
- **WHEN** LLM proposes Sector "AI产品" but existing active Sector "AI产品与工具" has cosine similarity 0.91
- **THEN** the system SHALL skip creating "AI产品" and log the dedup event

#### Scenario: Below threshold no action
- **WHEN** TagHierarchyPlacementScheduler runs and finds 8 unplaced Tags in category "event" with auto_sector_threshold=15
- **THEN** the system SHALL NOT trigger auto Sector generation

### Requirement: Sector generation — LLM mode
The system SHALL support user-triggered LLM Sector regeneration. The LLM SHALL receive the current Sector list (with source and Tag count), the current hierarchy tree structure, and recent Tag distribution trends. The LLM SHALL output incremental suggestions: keep, add, merge, or split. The system SHALL present a diff preview (affected Tag count + summary of changes) for user confirmation before execution. Protected Sectors SHALL only appear as "keep" or "split" suggestions.

#### Scenario: LLM suggests new Sector
- **WHEN** user clicks "LLM 重新生成板块" for category "event" and LLM suggests adding Sector "AI开源"
- **THEN** the system SHALL display diff: "保留 3, 新增 1, 受影响标签 22" and wait for user confirmation

#### Scenario: LLM suggests merging Sectors
- **WHEN** LLM suggests merging auto Sector "技术架构" (8 tags) into manual Sector "AI产品" (32 tags)
- **THEN** the system SHALL display the merge in the diff preview and migrate all Tags from "技术架构" to "AI产品" upon confirmation

#### Scenario: Protected Sector cannot be deleted by LLM
- **WHEN** LLM suggests deleting manual Sector "AI产品" (protected)
- **THEN** the system SHALL override the suggestion to "keep" and display the protected status in the diff preview

#### Scenario: User rejects LLM suggestions
- **WHEN** user clicks "取消" on the diff preview
- **THEN** no Sectors SHALL be modified and the operation SHALL be discarded

### Requirement: Sector generation — manual mode
The system SHALL allow users to create a Sector by providing label (required) and description (optional). If description is not provided, the system SHALL invoke LLM to generate one based on the label. The system SHALL generate embedding for the Sector. The Sector SHALL be marked as `protected=true` and `source='manual'`.

#### Scenario: Manual Sector with description
- **WHEN** user creates Sector with label="AI安全" and description="AI安全与对齐相关话题"
- **THEN** the system SHALL create a board_concept with protected=true, source='manual', and generate embedding from label+description

#### Scenario: Manual Sector without description
- **WHEN** user creates Sector with label="AI开源" and no description
- **THEN** the system SHALL invoke LLM to generate description, then create the Sector with protected=true, source='manual'

### Requirement: Sector deletion rules
The system SHALL enforce deletion rules based on Sector source: auto-created Sectors with zero Tags SHALL be deleted during health check; LLM-created Sectors whose Tag count has declined by ≥50% from peak SHALL be marked as "declining" (not deleted); manual Sectors SHALL NOT be deleted or marked by automated processes — only user manual action.

#### Scenario: Auto Sector with zero Tags deleted
- **WHEN** health check runs and auto Sector "技术架构" has 0 associated Tags
- **THEN** the system SHALL DELETE the Sector

#### Scenario: LLM Sector with declining Tags marked
- **WHEN** health check runs and LLM Sector "AI商业" had peak 18 Tags and now has 7 Tags (61% decline)
- **THEN** the system SHALL mark the Sector as "declining" and NOT delete it

#### Scenario: Manual Sector never auto-deleted
- **WHEN** health check runs and manual Sector "AI产品" has 0 Tags
- **THEN** the system SHALL NOT modify the Sector

#### Scenario: User manually deletes any Sector
- **WHEN** user clicks delete on Sector "AI商业"
- **THEN** the system SHALL DELETE the Sector regardless of source or protection status

### Requirement: Sector data model
The `board_concepts` table SHALL add columns: `source` (TEXT: 'auto'/'llm'/'manual', default 'auto') and `protected` (BOOLEAN, default false). Manual mode SHALL set source='manual', protected=true. Auto mode SHALL set source='auto', protected=false. LLM mode SHALL set source='llm', protected=false for new Sectors.

#### Scenario: Auto-created Sector fields
- **WHEN** a Sector is created via auto mode
- **THEN** source='auto' and protected=false

#### Scenario: Manual-created Sector fields
- **WHEN** a Sector is created via manual mode
- **THEN** source='manual' and protected=true

### Requirement: Tag归属Sector
When a Tag has embedding ready and its category has active Sectors, the system SHALL compute cosine similarity between the Tag's semantic embedding and each active Sector's embedding. The Tag SHALL be assigned to the highest-similarity Sector if similarity ≥ 0.6 (configurable). Tags with no Sector match SHALL be marked unplaced.

#### Scenario: Tag matched to Sector
- **WHEN** Tag "GPT-5发布" has semantic embedding with similarity 0.82 to Sector "AI产品"
- **THEN** tag.concept_id SHALL be set to the Sector's ID

#### Scenario: Tag below threshold unplaced
- **WHEN** Tag "冷门事件" has max similarity 0.45 to all Sectors in its category
- **THEN** tag.concept_id SHALL remain NULL and Tag SHALL be marked unplaced
