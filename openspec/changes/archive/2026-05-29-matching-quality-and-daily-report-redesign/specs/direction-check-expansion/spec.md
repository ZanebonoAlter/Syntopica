# direction-check-expansion

## Summary

将方向校验从 `max_sim` 扩展到 `hit_rate` 和 `weighted` 规则。`direct_hit` 因精确重叠已足够可靠，仍跳过方向校验。

## Behavior

### 方向校验执行时机

- 在 `evaluateSemanticBoardMatches` 的 switch 语句之后，对 `matchReason != ""` 且 `matchReason != "direct_hit"` 的结果统一执行方向校验
- 计算方式不变：`cosine(tagEmbedding, boardEmbeddings[boardID]) < config.DirectionSimThreshold → directionMismatch=true`
- 不影响 score，仅标记

### 不变项

- `SemanticBoardMatchConfig.DirectionSimThreshold` 仍为 0.5
- `tagEmbedding` 和 `boardEmbeddings` 加载逻辑不变
- `replaceTopicTagBoardLabels` 写入逻辑不变
- 现有方向校验测试 `TestEvaluateSemanticBoardMatches_DirectionCheck` 需补充 hit_rate/weighted 场景

## Test Cases

- `hit_rate` 匹配 + 方向校验通过 → `DirectionMismatch=false`
- `hit_rate` 匹配 + 方向校验不通过 → `DirectionMismatch=true`
- `weighted` 匹配 + 方向校验不通过 → `DirectionMismatch=true`
- `direct_hit` 匹配 → 不执行方向校验（无论 embedding 有无）
