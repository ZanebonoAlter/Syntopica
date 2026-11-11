<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## N/A Reason

本 change 是**纯后端算法与运营端点**：

- 质心混合公式改动全部发生在 `backend-go/internal/topicgraph/`，不产生任何界面元素；匹配结果经既有 lane/section 展示链路呈现，展示结构不变
- 新增 `POST /api/daily-reports/recompute-topic-centroids` 为运营触发端点（curl 调用），与既有 backfill-* 端点族同等待遇，无 UI 入口
- 不触碰 `front/`

`ui-impact: none` 与本 change 的实际改动面一致：产出为质心计算行为契约与配置项。
