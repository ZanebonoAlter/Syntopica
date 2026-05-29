## Why

docs/reference/ 下的功能说明文档（frontend-features.md、content-processing.md、reading-preferences.md、frontend-components.md）存在两个问题：

1. **过时路径**：content-processing.md 引用已不存在的 contentprocessing/ 目录（已改名 content/）；frontend.md 和 frontend-components.md 引用已删除的 digest 路由页面，缺少新增的 tags 路由。
2. **职责混淆**：这些文档既不是"技术参考"也不是"开发规范"，而是功能使用说明，放在 reference/ 下定位不清。

同时，随着 v1.x 里程碑推进，功能说明散落在各里程碑的 user-guide/ 里，缺少一个统一的、面向用户的功能手册入口。

## What Changes

- 新建 docs/userguide/ 目录，按功能域组织 6 份用户手册（reading、feeds、ai、topic-graph、tags、narrative）
- 新建 docs/archive/ 目录，将 reference/ 下 4 份功能说明文档移入归档
- 修正 reference/architecture/ 下 3 份文档的过时路径（contentprocessing 改为 content、digest 路由删除、tags 路由补全）
- 更新 docs/README.md 索引反映新结构

## Capabilities

### New Capabilities
- userguide-directory: 用户手册目录，按功能域组织，作为用户理解系统能力的统一入口
- archive-directory: 归档目录，存放从 reference/ 迁出的功能说明文档

### Modified Capabilities
- docs-reference-layer: reference/ 不再包含功能说明文档（frontend-features、content-processing、reading-preferences 移出），architecture/ 下的路径引用需修正

## Impact

- 文件操作：新建 2 个目录、6 份用户手册；移动 4 份文档到 archive/；修正 3 份 architecture 文档
- 无代码变更：纯文档重组，不影响任何运行时代码
- 无 API 变更
