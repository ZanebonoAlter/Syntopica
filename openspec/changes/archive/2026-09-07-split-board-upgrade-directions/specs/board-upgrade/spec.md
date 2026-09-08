## MODIFIED Requirements

### Requirement: LLM 判断升级/跳过
系统 SHALL 按用户选定的「方向 × 来源」将候选送入 LLM 裁决，每轮 LLM 只面对单一决策空间，四个组合的决策空间如下：

| 方向 | 来源 | LLM 决策空间 | 建议内容 |
| --- | --- | --- | --- |
| 创建版块 | 单标签 | create_new \| skip | 簇 → 新版块名/描述/成员 aux |
| 创建版块 | 组合标签 | compose \| skip | 共现对 → 新组合名/描述/组件 |
| 版块扩充 | 单标签 | merge_into_existing \| skip | 候选 aux → 挂载到锁定版块 |
| 版块扩充 | 组合标签 | compose \| skip（建议携带目标版块） | 共现对 → 新组合 + 挂载到锁定版块 |

创建方向的 LLM prompt SHALL 包含全量已有版块名称列表，语义上与已有版块重复的簇/组合 SHALL 产出 skip（防重复创建）。skip 决策 SHALL 不落库。LLM SHALL NOT 在单轮内被要求在创建与合并之间选择，也 SHALL NOT 被要求自行指定合并目标版块（扩充方向的目标由用户在生成前锁定）。

#### Scenario: LLM 判断创建新 board
- **WHEN** 用户选择「创建版块 + 单标签」，簇 [新能源, 光伏, 储能] LLM 判断值得升级且与已有版块不重复
- **THEN** 系统 SHALL 返回 create_new 建议（版块名、描述、候选辅助标签），等待用户确认

#### Scenario: LLM 不再产出 merge_into_existing
- **WHEN** 创建方向生成中 LLM 对某簇返回 merge_into_existing（越权决策），或簇 [AI, transformer, 深度学习] 主题与已有版块「人工智能」重复
- **THEN** 系统 SHALL 过滤丢弃越权 merge 决策（创建模式决策空间仅 create_new/skip）；重复簇产出 skip，不落库——用户可通过「版块扩充 + 单标签 + 选择"人工智能"」路径收纳这些标签

#### Scenario: 扩充×单标签产出挂载建议
- **WHEN** 用户选择「版块扩充 + 单标签 + 版块 B」，候选 aux「美债拍卖」被 LLM 判定属于 B
- **THEN** 系统 SHALL 返回 merge_into_existing 建议（target_board_id=B），不要求也不接受 LLM 指定其他目标

#### Scenario: LLM 判断跳过
- **WHEN** 簇内辅助标签过于分散，LLM 判断不足以形成板块
- **THEN** 系统 SHALL 返回 skip，不落库

### Requirement: suggestUpgrades API 支持 mode 参数

`POST /api/semantic-boards/upgrade/suggest` SHALL 接受组合参数取代原 `mode` 参数：

- `direction=create|expand`（必填）
- `source=aux|composite`（必填）
- `target_board_id`（direction=expand 时必填，指向有效活跃版块；direction=create 时 SHALL 拒绝携带）
- `days`（可选时间窗口，语义不变）

方向与来源组合 SHALL 限定为四格之一；`direction=expand` 缺 `target_board_id`、`direction=create` 携带 `target_board_id`、或 `target_board_id` 指向不存在/非活跃版块时，系统 SHALL 返回参数错误。

#### Scenario: API default mode backward compatible

- **WHEN** 前端调用 `POST /api/semantic-boards/upgrade/suggest` 不带 direction/source，或携带旧 `?mode=discover_new`
- **THEN** 系统 SHALL 返回 400 参数错误（旧 mode 参数与无参调用均不再接受；前端同仓同步切换，无兼容承诺）

#### Scenario: API expand mode

- **WHEN** 前端调用 `POST /api/semantic-boards/upgrade/suggest?direction=expand&source=aux&target_board_id=42`
- **THEN** 系统 SHALL 以扩充×单标签模式运行，返回的 merge_into_existing 建议 target_board_id 恒为 42

