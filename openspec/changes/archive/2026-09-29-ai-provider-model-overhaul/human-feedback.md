# Human Feedback — ai-provider-model-overhaul

> **定位**：记录用户在 change 生命周期内提出的产品/流程反馈，作为本 change 修订依据；同时是 harness 定期体检（`harness-retro`）的输入——人类回答沉淀为改进意见，用于流程优化与效果计量。
> **消费方**：① 本 change 修订（FB-1/2/3 → proposal/design/ui-design/原型 与 tasks 增量）；② harness-retro 定期扫描（FB-4 起的机制类反馈）；③ 归档时未消化条目须显式处置（转新 change / 标记放弃），不许静默丢。
> **格式**（design D7 条目级契约，一行正则全解析）：条目标题行 `### FB-<N> <日期>【<类型>|<归因>】<标题>`；条目含状态行 `- **状态**：<状态词>（括号说明可选）`。类型分 `产品`（改本 change 的东西）/ `流程`（改做事的方式与 harness）；归因五类封闭 `frontend | backend | test-gap | fact-check | process`（frontend=前端设计/交互/视觉；backend=后端业务/参数/数据；test-gap=测试该拦未拦；fact-check=探索或搬运时未核实就写，多因按根因优先归此类；process=流程/harness 本身）；状态五值封闭 `待处理 | 处理中 | 已消化 | 已转挂 | 已放弃`。词表详见 `bash scripts/harness/feedback.sh menu`。
> **补标说明**：2026-09-29 随 add-change-lifecycle-retro 8.2 补归因标注与状态词归一（条目与处置表不一致处以处置记录为准），存量小一次补齐不留推断口径。

## 反馈清单

### FB-1 2026-09-29【产品|fact-check】预设厂商目录应覆盖 pi-sdk 内置全部厂商，不是原型列出的那几家

- **状态**：已消化（预设目录扩容已随 tasks §8 实现并部署：44 预设 + 分组呈现；探查事实见正文）
- **原文要点**：「不是只接入原型中列出的厂商，而是接入 pi-sdk 中现有接入的那些」。
- **已核实事实**（2026-09-29，探查 pi-coding-agent dist）：
  - 内置 provider 清单 = `dist/core/model-resolver.js` 导出的 `defaultModelPerProvider`，共 **41 家**：
    `amazon-bedrock, ant-ling, anthropic, openai, azure-openai-responses, openai-codex, radius, nvidia, deepseek, google, google-vertex, github-copilot, openrouter, vercel-ai-gateway, xai, groq, cerebras, zai, zai-coding-cn, mistral, minimax, minimax-cn, moonshotai, moonshotai-cn, huggingface, fireworks, together, baseten, opencode, opencode-go, kimi-coding, meta, cloudflare-workers-ai, cloudflare-ai-gateway, qwen-token-plan, qwen-token-plan-cn, qwen-token-plan-individual, xiaomi, xiaomi-token-plan-cn, xiaomi-token-plan-ams, xiaomi-token-plan-sgp`
  - 各家的显示名/认证方式/环境变量：`docs/providers.md`（apiKey 主表 + 云厂商专节）；默认模型见 `defaultModelPerProvider`。
  - 现行实现预设目录仅：DeepSeek / 小米 MiMo / ZAI / OpenRouter / Ollama / llama.cpp / 自定义（7 项）。
- **影响面**：VendorDialog 预设目录常量扩到 pi 全集 + 保留 Ollama/llama.cpp/自定义（pi 清单里没有的本地项）；目录过长需分组呈现（云端直连 / 聚合网关 / 本地 / 自定义）；预设需携带 base_url + 思考方言 + 结构化输出档（pi 侧无现成结构化输出元数据，按厂商文档逐家标注或默认 `json_object` 待校准）。
- **后续动作**：设计增量（预设目录数据结构 + 分组 + 非覆盖家数回退自定义表单）→ ui-prototype 目录页重做 → 属信息架构级 UI 变更，**ui-approval 需重置**。

