# 业务流程（Flow）

> **业务设计层 · 五位一体活文档**：每个 flow 文档同时承载「需求说明 + 链路设计 + 业务约束 + 代码索引 + 变更溯源」，按"大功能"切分，跨前后端。
> 本目录**替代原用户手册目录（已删除）** 承接"系统能做什么、怎么用"的说明（面向使用视角的「需求说明」节）；API 参考唯一权威源为 `docs/reference/api/`。

## 这层装什么（五段式）

每个 `flow/<功能>.md` 固定五个二级标题（`scripts/check-standards.sh` A 段校验齐全）：

1. **需求说明** — 功能给用户解决什么问题（面向使用视角）
2. **链路设计** — mermaid 流程图 + 状态流转
3. **业务约束与不变量** — 状态机/幂等/去重/限额等业务红线。**是 constraint-injection extension 的注入数据源（业务规范 what 段）**——apply 改代码前按命中 domain 自动注入 system prompt（见《开发执行规范》§0.6）。注入为双源，执行规范 how 段来自 `standard/` 文档头标 `doc-impact-applies` 且命中的「## Requirements」节。
4. **代码入口** — 后端 handler/service + 前端 feature 入口（含 `backend-go/internal/<domain>/` 包路径，context 靠它关联 flow↔domain）
5. **变更溯源** — archive change 链接表（见《开发执行规范》§12.2）

**不装**：代码怎么写（去 `standard/`）、架构骨架（去 `architecture/`）、跨功能传导耦合（去 `architecture/coupling-map.md`）。

## 权威中英词表（术语统一基准）

flow 文档与 UI 文案统一使用下列译名；与本表冲突的历史写法（如「板块」）属错别字，修正时以本表为准：

| 中文 | 英文/代码标识 | 说明 |
| ---- | -------------- | ---- |
| 版块 | semantic board（`semantic_labels.label_type='board'`） | 用户可管理的长期语义资产；历史误写「板块」一律修正为「版块」 |
| 话题 | persistent topic（`board_persistent_topics`） | 版块内跨日存活的话题框架 |
| 主题标签 | topic tag（`topic_tags`） | 文章级事件/主题标签 |
| 章节 | section（`daily_report_sections`） | 日报内聚类章节 |
| 叙事线 | thread（`daily_report_threads`） | 日报章节内的叙事线程；「叙事线」是 thread 的唯一合法中文指称 |
| 订阅发现候选 | feed recommendation（`feed_recommendations`） | 偏好向量推荐的订阅源候选 |

**「叙事」的合法用法登记**（其余场景改用「日报/叙事线/章节」语境）：

1. 产品页专有名「叙事工坊」（tags 页）——保留不改。
2. `evolution_narrative`（结构演化叙述）——data-enrichment 域活概念，指版块结构演化的叙述文本，与已下线的 narrative_boards/narrative_summaries 双轨无关。

> 历史背景：旧「叙事摘要双轨」（narrative_boards / narrative_summaries 表及生成链路）已随 retire-narrative-legacy 全量下线，日报（board_daily_reports）是其唯一承接者。

## 全局主链路

```mermaid
flowchart LR
  RSS[RSS 源] --> BACK[backend-go 拉取/解析]
  BACK --> PG[(PostgreSQL 持久化)]
  PG -.可选.-> ENRICH[全文抓取/内容补全]
  PG -.可选.-> SUMMARY[AI 总结]
  PG -.可选.-> DIGEST[Digest 聚合]
  PG -.可选.-> TAG[标签向量化/自动合并]
  ENRICH & SUMMARY & DIGEST & TAG --> API[backend API]
  API --> FETCH[前端 app/api 拉取]
  FETCH --> STORE[apiStore 映射]
  STORE --> DERIVE[派生 store / feature 组件]
  DERIVE --> UI[UI 渲染]
```

## 流程索引

