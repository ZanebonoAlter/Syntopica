# #9 - 前端层级模板配置未写入后端

## What to build

数据库 `hierarchy_config.templates = {}`（空），导致 `PlaceTagInHierarchy` 对所有 category 立即返回 `"no hierarchy template"` 错误，0 个层级放置 LLM 调用，0 个 pending_changes，UI 上"LLM 建议"一片空白。

前端 `TemplateSettingsDialog` 可能有硬编码的模板定义或未正确调用保存 API。需要排查：
1. 前端模板配置的初始化来源（硬编码 vs 从后端读取）
2. 保存模板时是否正确调用 `PUT /hierarchy/config`
3. 后端 API 是否正确将 templates 写入 `hierarchy_config` 表
4. 确保配置写入后 `PlaceTagInHierarchy` 能正确获取模板并执行

这是标签层级闭环的前置依赖——没有模板，整个放置流程不会启动。

## Acceptance criteria

- [ ] 前端 TemplateSettingsDialog 打开时显示有意义的默认模板
- [ ] 保存模板后 `hierarchy_config.templates` 非空，包含 event/person/keyword 的层级定义
- [ ] 新标签创建后 `PlaceTagInHierarchy` 不再报 "no hierarchy template" 错误
- [ ] ai_call_logs 中出现 `hierarchy_match` / `hierarchy_create` 操作记录
- [ ] UI 上 PendingChange 面板能显示 LLM 生成的建议

## Blocked by

None - can start immediately. 建议在 #5-#8 之前或并行处理，因为这是层级功能能否工作的根因。
