## REMOVED Requirements

### Requirement: Source A abstract tag creation via LLM judgment
**Reason**: The `ExtractAbstractTag` → `processAbstractJudgment` path in `findOrCreateTag` (Source A) bypasses the concept fence and creates abstract tags with `concept_id=NULL`. This contradicts Decision 7 of `hierarchy-concept-fence` which mandates Source A only handles tag reuse/merging. Abstract creation is exclusively handled by `PlaceTagInHierarchy` (Source B) which operates within concept boundaries.

**Migration**: Abstract tags already created via this path remain in the database with their existing parent-child relations. The cleanup scheduler (`TagHierarchyCleanupScheduler`) continues to maintain them via merge/move/reuse. New abstract creation only occurs through `PlaceTagInHierarchy` after concept matching succeeds.