#### Scenario: 创建方向调用
- **WHEN** 前端调用 `POST /api/semantic-boards/upgrade/suggest?direction=create&source=aux&days=7`
- **THEN** 系统 SHALL 以创建×单标签模式运行，返回的建议 decision ∈ {create_new}（skip 不返回）

#### Scenario: 扩充方向调用
- **WHEN** 前端调用 `POST /api/semantic-boards/upgrade/suggest?direction=expand&source=composite&target_board_id=42`
- **THEN** 系统 SHALL 以扩充×组合标签模式运行，返回的 compose 建议 SHALL 携带 target_board_id=42

#### Scenario: 扩充缺目标版块被拒
- **WHEN** 前端调用 `?direction=expand&source=aux`（缺 target_board_id）
- **THEN** 系统 SHALL 返回 4xx 参数错误，不启动生成

#### Scenario: 旧 mode 参数被移除
- **WHEN** 调用携带 `?mode=discover_new` 或 `?mode=expand_existing`
- **THEN** 系统 SHALL 返回参数错误（旧参数不再接受，前端同仓同步切换）

### Requirement: co-tag 高频共现对产出组合标签建议
系统 SHALL 在升级建议生成流程中，基于 co-tag 共现统计产出 compose 决策建议。组合标签建议按方向区分：

- **创建方向**（direction=create & source=composite）：现状行为——同一文章内共现频次 ≥ composite_cotag_min_cooccurrence（默认 10，ai_settings 可配）且各组件 aux label ref_count ≥ 升级阈值的标签对/三元组，经 LLM 裁决是否值得组合，产出 compose 建议（不带目标版块）。
- **扩充方向**（direction=expand & source=composite）：候选共现对 SHALL 先按与目标版块的相关性过滤（召回规则见 board-upgrade-expand），LLM 裁决「值得组合且属于目标版块」，产出的 compose 建议 SHALL 携带 target_board_id。

两方向的 LLM 判定均为单一决策空间（compose|skip）；suggestion_hash 幂等与 dismissed 冷却期复用现有机制（扩充方向 hash 计算包含 target_board_id）。LLM 判定不值得组合时产出 skip，不落库。

**已存在组合防重复（2026-09-07 报障补丁）**：候选组件集与既有 active composite 标签完全一致时——创建方向 SHALL 整体排除该候选（组合已存在，无需再建议创建）；扩充方向仅当该组合已挂载目标版块时排除（挂载已完成），未挂载时保留候选（语义退化为「挂载既有组合进目标版块」，确认时按组件去重复用）。部分重叠（非完全一致）不排除。另：存量 pending 的 discover_new 管线建议一次性置 dismissed（迁移 20260907_0001，旧 hash 格式永不被新幂等命中，永久混入列表干扰判断）。

#### Scenario: 已存在组合不再被建议创建
- **WHEN** 组合「美联储加息」（组件 [美联储, 加息]）已存在且 active，创建方向生成时该组件对共现频次达标
- **THEN** 该候选被排除，不送 LLM 裁决，不产出建议

#### Scenario: 已挂载组合不再被扩充建议
- **WHEN** 扩充版块 B 时，组合「美联储加息」已存在且已挂载 B
- **THEN** 该候选被排除；若该组合存在但未挂载 B，则保留候选（确认时复用既有组合挂载）

#### Scenario: 高频共现对产出 compose 建议
- **WHEN** 创建方向生成时，「美国国债」与「收益率」在 co-tag 窗口（30 天）内共现 15 次 ≥ 10，且两者 ref_count 均达标，LLM 裁决值得组合
- **THEN** 产出 decision="compose" 的建议（组合名「美债收益率」、组件 [美国国债, 收益率]、无 target），经 hash 幂等检查后落库为 pending

#### Scenario: 扩充方向的组合建议携带目标
- **WHEN** 用户扩充版块 B（美债）时，共现对「美国国债 × 收益率」通过版块相关性召回，LLM 裁决值得组合且属于 B
- **THEN** 产出 decision="compose" 的建议（组合名「美债收益率」、组件 [美国国债, 收益率]、target_board_id=B）

