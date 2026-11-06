# Seam A 交接 — 体检信号与分诊

## 交付范围

任务1.1—1.4已实现。原worker因超时未交接，父线程读取恢复后的只读盘点、核对原始成功测试日志与恢复hash证据后补记；不重新实现、不把原型测试当产品测试，也未为外部门禁重复执行。

- 主实现run：54c66c00-127b-4d00-9029-3be2ad273406，model=zai-coding-cn/glm-5.3:max。
- 恢复后只读盘点run：9bc43d6e-a961-4ba9-b43a-ba45ef343aab，报告保存于该run的managed输出。
- 误stash恢复事实、固定对象/备份/98路径核验见 recovery-2026-10-28.md；未跟踪signals.go与兼容测试和备份字节相同。

## 文件与JSON接缝

`models/board_health.go`、`models/board_health_compat_test.go`；`service/boardhealth/boardhealth.go,config_test.go,signals.go,metrics.go,metrics_test.go,repo_pg_test.go,llm.go,llm_test.go,run.go,run_test.go`；handler原模型透传逻辑无需改动，仅扩 `handler/board_health_handler_test.go`。

- `metrics.triage`: `[{aux_id,category,evidence,suggestion?}]`，四类remove/rewrite/keep/insufficient；非rewrite不输出suggestion。历史/降级为null，前端须归一为空数组。标签名通过metrics.labels按aux_id映射，不在triage复制。
- `metrics.board.magnet_flow`: `{failed_tags,flow_tags,top_board:string|null,top_tags}`；无失败整体nil，有失败但没有可靠对手则top_board=null。
- `metrics.labels[]`: 原分布不变，加 `description?`、`magnet`、`neighbors`、`desc_flags`。magnet=`{all_mean,failed_mean,all_samples,failed_samples,suspicious,insufficient}`；均值可null。neighbors=`[{aux_id,label,cosine}]`；缺自身向量时nil，不能冒充零相似度。
- 配置规范名称沿当前体检已有前缀：`board_health_magnet_gap` 默认0.08、`board_health_magnet_min_failed` 默认5。design D2结论本就采用这两个名称，父线程仅修正其落点中残留的config.go/semantic_board_health_*旧引用；其余四个既有board_health配置不改。

## 算法与降级

- 磁铁复用matchedRows+identity向量，两侧均值差>=gap且全量/失败两侧样本够；不按辅助挂载数筛样本，不做单标签吸引因果归因。
- pgvector active auxiliary top3排自己/disabled/非aux/NULL向量；SQL测试使用golden schema实际维度（topic_tag_embeddings为4096），不改业务schema适配测试。
- 流向只采失败tag在其他active版块的有效(direction_mismatch=false)挂载；无可靠流向显式未知。
- 固定词根预筛，空描述未命中为空；loadCompositionLabels带出description。
- 同一次LLM调用产score/comment/notify/triage。完整标签清单保留，仅额外四行证据块按60截断，磁铁/描述预筛/低位次优先。
- aux_id未知/重复/漏覆盖、非法分类、缺证据或rewrite缺suggestion时整体裁决降级；不伪造补齐。score/comment/triage共同为空，统计与信号照常持久化、轮次继续。
- 提示词保护短名、绝对健康与合理多归属；真正LLM效果仍待任务5.4核对，不能凭桩模型宣称短名零误判已实证。
- boardhealth未引入board依赖；体检/GET仍不写匹配、升级建议或标签。

## 已执行验证（原始日志已核对，不重跑）

以下均在backend-go目录，原worker误stash前已完成，非short PG包含在内：

1. `GOMAXPROCS=2 go test -short -count=1 -p 1 -run 'TestSystemPrompt|TestValidateTriage|TestBuildScorePrompt|TestComputeMagnet|TestScreenDescription|TestComputeLabelAttribution|TestBoardMetrics' ./internal/tagmanagement/service/boardhealth/` → ok。
2. `GOMAXPROCS=2 go test -count=1 -p 1 -run 'TestQueryAuxNeighbors|TestLoadNeighborsForLabelsVectorMissing|TestLoadFailedFlow|TestCollectBoardStatisticsSignals|TestCheckupRoundPersistsSignalsAndTriage' ./internal/tagmanagement/service/boardhealth/` → ok 6.453s。
3. `GOMAXPROCS=2 go test -count=1 -p 1 -run 'TestGetBoardHealthCheckup' ./internal/tagmanagement/handler/` → ok 0.048s。
4. `GOMAXPROCS=2 go test -count=1 -p 1 -run 'TestBoardHealthMetricsJSONCompat' ./internal/tagmanagement/models/` → ok 0.029s。
5. `GOMAXPROCS=2 go test -p 1 -count=1 ./internal/tagmanagement/service/boardhealth ./internal/tagmanagement/handler ./internal/tagmanagement/models` → 三包ok（10.494s / 8.844s / 13.772s），包含真实临时testcontainer PG。
6. `golangci-lint run ./internal/tagmanagement/service/boardhealth/... ./internal/tagmanagement/handler/... ./internal/tagmanagement/models/...` → 0 issues。
7. `go build ./...`、影响三包go vet → 原worker报告成功；恢复后用户回传自动lint/vet/build与三个域short门禁绿。

中途测试夹具错误已修正后通过；没有把早期红误当当前红。恢复前后源内容固定hash一致，未重复执行已通过的三包PG测试。后续若seam B改动公用models/handler，会按新影响范围做必要增量验证，属于新改动回归而非为旧外部门禁重跑。

## 待后续执行

seam B持久修复任务/原子保存/匹配恢复、seam C真实UI、聚焦review、真LLM效果、部署及reference尚未完成；此交接不代表整个change完成。不动无关board_health_migration_test.go的存量格式，不暂存/清理其他change，不再允许子线程Git写命令。

## A2 增量回归（2026-10-28，review Low 两修）

独立审查A-L1/A-L2已修复（详见 review.md 同编号行与 seam-a2-handoff.md）：

- `boardhealth.go parseFloatDefault` 增非有限值守卫（`math.IsNaN/IsInf`）：NaN（大小写/带符号/首尾空白）与 ±Inf/infinity 一律回退传入 fallback；正常 0/100 边界与越界/坏文本回退行为不变。
- `run.go convertTriage` 仅在持久化转换时清空非 rewrite 类别的 Suggestion（rewrite 保留原文案）；类别/证据不因此丢弃，输入 LLM 结果 slice 不被改写。
- 新增纯函数回归 `service/boardhealth/review_regression_test.go`：`GOMAXPROCS=2 go test -p 1 -count=1 -run 'TestParseFloatDefaultNonFiniteGuard|TestConvertTriageClearsNonRewriteSuggestion' ./internal/tagmanagement/service/boardhealth` → ok；`golangci-lint run ./internal/tagmanagement/service/boardhealth/` → 0 issues。未重跑原三包PG/short门禁（既有绿不重复执行）。
