# 规划核对记录

## 范围与当前状态

仅完成规划制品，没有修改业务代码、运行迁移、部署、真实LLM或外部检索。规划文件存在不代表实现完成，tasks全部未勾选。

- schema：syntopica-ui；proposal/ui-design/specs/design/tasks齐全。
- 10个Requirement故事，63个Scenario（含38个继承Scenario，新增25个），均有test-cases主链路与tasks测试落点。
- 仅计划新增两个测试文件：service/signal_sources_test.go、service/signal_page_fetch_test.go；其余复用旧资产。
- UI minor：现有候选详情和报告附录内增字段/引用，不新增页面/导航；不需要major原型审批。
- 前置：redesign-enrichment-research-workbench完成并同步主spec后才apply；本change未归档、未实现。

## 已执行的规划检查

- `openspec validate add-question-driven-news-research --strict`：通过。
- `bash scripts/harness/check-tasks-tail.sh openspec/changes/add-question-driven-news-research`：通过（尾三节、doc-impact、Review任务齐备）。
- 两个capability的test-assets.sh反查：已完成；前置change未同步部分额外按其delta核对。
- 机械核对：63个Scenario标题唯一且全部出现在测试故事和任务映射；10故事；38旧Scenario逐行处置；25个任务未勾选；规划文件无行尾空格且均以换行结束。
- `scenario-trace.sh`：**未通过实施期文件存在性检查**，仅缺上述两个计划新增测试文件；无Scenario漏映射。不创建空测试或将计划伪装成人工验收来凑绿，实施后再跑，记录在任务6/8及待办中。
- 未运行产品单测、临时PG、浏览器验收或真实效果评测；相关结果不得标为通过。

## 自查中修正

- 查明Bocha已生产接线，未采信web_search.go顶部过时的“无真实后端”注释。
- 现有readability抓取不经过safefetch，因此设计为signal专用安全适配，不直接暴露旧fetch实现。
- 主规格schema2落后于前置change，重叠MODIFIED条文保留其schema3/数值呈现契约。
- 发现快照新增正文后，研究prompt不得整体marshal快照，必须先给候选来源目录，实际读取后才交正文，避免“先读门”被正文预注入绕过。
- scenario-trace不支持混写测试路径与人工叙述，已将人工browser证据留在任务6.3/UI契约，映射列只写测试文件。

## 派发偏离

项目要求派发只读探索；explore档未被subagent发现，改用工具列出的scout仍因后台宿主不可用而启动失败。两次都未运行子任务。已向用户说明，主线程完成只读调查和规划，不绕其它CLI、不伪称独立审查。实现阶段Review仍是未完成任务。

## 部署影响（实施后）

新发现带问题简要说明；新研究可读冻结原文、使用已配置Bocha和安全页面读取，并明确答题范围。旧候选/报告不自动升级或补资料；使用新流程需重新发现。外搜配置缺失可本地解释或披露缺口，无新供应商必配项、无批量重算。现在未部署，用户无需操作。