| 文档 | 大功能 | 涉及 domain / feature |
| ------ | -------- | ---------------------- |
| [reading.md](./reading.md) | 主阅读页（启动/切分类/切feed/打开文章） | reader / shell, articles |
| [content-enrichment.md](./content-enrichment.md) | 文章内容增强（Firecrawl 全文/整理稿） | dataenrichment, reader / articles |
| [data-enrichment.md](./data-enrichment.md) | 数据富化编排（爬取/补全/向量化的编排链） | dataenrichment |
| [ai-summary.md](./ai-summary.md) | AI 总结批量生成 | reader / ai |
| [daily-report.md](./daily-report.md) | 日报 / Digest 聚合 | admin(daily_report) / ai |
| [topic-graph.md](./topic-graph.md) | 话题图谱（PersistentTopic 归属/展示） | topicgraph / tags |
| [semantic-board.md](./semantic-board.md) | 语义版块（管理/匹配/升级/治理面板） | tagmanagement / tags |
| [scheduler.md](./scheduler.md) | 定时任务（横切：feed刷新/状态回传/手动trigger） | admin(scheduler) / settings |

## 约束

- 不再维护本地镜像数组同步链，不再使用 `syncToLocalStores()`
- 组件层优先消费已映射好的前端模型
- 与后端交互的细节只应停留在 `app/api` 和 store

## 变更溯源