#### Scenario: LLM 裁决无意义组合被过滤
- **WHEN** 候选共现对「日本」「市场」共现 12 次，LLM 裁决组合「日本市场」无明确指向语义
- **THEN** 产出 skip，不落库

#### Scenario: 同 hash 建议幂等
- **WHEN** 下一轮生成产出与既有 pending 建议 hash 相同的 compose 建议
- **THEN** 该建议 SHALL 被跳过（skipped），不重复入库

#### Scenario: dismissed 冷却期内拦截
- **WHEN** 用户 dismiss 某 compose 建议，冷却期内同 hash 建议再次生成
- **THEN** 该建议 SHALL 被冷却期拦截（cooldown_blocked），期满后才可重生

### Requirement: compose 建议确认执行
用户确认 compose 建议后，系统 SHALL 在同一事务内执行：

1. 创建组合标签（label_type="composite"，source="upgrade_suggest"）及其 composite_components 组件引用、生成组合 embedding、写 ref_count 初始值（命中 L1/L2 去重时复用既有组合）；
2. 若建议携带 target_board_id（扩充方向），在同一事务内将组合标签写入目标版块的 board_composition；
3. 将建议标记为 confirmed。

组合创建失败或挂载写入失败（如 embedder 失败、去重冲突异常）则整体回滚，建议保持 pending。

#### Scenario: 确认创建组合标签
- **WHEN** 用户确认创建方向的 compose 建议「美债收益率」
- **THEN** 事务内创建组合标签 + 组件引用 + LLM 组合 embedding，建议状态 → confirmed，不挂载任何版块

#### Scenario: 确认扩充方向的组合建议
- **WHEN** 用户确认携带 target_board_id=B 的 compose 建议「美债收益率」
- **THEN** 事务内创建组合标签（含去重复用路径）并挂载进 B 的 board_composition，建议状态 → confirmed

#### Scenario: 创建失败回滚
- **WHEN** 确认执行时 embedder 调用失败
- **THEN** 整体回滚，组合标签不落库，建议保持 pending，错误返回

#### Scenario: 挂载失败回滚
- **WHEN** 扩充方向确认执行时 board_composition 写入失败
- **THEN** 整体回滚（组合标签不落库），建议保持 pending，错误返回

#### Scenario: 确认时命中既有组合去重
- **WHEN** 确认执行时 L1/L2 去重命中既有组合标签
- **THEN** 不新建，复用既有组合（ref_count++），挂载（如携带 target）照常执行，建议仍标记 confirmed，执行结果提示复用

### Requirement: 前端渲染 compose 建议
升级建议面板 SHALL 渲染 decision="compose" 的建议：展示建议组合名、组件标签序列、共现证据（共现频次、窗口、代表事件标题）；扩充方向的建议 SHALL 醒目展示目标版块（「将挂载到：美债」）；提供确认执行（创建组合标签，扩充方向含挂载）与 dismiss 操作；决策过滤 tab 含「组合」选项。

#### Scenario: compose 建议卡片
- **WHEN** 建议列表包含 compose 建议「美债收益率」（组件 [美国国债, 收益率]，共现 15 次）
- **THEN** 面板 SHALL 展示组合名、组件序列、共现频次证据，提供确认/dismiss 操作

#### Scenario: 扩充组合建议展示目标版块
- **WHEN** 建议列表包含携带 target_board_id=B 的 compose 建议
- **THEN** 面板 SHALL 在卡片上展示「将挂载到：<版块 B 名称>」

#### Scenario: 决策过滤包含组合
- **WHEN** 用户切换决策过滤 tab 到「组合」
- **THEN** 列表 SHALL 仅展示 decision="compose" 的建议

## ADDED Requirements

### Requirement: 升级建议生成路径单一化
所有升级建议 SHALL 由用户选定方向后经 LLM 裁决产生。系统 SHALL NOT 生成 watch 观察池建议（单例簇 SHALL 不产建议，等待未来成簇后参与）、SHALL NOT 自动合成合并建议（双签名高置信合成路径废除）。存量 pending 的 watch 建议 SHALL 被一次性清理（删除行）；存量 pending 的高置信合并建议 SHALL 保留且可正常确认/dismiss（仅不再新生成）。