### FB-2 2026-09-29【产品|fact-check】厂商图标用 pi 现成资产，不自己搓

- **状态**：已消化（图标方案改用 pi provider-icons spritesheet，随 tasks §8 实现并部署：36 图标 + 首字母回退）
- **原文要点**：「图标可以用 pi-sdk 的而不是自己搓」。
- **已核实事实**：pi-web 自带 spritesheet `…/@agegr/pi-web/public/provider-icons.svg`（40KB，**30 个 symbol**）：`anthropic, openai, google, deepseek, groq, mistral, moonshot, minimax, fireworks, huggingface, cerebras, openrouter, xai, cloudflare, vercel, githubcopilot, aws, azure, kimi, qwen, zhipu, cohere, perplexity, together, grok, antgroup, nvidia, opencode, xiaomimimo, zai`（多家 lobe-icons 来源，另有 `lobe-icons-*` 辅助 symbol）。原实现已改用 Iconify mdi 预设图标 + 首字母回退。
- **影响面**：前端资产引入方式（拷贝该 SVG 进 `front/app/assets/` 作 spritesheet 内嵌，`<use href="#vendor-{id}">`）；41 家中无图标的 provider（baseten/meta/radius/opencode-go 等约 11 家）保留首字母回退；预设 id ↔ symbol id 需一张映射表。
- **后续动作**：随 FB-1 原型修订一起换图标方案；实现任务挂 tasks 4.x 增量。

### FB-3 2026-09-29【产品|frontend】能力路由交互重做：`qwen3.5-4b@qwen` 文字图标混杂、顺序交互乱，倾向 Dify 式卡片拖拽

- **状态**：已消化（拍板看板泳道式、经 FB-5 修正为行式泳道后随 tasks §8 实现并部署；原三选一记录——模型池 + 每能力一列卡片拖入/列内调序，否决 Dify 式画布与纯链式视觉优化）
- **原文要点**：「能力路由现在是抽象的例如 qwen3.5-4b@qwen 这种方式，文字图标混成一团，交互顺序一团乱麻，要不要参考类似 dify 的工作流的方式，实现卡片拖拽，这个涉及到原型调整」。
- **痛点拆解**：① 视觉——`model@vendor` 纯文字串拥挤无层次；② 交互——chips 点选追加 + drag/↑↓ 调序的组合链路学习成本高。
- **影响面**：**主交互范式变更 = ui-impact major 级**→ ui-design.md（User Journey/IA/Interaction Contract/Layout/Component Reuse 五节）+ ui-prototype 视图二重做 + **ui-approval 重置 pending，重走用户浏览器审批**；能力路由 section 深链（#capability-routes）与观测「定位」跳转锚点须保持；数据契约（route → [{model_id, thinking_level, priority}]）不受交互范式影响，后端零改动（若选画布连线且需存布局坐标，则另议）。
- **后续动作**：方案选定 → 更新 ui-design 五节 → 原型重做（含 1440/1920 双视口）→ 用户审批 approved → tasks 4.3 重做 + 回归用例（test-cases UI 节拍同步改）。

### FB-4 2026-09-29【流程|process】人类反馈应进入 harness 定期体检闭环，用于流程优化与效果计量

- **状态**：已转挂 → 用户指示在 `add-change-lifecycle-retro` change 中增加（retro `--lifecycle` 体检卡人类反馈段 + feedback.sh 三件套 + 制品清单制度化）；本文件继续作为 change 内反馈账本
- **原文要点**：「把我这次的反馈记录到 human-feedback.md 到 change 中，作为后续我要调整的规范依据 → 人类回答作为改进意见记录到 harness 定期体检，用于优化流程或者效果计量」。
- **诉求拆解**：① change 内：人类对产物（原型/实现/文档）的反馈落 `human-feedback.md` 条目化建账，修订有依据、归档有交代；② harness 层：这类反馈是「效果计量」的一手数据——哪个环节产物被用户打回（原型被否、实现被推翻、决策被质疑），就是 harness 检查点该优化的位置。
- **后续动作**（转挂 add-change-lifecycle-retro 后本 change 侧仅保留）：① `standard/shared/doc-authoring.md` + 开发执行规范 §0.6 把 human-feedback.md 纳入 change 可选制品清单（字段格式以本文件为准，可随转挂 change 一起做）；② retro 扫描与计量（消化率=已消化/(已消化+待处理)、打回率、origin 环节标注）归 add-change-lifecycle-retro。
- **计量口径建议**（供 retro 报表用）：消化率 = 已消化/(已消化+待处理)；打回率 = 触及 UI 审批/原型返工的 change 占比；每条反馈挂 `origin`（哪个制品环节产出：proposal/design/ui-design/实现/验证）。