| 日期 | 变更 | 摘要 | 归档位置 |
|------|------|------|----------|
| 2026-07-20 | docs-harness-consolidation | flow 升级五位一体活文档（需求/链路/业务约束/代码入口/溯源）；业务约束节作为注入数据源（时为 `doc-impact.sh context`，2026-08-22 起由 constraint-injection extension 接管，见 port-constraint-injection）；原 user-guide 定位由 flow「需求说明」节承接 | [archive/2026-07-20-docs-harness-consolidation](../../../openspec/changes/archive/2026-07-20-docs-harness-consolidation) |
| 2026-08-21 | add-change-scope | 新增 `scripts/change-scope.sh` 改动范围→最小验证命令机械判定（路径三档映射，未命中不猜）；quality-gate turn_end 升级自动跑影响包 `go test -short`（DB 集成测试 -short 自动 skip）。「测试只跑影响包」从自觉规则变机械执行 | [archive/2026-08-21-add-change-scope](../../../openspec/changes/archive/2026-08-21-add-change-scope) |
| 2026-08-21 | amend-dev-workflow | 测试纪律改用例先行（Scenario 即黑盒用例+复杂档白盒用例，顺序解绑，bug 先复现底线不变）；调研两级落点（change research.md / `docs/research/`），experience 回归纯踩坑复盘 | [archive/2026-08-21-amend-dev-workflow](../../../openspec/changes/archive/2026-08-21-amend-dev-workflow) |
| 2026-08-23 | port-constraint-injection | 移植 constraint-injection extension（harness 层每 turn 注入：flow「业务约束与不变量」节级注入 + standard JIT 路径命中 + 关键词命中粘性保前缀缓存 + pin_finding 两级落点）；`doc-impact.sh context` 子命令退役，9 个 flow 文档脚注数据源表述改指 extension | [archive/2026-08-23-port-constraint-injection](../../../openspec/changes/archive/2026-08-23-port-constraint-injection) |
| 2026-08-23 | harness-facts-tier-a | harness 层事实库落地（`.pi/harness/events.db` 六类事件记账：constraint.inject / pin.write / pin.read / gate.check / subagent.dispatch / session.start），constraint-injection 注入与 pin 读写经 lib/harness-log 自报，模型零参与；flow 文档作为注入数据源的机制不变，仅新增记账维度 | [archive/2026-08-23-harness-facts-tier-a](../../../openspec/changes/archive/2026-08-23-harness-facts-tier-a) |
| 2026-08-23 | constraint-domain-declaration | 约束注入改业务域显式声明：proposal.md 头部 `constraint-domains` 标记声明即注入（change 全文退出关键词命中源，根治 harness 类 change 撞车误注入）；9 个 flow 文档头部补 `doc-impact-applies` 标签成为 JIT 路径命中单一真相源（json jitDocs 退役） | [archive/2026-08-23-constraint-domain-declaration](../../../openspec/changes/archive/2026-08-23-constraint-domain-declaration) |
| 2026-08-23 | constraint-injection-tier-b | 注入工程升级两件：① 注入块总量预算（budgetBytes 默认 32K）+ 分层降级（keyword→jit→findings digest→域声明→占位，永不真丢，降级确定性保前缀缓存），constraint.inject 附 degraded 记账；② 档位持久化（mode.set 事件）+ 会话边界恢复三路径（resume/reload 两段式、startup 冷启动同 sessionId 恢复——真实链路返工实证 quit 重启走 startup）。事件词汇六类→七类，flow 约束节内容与辖区不变 | [archive/2026-08-23-constraint-injection-tier-b](../../../openspec/changes/archive/2026-08-23-constraint-injection-tier-b) |
| 2026-08-24 | retire-narrative-legacy | narrative 遗留双轨清算：DROP narrative_summaries/narrative_boards（20260824_0001 destructive）、删 models/admin 死方法/死路由/dump-sanitizer 条目、NarrativeGenerateDialog→DailyReportGenerateDialog；本 README 落权威中英词表（版块/话题/主题标签/章节/叙事线/订阅发现候选）+「叙事」合法用法登记 | [`openspec/changes/archive/2026-08-24-retire-narrative-legacy`](../../../openspec/changes/archive/2026-08-24-retire-narrative-legacy) |
| 2026-08-23 | fix-mode-recovery-cross-session | 档位恢复勘误：砍掉 tier-b 第 2 段「全局最新 mode.set 兜底」——多 pi 窗口并行是常态，全局最新几乎必然属其他窗口（实测探索窗口 reload 被灌入他窗口 implementation 档）；resume/reload/startup 三路径统一仅按同 sessionId 取数，/resume 用目标会话自身 id 必中，兜底防御场景不存在 | [archive/2026-08-23-fix-mode-recovery-cross-session](../../../openspec/changes/archive/2026-08-23-fix-mode-recovery-cross-session) |
| 2026-08-26 | harness-observability-fixes | harness 观测面修复三件：① gate.check diag 用 truncateDiagGate 节最小字节下限（不丢 FAIL 行）；② quality-gate 增量路由（触发集=本回合相对快照新增/变化路径，非 git 累积 diff）+ 失败粘性（上回合失败未转绿纯对话也重跑）；③ telemetry 补 subagent.complete 回填（完成派发也有账）。事件词汇七类不变，仅补完成态记账 | [archive/2026-08-26-harness-observability-fixes](../../../openspec/changes/archive/2026-08-26-harness-observability-fixes) |
| 2026-08-26 | test-case-entry-gate | 白盒用例反馈前置两件：① 复杂度声明制——proposal 头 MUST 携 `<!-- complexity: complex\|simple -->`（判定标准：状态机≥3状态/算法/多模块协议任一命中，义务家在开发执行规范 §2，openspec 官方 skill 不植入定制项）；② entry-gate 动工入口门禁——implementation 档 + 缺 test-cases*.md 时 steer 提醒进上下文（声明 complex=强提醒，simple/未声明+4 词兜底=质询），spec-gate ⑤a 归档同步声明优先。补文档义务从归档末端提前到动工瞬间，不再全靠自觉 | [archive/2026-08-26-test-case-entry-gate](../../../openspec/changes/archive/2026-08-26-test-case-entry-gate) |
| 2026-09-02 | harness-quick-wins | 事实库复盘四快赢：① pnpm lint 启用 eslint --cache（cmd.exe 原生增量 2~6s，旧全量 22.3s 黑洞退役）；② quality-gate 后端三命令并行 + lint 先行短路哨兵（编译失败跳过必红同因命令，未执行不记账）；③ gate.check 成功事件改采样记账（会话首条与转绿锚点必记，其后每 5 连续成功记 1 条，失败仍全量）；④ steer 催修分级（曾绿变红=[回归]强催，从未绿=[中间态]轻提示）。AGENTS.md/harness-facts skill 补齐 8 扩展全景档 | [archive/2026-09-02-harness-quick-wins](../../../openspec/changes/archive/2026-09-02-harness-quick-wins) |
