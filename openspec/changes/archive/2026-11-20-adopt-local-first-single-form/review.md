# Review — adopt-local-first-single-form

审查时间：2026-11-20（CST）；审查方式：派 `change-review` 子线程（minimax-cn/MiniMax-M3.1-Flash:max）只读审查 SRS 改写与 proposal/proposal 一致性，主线程逐条核实后处置。
审查范围：`docs/srs/syntopica-client/SRS.md`（改写后 511 行）↔ `proposal.md` 四条主决策 ↔ `PLAN.md` 状态标注 ↔ 仓库现状（迁移 `20260403_0003` / `20260614_0001`、主 spec `embedding-dimension-contract`、`daily-report` 域红线 7/14/23/37、`ai-summary` 域红线 1/2）。
共 27 条：4 High / 15 Medium / 8 Low。**High 全部修复，Medium 全部修复或采纳，Low 全部修复。**

### R-1 2026-11-20【High】日报「部分就绪也出报告」与 daily-report 域红线 23/37 正面冲突
- **状态**：已修复（US-D4 改为「提示缺料、不生成半成品」；§5.2 状态机改为 `深度批处理 → 等待队列 →〔双队列清空〕→ 本地组装`；状态流文字补「单版完整制 + 手动入口同步禁用」；NFR-8 同步改写）

### R-2 2026-11-20【High】§6.1 / §8.5 / §8.8 的 embedding 维度锚点指向已删除的配置键与已修掉的迁移
- **状态**：已修复（§6.1「标识」改为 `(model_id, dimension, precision)`，落 manifest + 运行时模型 + 每行 `Dimension` + capabilities 自述，明确**不落 `embedding_config`** 并注明该表键已被 `20260614_0001` 删除；「三数字收口」改为「已结构性退役 + 剩余纪律」；§8.5 改为「维度属于运行时数据契约，不是 schema 契约」；§8.8 改为「已闭合」并指向 `fix-disaster-recovery-channel` 2026-11-20 归档）

### R-3 2026-11-20【High】首启后没有任何机制把 embedding 维度钉到 2560 → 种子包必被拒收
- **状态**：已修复（FR-A2 验收补「首启结束后运行时空间自述 = `(qwen3-embedding-4b, 2560)`；维度未知按未知处理，不得默认 1024」；FR-G5 补精度维度）

### R-4 2026-11-20【High】备份条款沿用「EXCLUDED 清单不变」＝沿用已判定为错误的那版语义
- **状态**：已修复（§2.3 改为「以 `fix-disaster-recovery-channel` 归档后清单为准：`schema_migrations` 在备份内、扩展不在档内须预装、恢复走 restore 脚本」；§2.3 与 FR-G4 的备份时刻统一为「退出时 + 每日一次兜底 / 服务器 cron `0 4 * * *`」；FR-G4 补恢复演练验收）

### R-5 2026-11-20【Medium】manifest 泳道阈值标签错位，且把阈值固化进包内（违红线 7）
- **状态**：已修复（manifest 改 `space_constants_snapshot: {lane_l1_upper, lane_l2_upper}`，注明「只读快照，导入方生效阈值一律取本地 `ai_settings`」；FR-F2 补「阈值可经 `ai_settings` 在线覆盖」）

### R-6 2026-11-20【Medium】FR-F2 泳道判定漏观察期门禁（违红线 14）
- **状态**：已修复（FR-F2 改为「L1 = 距离 < 阈值**且最近 topic 为 active**；candidate 近距离降 L2；`is_vacuum` 降 L2」）

### R-7 2026-11-20【Medium】FR-F2/F4/F6 不点名 capability 与 operation
- **状态**：已修复（FR-F4 点名 `topic_tagging` / `summary` / `embedding` 与 `ai_routes.max_concurrency`；FR-F2 点名 `digest_polish`；FR-F6 补「只读进程内快照，不得为自述触发额外探测」）

### R-8 2026-11-20【Medium】「唯一出网」与推理包/种子包下载冲突；决策④种子包无落点
- **状态**：已修复（NFR-3 出网口径含「用户主动发起的软件/模型/种子包下载」；新增 FR-D6「种子包来源两档：安装包内置精简包 + 可导入外部包」并把体积计入 FR-A1 预算）

### R-9 2026-11-20【Medium】embedding 空间身份未含量化精度 → 云端 FP 与本地 Q4 被当同一空间
- **状态**：已修复（标识改三元组含 `precision`；FR-G5 明确「换模型**或换量化精度** = 全量重算」）

### R-10 2026-11-20【Medium】种子包载体形态自相矛盾（pg_dump + 外置 manifest 两个文件）
- **状态**：已修复（§6.2 补包格式定义：单文件 zip/tar 内含 `data.dump` + `manifest.json`，`data_sha256` 只对 `data.dump` 计算；FR-A6 与 FR-D1 统一引用）

### R-11 2026-11-20【Medium】「种子包内可用 float16 减半」与 pg_dump / `vector` 列通道不兼容
- **状态**：已修复（§6.1「现行空间」改为「float32 与 DB 一致；种子包按库内实际类型导出，不另做 fp16 装包转换；体积靠压缩与源裁剪」）

### R-12 2026-11-20【Medium】「算完即退」在现网机制下不成立，§8.1「零新代码」结论过强
- **状态**：已修复（§8.1 补「进程生命周期现网不托管，自退需新增能力，明确不属零新代码」；FR-F4 验收拆为「接入零新代码 ✔ / 自退策略可验收」）

