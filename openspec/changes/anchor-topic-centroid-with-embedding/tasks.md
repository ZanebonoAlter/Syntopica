# Tasks：anchor-topic-centroid-with-embedding

## 1. 实现混合公式与配置

- [x] 1.1 `PersistentTopicConfig` 增 `CentroidAnchorWeight float64`（默认 0.5），`LoadPersistentTopicConfig` 增 case `persistent_topic_centroid_anchor_weight`（ParseFloat，[0,1] 外回退默认+Warnf）
- [x] 1.2 抽纯函数 `blendCentroidWithAnchor(mean, anchor []float64, alpha float64) ([]float64, bool)`（维度不匹配/空首义返回 false）
- [x] 1.3 改造 `ComputeTopicCentroid`：滚动均值后加载首义做 α 混合；失败回退纯滚动+Warnf；<2 section 退化首义不变
- [x] 1.4 函数单测：blend（α=0/0.5/1、维度不匹配、空首义、零向量均值退化）+ config 解析边界（默认/0.3/0/1/非法）

## 2. 存量重算端点

- [x] 2.1 `POST /api/daily-reports/recompute-topic-centroids?board_id=<可选>`：参数校验（非法 400）、异步 goroutine（30min timeout，cancel-in-goroutine 模式）、逐话题 `UpdateCentroidOnSectionChange` 失败不中断
- [x] 2.2 handler 测试：无参/带 board_id/非法 board_id 三态 + 触发不阻塞

## 3. 测试

- [x] 3.1 更新 `daily_report_centroid_test.go`：混合断言（元素级 α=0.5）、有界漂移数值断言、退化/幂等/畸形维度话题不中断用例
- [x] 3.2 `daily_report_lane_integration_test.go` 既有「质心增量更新」用例断言改混合公式

## 4. 文档

- [x] 4.1 `docs/reference/flow/topic-graph.md` 质心小节（首义+滚动公式、α 配置、recompute 端点）+ 红线 17 + 代码入口
- [x] 4.2 `docs/reference/flow/daily-report.md` 锚点公式 2 处同步（加粗红线句不动）；`docs/reference/configuration.md` 新 key

<!-- doc-impact: flow, configuration -->

## 5. 验证

- [x] 5.1 `cd backend-go && golangci-lint run ./internal/topicgraph/... && go vet ./internal/topicgraph/...` → 0 error
- [x] 5.2 `cd backend-go && go test ./internal/topicgraph/repository ./internal/topicgraph/handler -count=1` → 全绿（含 testcontainer PG 用例）
- [x] 5.3 真库只读模拟（效果核对）：α=0.5 混合质心 → tag 268293 距离 0.1661→0.1868（>0.18 出直连区降级 L2）、创建时 5 条 section 0.1016→0.0351（拉回锚定）、tag 268151 0.1112→0.1247（仍 L1 未误伤）
- [ ] 5.4 部署后（见完工汇报）：记录基线（1009 质心到 268293/创建5条/最近5条距离）→ `curl --noproxy '*' -X POST localhost:5100/api/daily-reports/recompute-topic-centroids` → psql 复查三组距离符合 5.3 预期