### FB-5 2026-09-29【产品|frontend】能力路由再改横向泳道：每功能一行，降级状态内嵌行内

- **状态**：已消化（FB-3 形态修正：每能力一列 → 每能力一行；当日二次修订原型/文档，随 tasks §8 实现并部署）
- **原文要点**：「如果每个功能（分析日报等）占一行，作为一个泳道，把模型卡片拖过来，如果断掉了也能在这里体现（降级的可以在这体现）」。
- **方案**：行式泳道——每能力一行，行内自左向右 = 降级优先级（首选→备1→备2）；模型池置顶横排；**健康/降级内嵌**：失联供应商卡片红边 + 错误码 tooltip；近期失败卡标「24h 失败 N 次」；降级接管方标「接管 N 次」徽章；卡片间箭头即降级方向。数据源 /api/ai/health + /api/ai/observability/degradations（本 change 既有端点，后端零新增）。
- **与 FB-3 关系**：FB-3 拍板的「看板泳道 + 卡片拖拽」保留，形态修正为行式——纵向 8 列横向滚动易漏看，降级时序（从左到右）在行内才自然，且用户点名断链/降级要在路由界面上直接可见（此前需跳观测面板）。
- **影响面**：ui-design 五节再修（IA/Interaction/Layout 列→行）、原型视图二重做、tasks 8.3/test-cases 同步；approval 维持 pending；后端契约仍零改动（priority=行内序，不存坐标）。

### FB-6 2026-09-29【产品|frontend】供应商列表底部重复的「+ 新增供应商」按钮删除，只留 section 头右上角入口

- **状态**：已消化（原型当日修）
- **原文要点**：「第一个供应商与模型这里出现了两个新增供应商，要么只保留右上角的，不要有重复」。
- **影响面**：原型视图一删底部 .add-vendor 虚线按钮；实现时 AIProviderManagement 同样只保留 AppSectionHeader actions 位入口（空态引导文案里仍可引导到右上角）。

### FB-7 2026-09-29【产品|frontend】select 越界修复 + 点击添加通道（拖拽不是唯一路径）

- **状态**：已消化（原型当日修，用户已确认行式泳道方向「可以」）
- **原文要点**：「注意原型中比如 select 这些越界了，展示有点问题，而且也不是只能拖，手动点击应该也能选择 provider 加到泳道，因为最下面拖过去太复杂了」。
- **修复**：① 卡内溢出——泳道卡 172→208px + overflow:hidden，lc-foot 改 flex-wrap，select flex 收缩（min-width:0）+ 全局 select max-width:100%；② 点击通道——池卡带「＋」徽标，点击弹能力选择菜单（8 行列表，类型不匹配项置灰标「仅 embedding/llm」，已在行中标「已在行中」，点外/ESC 关闭），选中即追加行尾；池提示与空行引导同步改双通道文案。
- **影响面**：原型已修并验证（select 零溢出/菜单 8 项/追加成功/ESC 关闭）；ui-design Interaction Contract 主操作表补「点击添加」行；test-cases ②/U7 补菜单断言；实现时点击与拖拽同为一级通道（无障碍优势：键盘用户全程不依赖 DnD）。

### FB-8 2026-09-29【产品|fact-check】小米 Token Plan 预设 base_url 应为 token-plan 域名

