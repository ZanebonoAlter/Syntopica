<!-- complexity: standard -->
<!-- ui-impact: none -->
<!-- constraint-domains: daily-report, discovery, ai-summary -->

## Why

`docs/srs/syntopica-client/SRS.md` 的需求基线（2026-10-19~22 七轮探索固化）建立在「托管池 + 轻客户端」的 C/S 分离形态上。该形态的**唯一经济理由是摊薄 AI 算力**：S 端集中承担抓取与深度处理（打标 / embedding / 摘要），C 端只消费下发的素材包。2026-11-20 变现路径调研结论是机会不大——单用户 / 开源工具定位下，「标准源同 URL 只算一次」与「私有源额度上云」两项价值都不成立，S 端失去存在理由。

同期出现两个反向信号，支持把形态收敛到单机：

1. **本地算力已够用**（迭代 0 实测）：树莓派 RPi5 上 Qwen3-Embedding-4B-Q4_K_M 短语级 0.6~0.7s、冷启 6.5s，关 `GGML_CPU_REPACK` 后内存峰值 2.6GB；embedded PG 真负载验证通过（warm 冷启 122ms、PSS 181MB、三域向量查询不慢于托管端）。
2. **「要注册 / 要联网 / 要信别人」本身就是门槛**：对自用工具，隐私与本机可用性的价值高于「装完立刻有内容」；后者已可由静态种子素材包补齐，不需要一台常年在线的服务端。

⇒ 方向调整为**一体化本地客户端 + 可选一键下载推理包**，托管池与素材包分发整块退役。

## What Changes

### 四条主决策（2026-11-20 用户拍板）

| # | 决策 |
| --- | --- |
| 1 | **S 端整块删除**：素材包增量同步、池子运营约束、档1/档2 切换全部从基线移除，SRS 收敛为单形态 |
| 2 | **embedding 沿用 qwen3-embedding-4b / 2560 维不变**：零重算、零迁移；2560 维升级为**数据契约**（换模型 = 全库重算事件） |
| 3 | **本地 LLM 是可选增强**：默认路径仍是云端 API key（零下载、质量最好）；本地推理包作为"完全离线"第二路径 |
| 4 | **内置静态种子素材包**：装完即可看到日报，不必等自己抓够内容；一次性导入，非网络增量同步 |

### 连带改写（需求语义已变，不是简单删除）

- **术语体系**：档1/档2 → 「AI 供给路径」（云端 API key / 本机 Ollama / 本地推理包）；托管池 / 私有源额度 / 深度解析开关 / 素材包分发给全部退出。
- **§2.4 新鲜度分层**：从「两个数据来源（本地实时 / 池子深度）」改为「三个处理阶段（实时 / 深度 / 种子）」，分层描述处理深度而非数据来路。
- **0-4 job 开关矩阵语义反转**：原「档1 关掉 8 个供给侧 job 省成本」→ 单形态下供给侧 job（发现四件套 / 版块升级建议 / lifeline×3）**默认开启**，关掉的唯一理由是省电/省算力。
- **embedding 健康门 fail-close** 保留，但补充「能力就绪度自述 + 配置引导」：不是降级打标，而是明确告诉用户缺什么。
- **§6.1 embedding 空间版本化** 退出协议层，降级为本地数据契约（换模型 = 显式全量重算流程）。
- **素材包协议** 只保留 schema 语义（改名种子包，携带 manifest：schema 版本 + embedding 空间 + 源清单 + sha256）；cursor 增量协议、服务端订阅接口、token 鉴权、池化运维全部删除。

### 顺带处置

- `add-server-capability-negotiation`（0/32 任务）：其 `client_mode: off/tier1/tier2` 三态语义随本 change 失效，但 `GET /api/capabilities` 端点的动机（前端 5 处硬伤）在单形态下一条未消、反而更关键。建议改造为「能力就绪度自述」，**不在本 change 内实施**，另在 tasks 中登记待办。

## Capabilities

本 change **只改需求基线与设计文档**，不新增/修改 `openspec/specs/` 主 spec，不改动任何产品代码。行为契约的落地由后续实现 change 承接（首个为 `wire-capability-readiness` 系列）。

### New Capabilities

（无）

### Modified Capabilities

（无——需求基线文档不属 openspec 主 spec 管辖范围）

## Impact

- **重写**：`docs/srs/syntopica-client/SRS.md`（§1 引言 / §2 总体描述 / §3 用户故事 / §4 功能需求 / §5 流程 / §6 协议 / §7 NFR / §8 工程事实 / §9 开放问题 / §10 变更溯源）。
- **状态标注**：`docs/srs/syntopica-client/PLAN.md` 头部标注迭代路线待按单形态重排（本 change 不改迭代内容）。
- **标注待重绘**：`docs/srs/syntopica-client/diagrams/*.html`（四张高保真图为旧 C/S 形态存档）。
- **不动**：`front/`、`backend-go/`、`docs/reference/`（现网实现与 reference 文档描述的都是"能跑的单一实例"，与单形态不矛盾）。
- **不触碰**：`openspec/specs/`、其他活跃 change 的制品。

## 不含

- 前端能力就绪度的 UI 落地（另开 change，走 ui-design）
- 种子素材包的实际打样与体积定案（OQ-3，需从现网导一份实测）
- 本地推理包的下载器实现与分发渠道定案（OQ-4）
- PLAN.md 迭代路线重排（本 change 只标状态）
