# Review: guard-tag-exemplar-leakage

> 审查方式：主线程聚焦 review 已提交 diff（commit `e9c76760`），2026-10-28。审查范围：`tagmanagement/service/core/exemplar_guard.go`、`extractor_enhanced.go` / `extractor_section.go` 挂点、`cmd/admin` tagguard-cleanup、两份 prompt 的示例词集合与防漂移测试、真库清理口径。重点按「精读高风险文件 + grep 验证不变量」走：可空列扫描、SQL 参数化、集合与 prompt 同源方向、清理误删边界。

### R-1 2026-10-28【Medium】tagguard-cleanup 原始 SQL 未 `COALESCE` 可空列，NULL 扫描会让 dry-run 直接报错
- **状态**：已修复
- 依据：`articles.content` / `articles.firecrawl_content` 在库里可空（`information_schema` 实测 `is_nullable=YES`），查询把两列直接 `Scan` 进 Go `string`，`database/sql` 遇到 NULL 会报 `converting NULL to string is unsupported`，整次 dry-run 中断（失败是"叫出声"，不会误删，但工具在真实数据下会不可用）。
- 处置：查询改 `COALESCE(a.content, '') AS content` / `COALESCE(a.firecrawl_content, '') AS firecrawl_content`，并在上方注释写明原因；复跑 `go vet ./cmd/admin/` + `go build ./cmd/admin/` 零错误，真库 dry-run 正常输出（10 链接 / 待删 1 / 保留 9，见 tasks V7）。当前库里 exemplar 关联文章恰好无 NULL，故此前真库核对未暴露该分支。

### R-2 2026-10-28【Low】prompt 正例里的 aliases（"K8s"、"央行加息"）不在示例词集合，别名照抄不会被护栏拦截
- **状态**：已驳回（不修）
- 依据：`buildSectionExtractionPrompt` 的输出正例含 `"aliases":["K8s"]` 与 `"aliases":["央行加息"]`；`sectionPromptExemplarLabels` 只收主 label（Kubernetes / 某国央行上调基准利率）。防漂移测试 `TestExemplarLabelsCoverPrompts` 也只断言"集合 ⊆ prompt"，别名方向无覆盖，理论上模型照抄别名可绕过护栏。
- 驳回理由：① spec 把护栏对象定义为「prompt 示例词照抄」，别名不是示例标签本身，且别名（K8s 是 Kubernetes 通用缩写）本身是合理关键词，按「无提及即丢弃」处理会误伤正常语义；② 真库核对 `select id,label from topic_tags where label in ('K8s','央行加息')` → 0 行，未见实际泄漏；③ 扩集合会把维护面从"示例标签"扩到"示例标签+别名"，收益未证。留作观察项：若真实数据出现别名级泄漏再单开 change。

### R-3 2026-10-28【Low】tasks T5 措辞「清理工具判定函数单测」与落点不完全一致（无 CLI 层测试文件）
- **状态**：已修复
- 依据：`cmd/admin` 目录只有 `main.go`，无测试文件；实际被单测覆盖的是共享判定函数 `IsPromptExemplarWithoutEvidence` / `guardExemplarLeakage`（`exemplar_guard_test.go` 11 例），CLI 的 dry-run/apply 幂等靠真库人工核对（tasks 3.2/3.3）。
- 处置：不改代码（判定同源已由单测守住，CLI 层只剩 I/O 编排）；在 tasks 验证节补 V7 与 Scenario 映射表，把「判定单测 + 真库核对」的落点写实，避免把人工核对记成自动化覆盖。

- 审毕无其他发现（未发现误删风险：清理语料取 title+content+firecrawl 宽于提取可见输入，判定只会更保守；SQL 候选用 `IN ?` 参数化，无拼接注入面；两条提取出口都在持久化前过滤）。