- **状态**：已消化（当日修复：presets.ts 三家 token-plan baseUrl 改正 + 锚点断言）
- **原文要点**：「小米的url是token开头的，也就是 https://token-plan-cn.xiaomimimo.com/v1」。
- **核实**：pi 内置定义（chunk-OJP47DM6.js）——xiaomi-token-plan-cn/ams/sgp 分别为 `https://token-plan-{cn,ams,sgp}.xiaomimimo.com/v1`；标准 `xiaomi` 家保持 `https://api.xiaomimimo.com/v1`。初版预设四家全写成 api. 域名（生成脚本从原型 fixture 搬运时未区分）。
- **修复**：presets.ts 三家 token-plan baseUrl 改正 + presets.test.ts 加锚点断言。

### FB-9 2026-09-29【产品|frontend】供应商图标暗色模式看不清

- **状态**：已消化（当日修复：暗色主题 logo 容器浅底 + 首字母回退适配）
- **原文要点**：「供应商图标在暗色模式下看不清」。
- **根因**：pi provider-icons 为固定 fill 品牌色（deepseek 深蓝/openai 黑/mistral 黑等为白底设计），VendorLogo 容器底色用了 `--color-bg-sunken`（暗色主题下为深色 #0e161d），深底深图对比度不足。
- **修复**：暗色主题（html[data-theme='dark']）下 logo 容器改浅底（品牌 logo 按白底设计，浅底最保真）；首字母回退同色适配。

### FB-10 2026-09-29【产品|backend】默认超时 120s 太短，改 600s

- **状态**：已消化（当日修复：默认超时 120→600s 前后端同步，存量已配不动）
- **原文要点**：「默认超时120s太短了，600s吧」。
- **修复点**：后端 store_vendor.go 缺省值 120→600（store_vendor_test 断言同步）；前端 VendorDialog 三处初始/重置/回填缺省 120→600 + 保存 fallback。存量已配 vendor 的 timeout 不动（仅默认值变化）。

## 处置记录

| 日期 | 条目 | 动作 | 结果 |
|---|---|---|---|
| 2026-09-29 | FB-1/2/3 | 探查 pi 资源 + 三方案选项呈用户 | FB-3 拍板看板泳道式；FB-1/2 进入修订（proposal/design/ui-design/原型/tasks 增量） |
| 2026-09-29 | FB-4 | 建立 human-feedback.md 机制首例 | 本文件；转挂 add-change-lifecycle-retro 消费通道 |
| 2026-09-29 | FB-5 | 行式泳道二次修订（列→行 + 降级内嵌） | 已消化：原型/五节 ui-design/proposal/design D8-D9/test-cases/tasks §8 同步重做，截图 proto3-1440/1920 + dialog 三张，待用户浏览器确认（approval pending） |
| 2026-09-29 | FB-6 | 删底部重复新增按钮 | 已消化：原型删除；ui-design Interaction Contract 明确唯一入口在 section 头 actions 位 |
| 2026-09-29 | FB-7 | select 越界 + 点击添加通道 | 已消化：卡内 select 零溢出；池卡点击弹能力菜单追加行尾；截图 proto4-*；行式泳道方向获用户确认 |
| 2026-09-29 | 反馈修订轮原型 | 用户浏览器确认新原型（行式泳道 + 44 预设 + pi 图标 + 点击菜单） | ui-approval: approved；进入 tasks §8 实现 |
| 2026-09-29 | FB-1~7 实现 | tasks §8 六项全部完成并验证 | 107 测试全 PASS；部署后真页面冒烟过（44 预设/36 图标/8 泳道 10 卡/降级标注出数/无溢出）；卡级降级标注增加「结构吻合校验」防换绑错标（行头=能力级统计，卡标=与当前链吻合才落） |
| 2026-09-29 | FB-8/9/10 | 小米 token-plan 域名 / 暗色图标对比度 / 默认超时 600s | 三项均当日修复并部署（详见各条目） |
