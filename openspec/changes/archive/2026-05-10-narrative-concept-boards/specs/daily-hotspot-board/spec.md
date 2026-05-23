## ADDED Requirements

### Requirement: Large abstract tree becomes daily hotspot board
Abstract trees with node count ≥ N (default N=6) SHALL each generate a standalone "daily hotspot" NarrativeBoard on the target date. The Board's `name` SHALL equal the abstract tag's `label`, `is_system` SHALL be true, and `board_concept_id` SHALL be NULL.

#### Scenario: Large tree creates hotspot board
- **WHEN** abstract tree "AI行业动态" has 20 child event tags active on 2026-05-01
- **THEN** a NarrativeBoard is created with name="AI行业动态", is_system=true, board_concept_id=NULL, containing all 20 event tags

#### Scenario: Small tree does NOT create hotspot board
- **WHEN** abstract tree "开源工具" has 3 child event tags active
- **THEN** no hotspot board is created; the tree is routed to embedding matching instead

#### Scenario: N threshold is configurable
- **WHEN** ai_settings has narrative_board_hotspot_threshold=5
- **THEN** abstract trees with ≥5 nodes are treated as large trees

### Requirement: Hotspot board cross-day continuation
Daily hotspot boards SHALL link to yesterday's hotspot boards via `prev_board_ids` when they share the same `abstract_tag_id`.

#### Scenario: Same abstract tree on consecutive days
- **WHEN** on 2026-05-01, hotspot board "AI行业动态" has abstract_tag_id=201 with id=50; on 2026-05-02, the same abstract tree is active
- **THEN** the new hotspot board's prev_board_ids includes 50

#### Scenario: New abstract tree on first day
- **WHEN** an abstract tree first grows to ≥N nodes on 2026-05-01
- **THEN** its hotspot board has empty prev_board_ids

### Requirement: Hotspot board narratives continue across days
Narratives within hotspot boards SHALL link to yesterday's narratives from the same abstract tree via `parent_ids`, following existing continuation logic.

#### Scenario: Narrative continuation from yesterday's hotspot
- **WHEN** yesterday's hotspot board "AI行业动态" had narrative "GPT-5发布引发行业震动" (id=200)
- **THEN** today's narratives for the same hotspot board may reference parent_ids=[200] if they continue the same story
