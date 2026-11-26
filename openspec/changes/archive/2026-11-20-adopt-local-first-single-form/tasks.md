# Tasks — adopt-local-first-single-form

> 纯文档 change：豁免代码测试，但必须附 grep 一致性校验（`openspec/config.yaml` context）。

## 1. 需求基线改写（SRS）

- [x] 1.1 头部状态与来源：改为单形态基线，标注 C/S 形态存档位置与本次方向调整轮次
- [x] 1.2 §1 引言：目的 / 背景与痛点 / 产品愿景 / 术语表 / 参考文档全部按单形态重写（删托管池、档1、档2、标准源、私有源、深度解析开关、素材包同步；新增本地推理包、种子素材包、能力就绪度）
- [x] 1.3 §2.1 用户角色：删除管理员角色，改为轻量用户 / 离线用户 / 多设备用户 / 自维护者
- [x] 1.4 §2.2 总体架构：重画 mermaid（零中心服务、AI 供给三选、可选 llama.cpp sidecar），删除托管池与素材包分发 API
- [x] 1.5 §2.3 运行形态与演进：收敛为「桌面装法 + 可选服务器装法」，明确不存在对外分发池形态；pg_dump 双向通道保留
- [x] 1.6 §2.4 内容新鲜度分层：从「两个数据来源」改为「三个处理阶段（实时 / 深度 / 种子）」
- [x] 1.7 §3 用户故事：主题 A~F 按单形态重写（A5 种子包、C 去额度、F4 离线可用）
- [x] 1.8 §4 功能需求：FR-A 增补本地推理包下载与种子包导入；FR-B 降级为本机/局域网访问；FR-C 去额度；FR-D 重写为种子素材包；FR-E 推荐走用户 chat 路由；FR-F 重写深度批处理 + 供给侧默认开启 + 能力就绪度自述；FR-G 增补 AI 供给切换语义；**FR-H 删除**
- [x] 1.9 §5 关键交互流程：重画 5.1 时序（去托管池）、5.2 状态机（导入/校验/本地批处理）、5.3 条目生命周期（裸条目经本地批处理升级）
- [x] 1.10 §6 协议与数据约定：6.1 embedding 空间降级为数据契约、6.2 种子包 manifest、6.3 本机访问控制、6.4 保留 pg_dump
- [x] 1.11 §7 非功能需求：NFR-1 硬件门槛按实测数字重写（embedding 2.38GB 权重 / 峰值 2.6~4.8GB；chat 8B ≈ 4.7GB）、NFR-3 隐私强化、NFR-6 改为用户自担算力、NFR-7 离线可用
- [x] 1.12 §8 约束与工程事实：保留 5 条，新增 RPi5 embedding 实测、embedded PG 真负载验证、（2560 维 = 数据契约）
- [x] 1.13 §9 开放问题：删 OQ-3（cursor）、OQ-4（额度 N）；新增种子包范围/体积、推理包分发渠道、默认推理路径优先级、REPACK 取值
- [x] 1.14 §10 变更溯源：新增第 8 轮（2026-11-20 方向调整，四条主决策 + 作废清单）
- [x] 1.15 §附图表索引：标注高保真 HTML 为旧形态存档、单形态图待重绘
- [x] 1.16 按 `review.md` 27 条意见修正 SRS（4 High 全修：日报单版完整制 / embedding 维度锚点 / 首启落定 2560 / 备份清单语义；15 Medium + 8 Low 全部处置；新增 §10.1 编号迁移映射表、FR-D6 种子包来源两档、§6.2 包格式定义）

## 2. 设计文档同步（最小动作）

- [x] 2.1 `docs/srs/syntopica-client/PLAN.md`：头部加状态行「迭代路线按单形态重排中，见 `adopt-local-first-single-form`」，迭代正文不动
- [x] 2.2 `docs/srs/syntopica-client/diagrams/`：在 SRS 图表索引处标注四张 HTML 为 C/S 形态存档（不重绘图）
- [x] 2.3 登记待办：`add-server-capability-negotiation` 需重写三件制品——① `proposal.md`（删 `client_mode` 三态语义，改「能力就绪度自述 + 供给侧默认开启」）；② `specs/server-capability-negotiation/spec.md`（档位场景改就绪度场景）；③ `tasks.md`（32 项里 job 分组映射与注册期过滤可复用，档位相关项作废）。本 change 不动其制品，实施 FR-F6 前先做

## 3. 测试

<!-- 纯文档 change：豁免代码测试（openspec/config.yaml context） -->

- [x] 3.1 无受影响测试包：本 change 不改 `front/` 或 `backend-go/`，无代码测试可跑
- [x] 3.2 一致性校验见 §5.2（旧术语残留 0 命中）与 §5.4（一级章节结构未丢）

## 4. 文档

<!-- doc-impact: none -->
<!-- doc-impact-excuse: flow=本 change 只改 docs/srs/ 需求基线（不入库），未改 docs/reference/flow/；启发式命中来自编辑路径与关键词，非真实文档遗漏 -->

> **无 flow 影响**：同 4.2 的判定——本 change 只收敛不入库的需求基线，无代码/行为/flow 变更，故不补 flow 变更溯源行（§12.2 豁免）。

- [x] 4.1 `docs/reference/` 不需同步：本次只改需求基线，现网实现与 reference 描述的都是「能跑的单一实例」，与单形态不矛盾（校验方式见 §4.2）
- [x] 4.2 archive 后按 §12 补 flow 变更溯源链接 —— **判定无需补**（本 change 只改不入库的 `docs/srs/` 需求基线，无代码/行为/flow 变更），理由已记入 `review.md`「归档后溯源判定」节

## 5. 验证

- [x] 5.1 `openspec validate adopt-local-first-single-form --strict` → 退出码 0
- [x] 5.2 `grep -niE '托管池|档1|档2|私有源额度|素材包同步|同步 cursor|token 鉴权|上云解析' docs/srs/syntopica-client/SRS.md | grep -vE '退役|存档|曾评估|已删除|已随形态|第 [0-9] 轮|重写|原 |修正|编号迁移映射|FR-D 素材包同步|FR-F4 / F5|档1 → 档2'` → **0 命中**（旧术语只允许作为历史叙述出现在 §10/§10.1 与显式退役说明里，不得作为现行条款使用）
- [x] 5.3 `grep -niE '托管池|档1|档2|素材包' docs/srs/syntopica-client/PLAN.md` → 仅在「待重排」状态行与迭代正文的历史表述中出现，且 SRS 不再引用这些术语为新语义
- [x] 5.4 `grep -c '^## ' docs/srs/syntopica-client/SRS.md` → 与改写前同为 10 个一级章节 + 附录（结构未丢）
- [x] 5.5 人工通读：§2.2 架构图与 §2.3 装法、§2.4 分层、§4 FR 编号、§5 流程、§6 协议五处互不矛盾（尤其 FR-D 种子包 vs §6.2 manifest vs §5.1 时序一致）
- [x] 5.6 `bash scripts/harness/doc-impact.sh verify openspec/changes/adopt-local-first-single-form` → 退出码 0
- [x] 5.7 `bash scripts/harness/check-standards.sh --change adopt-local-first-single-form` → 退出码 0
- [x] 5.8 Review：派 `change-review` 子线程只读审查 SRS 改写与 proposal 一致性，意见按 High/Medium/Low 处置后在 `review.md` 建账（27 条已逐条处置）
