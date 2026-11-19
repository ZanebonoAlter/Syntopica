# 审查范围：stage-driven-constraint-injection

只读审查本 change 的制品与相关 diff，不纳入并发业务改动。先读 `AGENTS.md`、本目录 `proposal.md`、`stage.md`、`design.md`、`tasks.md`、`test-cases.md` 和两份 delta spec，随后查看 `.pi/constraint-injection.json`、`.pi/extensions/` 本次修改、`scripts/harness/` 本次修改及对应参考文档。目标路径无业务域红线命中，不补造摘要；用例设计以 `docs/reference/standard/shared/test-design.md` 为准。

重点：阶段来源及迁移后现有门禁是否保持；同族切换、跨 change/父子会话、压缩后文档路由和去重是否正确；是否仍有整节回退，与自主读取契约是否冲突；归档是否把失败/局部读取或其他 change/阶段的读取当成本 change 实现期证据；声明解析两个入口是否一致；测试是否实质验证场景。请用文件与行号、影响和建议列出 High/Medium/Low，区分已确认与待验证，不修改文件、不跑全量、不归档。若只读检查有无法验证的事项，显式列出。
