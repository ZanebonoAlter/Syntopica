# Review：dedupe-thread-article-copies

> 审查执行：派 `change-review` 档只读子线程（brief：`/tmp/dedupe-review-brief.md`，12 项重点审查），产出中文审查报告（总评 / 问题清单 / 核实记录）。审查范围：change 六件制品 + 实现 diff（`article_identity.go` 新增、`daily_report_repository.go` 改、两份测试新增/改）+ flow/api 文档改动。
> 总评：实现与 spec 口径一致、红线 45 存储语义零改动、折叠口径无越界；2 项 Medium（1 项阻塞归档门禁）+ 8 项 Low，全部处置完毕后方归档。

### R-1 2026-11-13【Medium】映射表分隔符导致归档门禁 scenario-trace 失败
- **状态**：已修复（tasks.md 映射表改逗号分隔；`bash scripts/harness/scenario-trace.sh` → 9 个 Scenario 映射齐全、退出码 0）

### R-2 2026-11-13【Medium】spec delta 契约缺口：同一 article id 重复出现未写进契约
- **状态**：已修复（delta「读路径投影」段补「同一 article id 在数组内重复出现 SHALL 折叠为一条」并补 Scenario「同一 id 重复引用折叠为一条」；同步更新 tasks.md 映射表）

### R-3 2026-11-13【Low】design.md D3 错引红线 22
- **状态**：已修复（改为「红线 21 细节层『数组契约』：保序、去重以首个出现为准」）

### R-4 2026-11-13【Low】article_identity.go 头部注释错引红线 49 / story_signals
- **状态**：已修复（改为只陈述事实：归一规则与红线 49 的内容身份键同一实现，红线 49 现存服务面是簇内文章数与 related-events，story 线已退役）

### R-5 2026-11-13【Low】design.md 陈旧表述「目前只服务 story_signals/heat 层」
- **状态**：已修复（Context 段改为「现存服务面是簇内文章数与 related-events，story 线已退役」）

### R-6 2026-11-13【Low】tasks.md 2.3 验证命令基线错（`git diff --stat HEAD~1 -- front/`）
- **状态**：已修复（改为 `git diff --name-only HEAD -- front/` → 输出为空）

### R-7 2026-11-13【Low】tasks.md 5.3 是不可执行占位符
- **状态**：已修复（换成可直接粘贴的只读 SELECT 命令 + 期望值 34；并新增 5.4 接口层核对命令（curl + python3 解析，期望 `[[169400]]`））

### R-8 2026-11-13【Low】tasks.md 文档任务缺「— 验证：」措辞
- **状态**：已修复（3.1/3.2/3.3 补验证形态；1.1/1.2/1.3 的交叉引用改为三形态措辞）

### R-9 2026-11-13【Low】用例名夸大：「变体#13 跨天同 link 也折叠」暗示不存在的跨天语义
- **状态**：已修复（改名为「折叠与时间无关：同 link 即折叠，无时间判据」）

### R-10 2026-11-13【Low】变体 #11（悬空引用）/ #16（部分失败后重试）集成层覆盖不全
- **状态**：已修复（集成用例补 seed 悬空 article id → 原样保留不报错；补「注入查询失败后重试成功 → 已折叠」两段断言）

### R-11 2026-11-13【Low】加固建议：红线 45 的读路径例外只在细节层，域注入只取红线层
- **状态**：已采纳（红线 45 首个加粗句主语收紧为「落库值 `daily_report_threads.related_article_ids` 仍是 …，不被任何身份/去重逻辑改写」，例外段仍留细节层；实跑红线抽取确认第 45 条抽出句为收紧后的原句、语义未反转）
