## Why

Tag hierarchy has degenerated into excessive abstraction layers with near-duplicate tags all pointing to the same small set of articles. For example, 5 articles about "DeepSeek首轮融资" are spread across 14 tags in a 4-level deep tree, where `DeepSeek首轮融资` and `DeepSeek 首轮融资` exist as separate active tags because a whitespace difference evaded slug dedup. LLM creates abstract tags without verifying that children cover meaningfully different article sets, leading to thin trees where each abstract node has only 1-2 children.

## What Changes

- **Normalize whitespace in slug generation**: Merge consecutive whitespace, trim, and ensure consistent separators so `deepseek首轮融资` and `deepseek 首轮融资` produce the same slug
- **Add information gain check before creating abstract tags**: Reject abstract tag creation when candidate children share >70% of their articles or when the proposed abstract would create a tree with leaf-count-to-depth ratio < 2 (thin trees)
- **Enforce minimum child count for abstract tags**: Require at least 3 distinct children (or 2 with sufficient article-set divergence) before creating a new abstract parent
- **Add cleanup task to fix existing data**: Merge whitespace-variant duplicates, flatten degenerate abstract layers (single-child parents promoted out), resolve multi-parent conflicts
- **Add multi-parent prevention**: Validate that assigning a tag to a new parent doesn't create a shared-subtree when alternatives exist

## Capabilities

### New Capabilities
- `tag-hierarchy-quality`: Enforces information gain thresholds and structural constraints on abstract tag creation, and provides scheduled cleanup to fix existing degenerate structures

### Modified Capabilities
<!-- None - this is a new quality gate on existing tagging behavior, not a requirement change to existing specs -->

## Impact

- **Backend Go**: `topicextraction/tagger.go` (slug normalization), `topicextraction/slug.go` (new normalize function), `topicanalysis/abstract_tag_judgment.go` (info gain check), `topicanalysis/abstract_tag_hierarchy.go` (multi-parent prevention), `tag_cleanup.go` (new cleanup tasks)
- **Data**: Existing `topic_tags` and `topic_tag_relations` will be cleaned up on first run of the new cleanup scheduler
- **No API changes**: Tags merged/flattened by cleanup are transparent to existing API consumers
- **LLM cost reduction**: Fewer abstract tag creations = fewer LLM calls during tag judgment
