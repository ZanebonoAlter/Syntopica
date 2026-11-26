# Human Feedback — board-health-triage-advice

> **定位**：记录用户在 change 生命周期内对产物（proposal/design/ui-design/原型/实现/文档）的反馈，作为本 change 修订依据；也是 harness-retro `--lifecycle` 体检卡「人类反馈段」的计量输入。人写给人的修订账本，不进 events.db（文件随 change 归档永久保存）。
> **消费方**：① 本 change 修订（条目 → 对应制品增量）；② harness-retro --lifecycle 计量（条数/归因分布/未处置数）；③ 归档对账（archive-readiness 顺带 feedback.sh verify——未处置条目提醒处置，不许静默丢）。
> **格式**（条目级契约，一行正则全解析）：条目标题行 `### FB-<N> <日期>【<类型>|<归因>】<标题>`；条目含状态行 `- **状态**：<状态>`。词表见 `bash scripts/harness/feedback.sh menu`。

## 反馈清单

### FB-1 2026-10-28【流程|process】用户要求先出可预览 UI 原型，确认后再进入正式实现；原 minor 合同未含原型。

- **状态**：已消化（已补交互原型；原型仍待用户确认，不代表允许开始正式实现）
- **原文要点**：用户要求先出可预览 UI 原型，确认后再进入正式实现；原 minor 合同未含原型。
- **已核实事实**：原 ui-design 为 minor / not-required / prototype=none；现有完整依据在 BoardWideWorkface 的 1040px 单级工作面，非新弹窗入口。
- **影响面**：本 change 的 ui-design.md、ui-prototype/ 与原型截图；无产品代码、API、数据或部署变动。
- **后续动作**：已生成单文件静态原型与可复跑机械验证，覆盖四类分诊、摘除确认/失败重试/成功、复制与历史降级。待用户明确确认后再开始 tasks.md，当前产品任务保持 0/25；不得把本条「已消化」视为用户批准。

### FB-2 2026-10-28【产品|frontend】原型仍像堆卡片，缺少优先级与处理后的下一步引导；需重审信息组织和操作主线。

- **状态**：已消化（v2 原型与扩围合同已补，仍待用户明确批准）
- **原文要点**：原型仍像堆卡片，后续引导不足。用户在结构化问题中明确选择「把描述修复也做成闭环」，同意先调整/拆分 OpenSpec 范围再出原型。
- **已核实事实**：现有 aux API 不支持描述修改；NULL 向量回填不更新已有向量；辅助标签全局共享；既有 backfill job 仅内存。不能只把复制改名为修复，也不能以本地置灰伪报全链完成。
- **影响面**：proposal/UI major 合同、描述修复新 delta、design D5/D7；旧 tasks/test-cases/data-map 明确标 v1 失效基线待批准后刷新。新增持久任务与迁移、缓存失效及匹配恢复属于正式实施范围；当前只改制品/原型。
- **后续动作**：已重做问题索引+单项核对/确认/三阶段进度/仅重试匹配/下一项的模拟原型；任务开始后编辑区收为只读折叠摘要，保留/不足/统计按需展开。42 项机械断言绿，视觉复查关键引导问题已修。现在请用户明确批准；不能把反馈已消化、模拟验证或扩围选择当作原型批准，不能声称真实修复后端已实现。

### FB-3 2026-10-28【产品|frontend】用户明确回复「这个可以」，批准v2问题处理与完整描述修复原型；随后允许实现派zai/glm-5.3 max。

- **状态**：已消化（批准已记录；正式产品尚未交付）
- **原文要点**：用户明确回复「这个可以」，批准v2问题处理与完整描述修复原型；随后允许实现派zai/glm-5.3 max。
- **已核实事实**：ui-design marker已更新major/approved；用户明确接受v2而非v1，允许进入正式实现。实现模型全称采用zai-coding-cn/glm-5.3:max。
- **影响面**：v2实施任务29项、完整故事用例/旧资产继承/白盒分支与实现接缝已重排；不改变已批准交互或单独放宽域红线。
- **后续动作**：按seam串行单写者实现与验证，逐任务全通过才勾选；不拿原型模拟或此次批准冒充实际功能完成。

### FB-4 2026-10-29【流程|process】GLM因5小时限额暂停后，用户明确选择「换minimax-3.1 flash max做」；本机登记模型minimax-cn/MiniMax-M3.1-Flash，按max续做原B2检查点。

- **状态**：已消化（模型选择与续做边界已记录，不代表功能完成）
- **原文要点**：GLM因5小时限额暂停后，用户明确选择「换minimax-3.1 flash max做」；本机登记模型minimax-cn/MiniMax-M3.1-Flash，按max续做原B2检查点。
- **已核实事实**：本机models.json登记provider=minimax-cn、id=MiniMax-M3.1-Flash、reasoning=true，旧会话registry未识别minimax-cn；commandcode尝试随后401且未执行工具。用户再次确认应使用minimax-cn/MiniMax-M3.1-Flash:max，现已固定该provider，不再以commandcode替代；用户已reload，acbe3a20无工具连通核验成功收到MINIMAX_CN_READY，现可用指定provider接续。旧GLM线程因429停止，B2部分源码已保留，当前门禁绿但2.2尚未交付。
- **影响面**：仅后续实现模型选择；v2 UI批准、业务范围、三P2修复与验收门禁不变，原已通过测试不重复跑。
- **后续动作**：使用原生新实现线程读seam-b2-api-checkpoint.md接续；保留原代码与外部脏改，不换CLI、不重做A/A2/B1、不私自降低thinking或宣称整批完成。