### R-13 2026-11-20【Medium】§5.2「失败回退 → 回退昨日日报」语义含混
- **状态**：已修复（状态流明确「展示昨日**已生成**的日报，不把昨日数据写成今日报告」）

### R-14 2026-11-20【Medium】FR-F6 无依赖标记；`add-server-capability-negotiation` 制品与新基线矛盾
- **状态**：已修复（SRS §9 增「依赖标记」段：FR-F6 依赖该 change 改造版为能力就绪度自述、tier 三态失效、实施前须重写其制品；tasks 2.3 已写具体待办）

### R-15 2026-11-20【Medium】源目录两条更新路径无优先级
- **状态**：已修复（FR-C1 明确「内置推荐源目录」与「`rsshub_catalog_sync` 发现页路由目录」是两套不同目录、互不覆盖）

### R-16 2026-11-20【Medium】「Docker PG 退役为可选」措辞越界；服务器装法无鉴权监听局域网无风险提示
- **状态**：已修复（§2.3 改为「不强制退役：embedded 为桌面默认与服务器推荐，现网存量 Docker 部署可继续用」；FR-B2 补「默认监听仅环回、界面须标明监听地址与局域网可见状态」）

### R-17 2026-11-20【Medium】需求编号被复用但无迁移映射表；proposal 的 OQ 引用过期
- **状态**：已修复（SRS 新增 §10.1「编号迁移映射表」；proposal 的 OQ 引用改为 OQ-3 / OQ-4）

### R-18 2026-11-20【Medium】PLAN.md 状态行把迭代 5 整块判死，误杀仍被依赖的三项
- **状态**：已修复（状态行改按议题粒度：失效 = 4-1~4-5 / 4-6 / 4-7 / 6-3；保留 = 6-2 / 6-4 / 5-2；路线图 M2 打「已退役」角标、M5 改「打包与分发收尾」）

### R-19 2026-11-20【Medium】唯一交付物因 `.gitignore` 不入库 → 无 diff 基线
- **状态**：已采纳（选方案 b：保持 `docs/srs/` 不入库的既有口径（与 `docs/research`/`docs/experience` 一致，改 ignore 需用户决策），在 SRS 头部写明「不入库、需从作者本机获取」，并把决策对照与编号映射落在本 change 已入库的 proposal + 本 review.md；tasks 4.4 的可复核性由 §10.1 映射表 + 本文件承担）

### R-20 2026-11-20【Low】FR-A6 与 FR-D1..D5 职责重叠
- **状态**：已修复（FR-A6 收敛为「导入入口」，校验/幂等/覆盖只在 FR-D 定义）

### R-21 2026-11-20【Low】平台矩阵三处不一致
- **状态**：已修复（FR-A1 改为 darwin/linux × amd64/arm64 + windows/amd64，windows/arm64 复用 x64 模拟，与 FR-A4 对齐）

### R-22 2026-11-20【Low】FR-F5「lifeline 默认注册」与源码不符；未声明与 `analysis_paused` 的关系
- **状态**：已修复（FR-F5 注明 `lifeline_weekly` 当前停用、monthly/yearly 在跑；补 `analysis_paused` 总闸对分析类 job 的优雅停语义）

### R-23 2026-11-20【Low】NFR-1 数字缺出处
- **状态**：已修复（chat 体积标「估算值，落地前实测校准」；embedding 数字指回 §8.6 实测）

### R-24 2026-11-20【Low】§2.2 图把本地推理包画成主路径
- **状态**：已修复（CLOUD 标「默认路径」、AIP 标「可选增强」）

### R-25 2026-11-20【Low】US-D3「跨文章线程」需与已退役事件线划清
- **状态**：已修复（US-D3 注明 `daily_report_threads`，不含已退役的事件线）

### R-26 2026-11-20【Low】FR-C2 验收标准恒真
- **状态**：已修复（改为「订阅 1000 个源不被任何配额逻辑拦截，界面不出现额度提示」）

### R-27 2026-11-20【Low】manifest 占位值；§9 缺两项必答开放问题
- **状态**：已修复（`article_count` 改示例值 1200；§9 增 OQ-8 种子包分发载体 / OQ-9 服务器装法监听与口令默认值）

## 结论

4 条 High 全部修复且经仓库现状核实（`embedding_config` 键已删、迁移 0003 已改、`fix-disaster-recovery-channel` 已归档、红线 23/37 为本仓库现行约束）。15 条 Medium 全部处置（14 修 + R-19 选「保持不入库 + 证据落到 change 制品」）。8 条 Low 全部修复。SRS 达到「可据此开实现 change」的质量线。

剩余风险（如实记录）：① 种子包体积/范围（OQ-3）与内置包是否超 FR-A1 的 150MB 预算，需从现网打样后定；② 本地推理包的进程自退策略（§8.1 补注）属实现期新增工作项；③ `add-server-capability-negotiation` 改造未在本 change 内做，实施 FR-F6 前须先重写该 change 制品。

## 归档后溯源判定（tasks 4.2）

- **无需补 flow 变更溯源链接**：本 change 只改 `docs/srs/syntopica-client/`（需求基线，`docs/srs/` 按仓库既有口径不入库），未改 `docs/reference/flow/` 任何业务域文档，也没有代码/行为改动。§12 的 flow 溯源针对代码与行为变更，本 change 不适用。
