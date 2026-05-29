## 1. 创建 userguide 目录

- [x] 1.1 创建 docs/userguide/reading.md — 从 frontend-features.md 提取"主阅读页"+"文章阅读"章节，合并 reading-preferences.md 用户可见部分（偏好如何影响阅读体验）
- [x] 1.2 创建 docs/userguide/feeds-and-categories.md — 从 frontend-features.md 提取"Feed与分类管理"章节
- [x] 1.3 创建 docs/userguide/ai-features.md — 从 frontend-features.md 提取"AI总结"+"AI Provider管理"章节
- [x] 1.4 创建 docs/userguide/topic-graph.md — 从 frontend-features.md 提取"Topic Graph"章节（不含叙事部分）
- [x] 1.5 创建 docs/userguide/tags.md — 从 frontend-features.md 提取"文章标签"章节
- [x] 1.6 创建 docs/userguide/narrative.md — 从 frontend-features.md Topic Graph 章节中提取叙事面板相关内容
- [x] 1.7 验证 6 份文件不含代码路径引用（backend-go/internal/、front/app/features/）

## 2. 归档旧功能文档

- [x] 2.1 移动 docs/reference/frontend-features.md 到 docs/archive/frontend-features.md
- [x] 2.2 移动 docs/reference/content-processing.md 到 docs/archive/content-processing.md
- [x] 2.3 移动 docs/reference/reading-preferences.md 到 docs/archive/reading-preferences.md
- [x] 2.4 移动 docs/reference/architecture/frontend-components.md 到 docs/archive/frontend-components.md
- [x] 2.5 验证 reference/ 下不再存在这 4 个文件

## 3. 修正 architecture 文档过时路径

- [x] 3.1 修正 docs/reference/architecture/frontend.md：删除 digest 路由引用（pages/digest/index.vue、pages/digest/[id].vue），补 pages/tags.vue，补 features/tags/、features/hierarchy-config/ 目录
- [x] 3.2 修正 docs/reference/architecture/backend.md：contentprocessing/ 改为 content/，topicanalysis/ 改为 tagging/analysis/，topicextraction/ 改为 tagging/extraction/
- [x] 3.3 验证 grep 不再命中 contentprocessing、pages/digest/ 等过时关键词

## 4. 更新索引

- [x] 4.1 更新 docs/README.md：加入 userguide/ 索引区块，删除归档文件的引用
- [x] 4.2 验证 README.md 所有链接指向正确路径
