<!-- complexity: standard -->
<!-- ui-impact: none -->

# Proposal: add-dispatch-constraint-digest

## Why

约束注入通道全部以**流程身份**（`mode` = requirements / implementation）为开关：`planInjection` 里关键词命中（`matchText = mode ? ... : ""`）、编辑路径 JIT（`if (state.mode && ...)`）、域限定 `allowedKeywordDocs` 三道闸均在无档位时关闭，结果只剩 `constraints-index.md` 索引本身。而 `开发执行规范 §0.6` 的豁免清单**明确允许** hotfix / 单文件小修 / 纯配置不开 change——这类任务天然没有 opsx 身份，于是耦合出两个后果：

1. **主会话裸奔**：探索/修复语境提到「日报」「定时任务」也不触发对应域红线；改 `backend-go/...` 也不注入路径映射文档。
2. **子线程不可自愈**：子线程跑在独立 node 进程，`session_start` 从事实库继承父会话的 `mode + boundChange`（父为 null 则继承 null），随后 `deliverChildInitStable` 一次性冻结稳定层并置 `childInitDelivered=true`；pi-web 子线程 SDK 又没有 `before_agent_start` 事件，动态层的命中计算同样 gated on mode——子线程从头到尾只能拿到索引，且读多少代码都不会自愈。

现行唯一兜底是 AGENTS.md 那句「派发任务文本 MUST 带必读文件清单 + 相关域红线摘要」，但**没有任何取数方法**（怎么取、取哪几条、什么格式），实践中必然退化为凭印象写两行或直接省略。

本 change 采用**主线程主动注入 + 规范积极引导**路线：把这条空 MUST 变成一条命令 + 一个固定格式，并让"有没有做到"可量化。**不改任何既有注入行为**（不放开门槛、不动稳定层/继承链路），先跑观察窗口，用数据决定是否还需要插件兜底。

## What Changes

- **新增取数入口** `scripts/harness/constraint-digest.sh`：`--domains <域名列表>` 与 `--paths <目标路径列表>` 两种输入，输出域红线句（逐行）+ 细节层指引，控制在 5–10 行/域。红线提取**复用 constraint-injection 的 `extractRedlines` 同一实现**（esbuild bundle 既有模式），杜绝第二份正则；路径反查复用文档头部 `doc-impact-applies` 标签的扫描语义。
- **规范从空 MUST 改为三步可执行**：`AGENTS.md` 子线程纪律条目 + `docs/reference/开发执行规范.md` §0.6 步骤 3 补齐「判域 → 取红线 → 粘进 brief `## 约束（红线）` 段」，并给出 brief 模板片段与子线程自取兜底命令（`constraint-digest.sh --paths <你的目标>`）。
- **派发可观测**：`subagent.dispatch` 事件 payload **append-only** 追加 `constraintDomains`（从派发任务文本解析 `## 约束（红线）` 段得到的域清单）与 `constraintDigest`（present/absent）；`harness-retro` 增补「派发带域比例」口径，供观察窗口回检。
- **文档注册**：`docs/reference/harness/pi-extensions.md` 增补脚本与字段说明；`docs/reference/constraints-index.md` 指针到取数命令（`doc-impact-applies` 标签继续做唯一路径判据）。

### 非目标（本 change 明确不做）

- **不放开门槛**：未激活档仍只注入索引；`matchText` / JIT / `allowedKeywordDocs` 三处 gate 一律不动。
- **不改子线程继承链路**：不碰 `deliverChildInitStable` / 快照 key / `mode.set` 语义。
- **不做插件自动推断兜底**：无档 JIT 兜底作为观察窗口后的候选决策（见 design §观察窗口），本 change 不实现。
- **不统一 4 处域清单**（flow basename / keywordDocs / constraints-index / doc-impact menu）：本 change 只确立"业务域唯一真相源 = flow basename"，其余三处按别名/子集在 design 记录，不在此次收敛。

## Impact

- **新增脚本**：`scripts/harness/constraint-digest.sh` + `constraint-digest.smoke.sh`（断言红线提取与真实 TS 实现逐字节一致、路径反查命中/无命中/多域三种情况）。
- **规范文档**：`AGENTS.md`（子线程纪律条目）、`docs/reference/开发执行规范.md` §0.6 步骤 3。
- **机制文档**：`docs/reference/harness/pi-extensions.md`（脚本 + 字段）、`docs/reference/constraints-index.md`（取数指针）。
- **代码**：`.pi/extensions/harness-telemetry.ts`（`subagent.dispatch` payload 追加两字段，append-only，不改既有字段）。
- **行为可见变化**：red 无（注入行为不变）；主线程多一条可选命令；事实库多两个字段；`harness-retro` 报告多一段比例。
- **旧数据降级**：既有 `subagent.dispatch` 记录无新字段 → retro 统计按"未知"计数，不回溯推断、不记为 absent（避免把历史数据算成漏）。
- **风险**：路径反查为纯子串匹配（非 glob），且 15 篇 standard 文档中 8 篇缺 `doc-impact-applies` 标签 → 路径无命中时脚本输出「未命中」并提示回退 `constraints-index.md` 域表，不静默给空结果。
