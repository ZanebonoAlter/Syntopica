# Hierarchy Template Config

## Purpose

定义标签层级模板系统：固定模板库 + 可配置层级参数，深度反推层级判定，配置变更安全机制。

## ADDED Requirements

### Requirement: 系统提供固定层级模板库
系统 SHALL 提供 5 个固定层级模板：event（3 层）、person（2 层）、keyword:technology（3 层）、keyword:company_business（3 层）、keyword:concept（3 层）。模板数量不可变更，但每个模板的层级定义参数可配置。

#### Scenario: 加载默认模板
- **WHEN** 系统启动且数据库无已保存配置
- **THEN** 系统从代码中加载 5 个默认模板作为运行时配置

#### Scenario: 从数据库加载配置覆盖默认
- **WHEN** 数据库中存在已保存的 `hierarchy_config` 记录
- **THEN** 系统使用数据库配置覆盖默认模板的层级参数

### Requirement: 层级定义可配置
管理员 SHALL 能够通过 API 调整每个模板的层级参数：层级名称（`name`）、层级描述（`description`）、是否叶子节点（`is_leaf`）、最大子标签数（`max_children`）、禁止的名称模式（`forbidden_patterns`）。

#### Scenario: 修改 event 模板的 L1 名称
- **WHEN** 管理员 PUT /api/hierarchy/config 将 event 的 L1 名称从"事件类型"改为"事件大类"
- **THEN** 系统保存新配置，生成新版本号，后续新标签的 L1 使用新名称作为 LLM prompt 中的层级标识

#### Scenario: 拒绝新增模板
- **WHEN** 管理员尝试在配置中添加第 6 个模板
- **THEN** 系统返回 400 错误 "不能新增模板"

### Requirement: 深度反推层级
系统 SHALL 通过标签在抽象树中的路径深度反推其抽象层级，而不依赖 `topic_tags` 表的新列。映射规则：depth ≤ template.MaxLevel → level = depth；depth > template.MaxLevel → level = template.MaxLevel。

#### Scenario: event 3层模板下 depth=3 的标签
- **WHEN** 标签在 event 抽象树中的路径深度为 3 且 event 模板有 3 层
- **THEN** 标签的层级判定为 L3（叶子）

#### Scenario: depth 超过最大层数时截断
- **WHEN** event 模板改为 2 层后，一个原有 depth=3 的标签
- **THEN** 标签的层级判定为 L2，且被标记为待处理（需要重新挂载其子标签）

### Requirement: 新标签默认为叶子节点并向上聚合
所有新创建的标签 SHALL 默认判定为叶子层级（模板最底层），然后逐层向上查找或创建父标签。event 和 keyword 从 L3 开始，person 从 L2 开始。

#### Scenario: 新 event 标签挂载到已有 L2
- **WHEN** 创建新 event 标签 "OpenAI春季发布会" 且 L2 候选池中有 embedding 相似度 0.92 的 "OpenAI"
- **THEN** 标签直接挂载到 "OpenAI" 下，不调 LLM

#### Scenario: 新 event 标签需要创建 L1 和 L2
- **WHEN** 创建新 event 标签 "星舰第八次试飞" 且 L2 候选池无匹配（最高 0.62）
- **THEN** 系统创建 L2 "SpaceX" → 递归向上找 L1，L1 无匹配时 LLM 生成新类型 "航天发射"

### Requirement: L2 父标签匹配使用三级阈值
L2 父标签查找 SHALL 使用 embedding 相似度三级阈值：≥0.85 直接挂载不调 LLM；0.60-0.85 调 LLM 从候选中选择或决定创建新标签；<0.60 直接创建新 L2 标签。

#### Scenario: 高相似度直接挂载
- **WHEN** L2 候选最高相似度 0.88
- **THEN** 系统跳过 LLM 调用，直接将标签挂载到该候选

#### Scenario: 中等相似度 LLM 判断
- **WHEN** L2 候选最高相似度 0.72 且次高 0.68
- **THEN** 系统调用 LLM 从候选列表中选择最合适的父标签或决定创建新标签

### Requirement: L1 事件类型动态 LLM 生成
event 模板的 L1 事件类型 SHALL 由 LLM 动态生成，每次判断时以已有事件类型作为 few-shot 参考。新创建的事件类型需经过 embedding 去重检查。

#### Scenario: LLM 生成新事件类型
- **WHEN** L2 "SpaceX" 被创建且已有 L1 中无匹配类型
- **THEN** LLM 返回新建类型 "航天发射"，保存后触发 L1 embedding 去重检查

#### Scenario: 复用已有事件类型
- **WHEN** L2 "OpenAI" 需要 L1 且已有类型中有 "产品发布"（embedding 相似度 0.91）
- **THEN** LLM 选择 "产品发布" 作为父标签，不创建新类型

### Requirement: L1/L2 embedding 优先去重
创建新 L1 或 L2 标签后，系统 SHALL 立即进行 embedding 去重检查。L1 去重阈值 0.90（embedding → LLM 确认是否合并）；L2 去重阈值 0.95（embedding 直接合并，不调 LLM）。

#### Scenario: L2 高相似直接合并
- **WHEN** 新创建 L2 "OpenAI公司" 且已有 L2 "OpenAI" 的 embedding 相似度为 0.97
- **THEN** 系统不创建新标签，直接复用 "OpenAI"

#### Scenario: L1 相似调 LLM 判断
- **WHEN** 新创建 L1 "新品发布" 且已有 L1 "产品发布" 的 embedding 相似度为 0.92
- **THEN** 系统调用 LLM 判断是否为同一概念，LLM 确认后合并为 "产品发布"

### Requirement: 配置变更生成待处理清单
修改层级模板配置后，系统 SHALL 扫描所有现有标签和关系，将违反新规则的项目记录到 `hierarchy_pending_changes` 表，状态为 `pending`。

#### Scenario: 模板最大深度减小后标记超限标签
- **WHEN** event 模板从 3 层改为 2 层
- **THEN** 所有 depth=3 的 event 标签及其子关系被标记为待处理，原因 "depth_exceeded"

#### Scenario: 修改层级约束后标记不匹配标签
- **WHEN** L3 新增 `is_leaf=true` 约束
- **THEN** 所有非叶子的 L3 标签（即 depth≥3 且有子标签的）被标记为待处理

### Requirement: 手动触发 rebuild 重新整理
用户 SHALL 能够通过 `POST /api/hierarchy/rebuild` 触发重新整理，对 `pending` 状态的标签按模板重新放置。支持 `category` 过滤和 `dry_run` 预览。

#### Scenario: rebuild 重新挂载待处理标签
- **WHEN** 用户 POST /api/hierarchy/rebuild 且 category=event
- **THEN** 系统对每个 pending 的 event 标签重新走向上聚合流程，完成后标记为 `resolved`

#### Scenario: dry_run 预览
- **WHEN** 用户 POST /api/hierarchy/rebuild 且 dry_run=true
- **THEN** 系统返回待处理标签列表和预计操作，不做实际修改

### Requirement: 配置版本管理
每次配置修改 SHALL 生成新版本号（递增），旧配置保留在 `hierarchy_config_versions` 表中。支持按版本回滚。

#### Scenario: 配置修改自动递增版本
- **WHEN** 管理员修改模板配置
- **THEN** version 字段从 1 递增到 2，旧配置写入 versions 表
