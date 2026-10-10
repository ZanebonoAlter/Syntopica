# Tasks: fix-event-tag-time-anchor

## 1. Prompt 治根（A）

- [x] 1.1 `tagger.go generateTagDescription` event 分支：替换"伊朗袭击以色列 → 2024年4月…"few-shot 示例为时间中性/虚构示例；Description requirements 追加时间锚约束（以 Context `[日期: …]` 为参照、禁止引入 Context 外时间信息、同名历史事件以 Context 为准）。验证：`go test ./internal/tagmanagement/... -run TestGenerateTagDescription` 新增断言用例通过；`rg -n "伊朗" backend-go/internal/tagmanagement/service/core/` 无 prompt 命中
- [x] 1.2 `extractor_section.go buildSectionExtractionPrompt` 与 `extractor_enhanced.go buildMergedExtractionPrompt`：输出正例中的"伊朗袭击以色列"标签名换为中性/虚构事件。验证：`rg -n "伊朗袭击以色列" backend-go/` 仅剩测试断言或无命中；`go test ./internal/tagmanagement/...` 通过
- [x] 1.3 单测：新增/调整 prompt 内容断言（示例不含真实历史事件年月组合；时间锚约束句存在）。验证：`go test ./internal/tagmanagement/service/core/ -run Prompt` 通过

## 2. 存量修复 CLI（C）

- [x] 2.1 扫描器：event 标签 description 最早年份提取（Go 正则）+ 与 created_at 年份差 ≥1 筛选，输出候选清单（ID/label/created_at/description 预览）。验证：单测覆盖年份提取（多年份/无年份/跨年表述）与边界（差=1、差=0、无年份）
- [x] 2.2 修复命令：dry-run 默认出清单并导出原 description 备份 CSV；`--apply --ids` 显式圈选后按首挂文章 articleContext（`[日期: 发布日期] 标题. 摘要≤800字`，与 persistArticleTags 口径一致）重生成 description。验证：单测覆盖 ID 白名单过滤（非候选不触碰、重复执行幂等）；真库 dry-run 清单含已知标签 183730
- [x] 2.3 命令入口挂载到 admin cmd。验证：`go build ./...`；dry-run 帮助信息可运行

## 3. 验证与收尾

- [x] 3.1 门禁：`golangci-lint run ./...` + `go vet ./...` + 影响包 `go test ./internal/tagmanagement/...` + `go build ./...` 全绿
- [x] 3.2 效果核对（test-cases ④）：真库 dry-run 出清单 → 人工圈选病态 ID（至少含 183730 伊朗袭击以色列）→ `--apply` 重生成 → 抽样核验新 description 时间锚与首挂文章日期一致；再跑扫描确认已修复标签不再命中
<!-- doc-impact: flow -->
- [x] 3.3 文档：docs/reference/flow/semantic-board.md 业务约束节补充"event 标签 description 时间锚以首挂文章日期为准"红线句（若有标签描述相关小节）；跑 `bash scripts/harness/doc-impact.sh verify` 通过