#### Scenario: 单例簇不产建议
- **WHEN** 聚类产出单例簇（size=1）
- **THEN** 该簇 SHALL 不产出任何建议，不进任何观察池

#### Scenario: 存量 watch 建议被清理
- **WHEN** 本变更部署后执行数据清理
- **THEN** decision="watch" 的 pending 建议 SHALL 从升级建议表删除

#### Scenario: 存量高置信合并建议保留可确认
- **WHEN** 用户查看持久化建议列表，存在变更前生成的高置信 merge 建议
- **THEN** 该建议 SHALL 正常展示且可确认/dismiss，生命周期不受影响

### Requirement: 定时生成仅创建方向
定时任务 job_board_upgrade_suggest SHALL 仅以创建方向运行（创建×单标签 与 创建×组合标签），自动涌现新板块与新组合标签建议；版块扩充方向 SHALL 仅由用户在面板选定目标版块后手动触发。定时任务生成失败时仅记日志且不阻塞兄弟 job（沿用既有约定）。

#### Scenario: 定时任务跑创建方向
- **WHEN** job_board_upgrade_suggest 按计划触发
- **THEN** 系统 SHALL 生成创建方向建议（create_new 与 compose，均不带 target），不生成任何扩充方向建议

#### Scenario: 扩充不自动执行
- **WHEN** 定时任务运行周期内无用户操作
- **THEN** 系统 SHALL NOT 对任何版块自动生成扩充建议

### Requirement: 生成入口模式选择
升级建议面板 SHALL 提供两步模式选择作为生成入口：第一步选择方向（创建版块 / 版块扩充），第二步选择来源（单标签 / 组合标签）；选择「版块扩充」时 SHALL 展示版块单选下拉（仅活跃版块，含版块名与描述）且 MUST 选定一个版块后才能生成。旧内存探索区（候选列表、簇列表、内存建议及「获取 LLM 建议」链路）SHALL 移除；持久化建议列表为唯一展示区。

#### Scenario: 选择创建版块方向
- **WHEN** 用户依次选择「创建版块 → 单标签」并点击生成
- **THEN** 前端 SHALL 调用 suggest API（direction=create, source=aux）并刷新持久化建议列表

#### Scenario: 扩充方向必须选定版块
- **WHEN** 用户选择「版块扩充」但未从下拉选定版块
- **THEN** 生成按钮 SHALL 处于禁用状态；选定版块（如「美债」）后生成按钮可用，调用 direction=expand&source=…&target_board_id=美债ID

#### Scenario: 旧内存探索区移除
- **WHEN** 用户打开升级建议面板
- **THEN** 面板 SHALL 不再展示候选列表、簇列表与内存建议区，仅展示模式选择入口 + 持久化建议列表

## REMOVED Requirements

### Requirement: getUpgradeCandidates API 支持 mode 参数
**Reason**: 旧内存探索 UI 退役，候选收集成为生成流程的内部步骤，无需独立暴露候选查询 API；mode 参数语义已由 direction/source/target_board_id 取代。
**Migration**: 前端不再调用 `POST /api/semantic-boards/upgrade/candidates`；该 API 端点移除。如需查看候选规模，由生成接口响应附带的统计信息提供。

### Requirement: 前端展示 Board Affinity 参考信息
**Reason**: 模式拆分后创建建议不再承载「合并参考」职责——相似版块判断移交扩充方向的候选召回与 LLM 二分类；在创建卡片上展示相似板块会诱导跨模式操作。
**Migration**: 创建建议卡片不再展示 board_affinities 字段；防重复创建由创建方向 LLM prompt 携带全量版块列表保障（见「LLM 判断升级/跳过」）。

### Requirement: 前端人工合并下拉操作
**Reason**: 扩充方向的目标版块在生成前由用户锁定，建议行内不再需要「合并到...」改目标下拉；目标不合适时应 dismiss 后换版块重新生成。
**Migration**: 建议卡片移除「合并到...」下拉；扩充建议的 target 在卡片上展示为锁定信息。ConfirmSuggestion API 的 merge 路径不变（target 来自建议本身）。
