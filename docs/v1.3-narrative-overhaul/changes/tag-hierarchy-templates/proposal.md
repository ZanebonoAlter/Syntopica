## Why

当前标签层级结构缺乏"抽象程度"的显式建模，LLM 判断父子关系时只有二元选择（parent/child），不知道目标标签应该在哪个抽象层级。实际准确率约 70-80%，跨层级错误频繁出现（如 "DeepSeek 外部融资 > 大模型发布与生态动态 > OpenAI 运营与产品细节"——同级标签被错误链成父子）。根本原因是架构缺少层级模板约束，每次判断都是局部的、无上下文的。

## What Changes

- 引入固定层级模板系统：event 3 层、person 2 层、keyword 3 个子类各 3 层
- 所有新标签默认为叶子节点（L3），通过向上聚合查找/创建父标签
- 层级通过路径深度反推（方案 B），不改数据库表结构
- 模板层级可配置（管理员调整层级的名称、描述、约束），但不能新增或删除模板
- 配置变更生成待处理清单，用户手动触发 `rebuild` 重新整理
- Person 从现有 4 层隐式结构简化为 2 层（人物群组 → 具体人物）
- Event L1 事件类型动态 LLM 生成，已有类型作 few-shot 参考
- 嵌入 L1/L2 去重：embedding 优先（阈值 0.90/0.95），LLM 兜底
- 改造现有清理调度器 Phase 3/4/6，对齐模板约束

## Capabilities

### New Capabilities
- `hierarchy-template-config`: 层级模板定义、存储、加载和层级调整配置。固定模板库 + 可调层级参数，支持深度反推层级判定，配置变更生成待处理清单和手动 rebuild 触发。

### Modified Capabilities
- `tag-hierarchy-quality`: Phase 3 增加模板深度/跨分类合规检查；Phase 4 收养范围限定同模板同层；Phase 6 从"LLM 整树审查"改为"模板对齐审查"（层级对齐检查、L1/L2 embedding 去重、叶子归属抽样复查）。不自动修复，生成待处理清单。

## Impact

- **Backend**: `internal/domain/topicanalysis/` 新增 `hierarchy_config.go`、`hierarchy_template.go`；修改 `abstract_tag_judgment.go`（注入层级上下文到 prompt）、`hierarchy_cleanup.go`（Phase 6 重写）；修改 `jobs/tag_hierarchy_cleanup.go`（Phase 3/4/6 改造）
- **Database**: 新增 `hierarchy_config` 和 `hierarchy_pending_changes` 两张表；`topic_tags` 表不需要变更
- **API**: 新增 `GET/PUT /api/hierarchy/config`、`GET /api/hierarchy/pending`、`POST /api/hierarchy/rebuild`
- **Frontend**: 新增层级配置页面和待处理管理组件
- **Migration**: 需要为现有标签打上层级（一次性脚本，基于深度反推）
