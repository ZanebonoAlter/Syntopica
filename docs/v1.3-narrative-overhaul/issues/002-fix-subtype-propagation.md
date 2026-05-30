# #2 - 修复 SubType 字段在提取→存储链路中丢失

## What to build

LLM 提取标签时会输出 `sub_type` 字段（`ExtractedTag.SubType`），`validateSubType()` 也有 fallback 逻辑（未知 → "concept"）。但在 `resolveCandidate()` 将 `ExtractedTag` 转为 `TopicTag` 时漏掉了 SubType 赋值，导致存库时 `topic_tags.sub_type` 为 NULL。同时 `findOrCreateTag()` 中所有 existing-tag 命中路径的 UPDATE 也都没有包含 SubType。

修复这两处，确保 SubType 在提取→转换→存库全链路中不丢失。

## Acceptance criteria

- [ ] `resolveCandidate()` 的 `TopicTag` 构建包含 `SubType` 字段
- [ ] `findOrCreateTag()` 中以下路径的 existing-tag UPDATE 包含 SubType：
  - exact match（line ~100）
  - event fallback（line ~166）
  - merge path（line ~217）
  - slug fallback（line ~270）
- [ ] 新建标签时 `SubType: tag.SubType` 已存在（line ~301，确认无遗漏）
- [ ] `go build ./...` 通过，`go test ./internal/domain/tagging/...` 通过
- [ ] 数据重建后 `topic_tags.sub_type` 不再为 NULL

## Blocked by

None - can start immediately（与 #1 并行）。
