## ADDED Requirements

### Requirement: Userguide directory structure
docs/userguide/ SHALL 包含 6 份按功能域组织的用户手册文件：

| 文件 | 功能域 | 内容来源 |
|------|--------|----------|
| reading.md | 阅读功能 | frontend-features.md "主阅读页"+"文章阅读" + reading-preferences.md 用户可见部分 |
| feeds-and-categories.md | 订阅与分类管理 | frontend-features.md "Feed与分类管理" |
| ai-features.md | AI 功能 | frontend-features.md "AI总结"+"AI Provider管理" |
| topic-graph.md | Topic Graph | frontend-features.md "Topic Graph" |
| tags.md | 标签系统 | frontend-features.md "文章标签" |
| narrative.md | 叙事面板 | frontend-features.md 中 Topic Graph 章节的叙事相关内容 |

#### Scenario: Userguide directory listing
- **WHEN** 列出 docs/userguide/
- **THEN** 可见 reading.md、feeds-and-categories.md、ai-features.md、topic-graph.md、tags.md、narrative.md 6 个文件

### Requirement: Userguide content orientation
docs/userguide/ 下的文档 SHALL 面向用户，描述"系统能做什么、怎么用"，而非"代码怎么实现"。技术实现细节留在 docs/reference/ 中。

#### Scenario: User reads about reading features
- **WHEN** 用户打开 docs/userguide/reading.md
- **THEN** 看到阅读页面的三栏布局、文章阅读操作、已读追踪、内容增强等用户可见功能的说明

#### Scenario: No code paths in userguide
- **WHEN** 搜索 docs/userguide/ 下的文件
- **THEN** 不包含 backend-go/internal/ 或 front/app/features/ 等代码路径引用

### Requirement: Userguide content source
userguide 文档的内容 SHALL 从现有文档拆分提取，保持原文表述，不做内容重写。

#### Scenario: Reading guide content source
- **WHEN** 对比 docs/userguide/reading.md 和 docs/reference/frontend-features.md 的"主阅读页"+"文章阅读"章节
- **THEN** 核心内容一致，用户可见功能描述相同

### Requirement: Userguide update mechanism
每个 v1.x 里程碑完成后，任务输出的功能文档 SHALL 手动更新到 docs/userguide/ 对应文件中。

#### Scenario: Post-milestone userguide update
- **WHEN** v1.3 完成并引入新的标签管理功能
- **THEN** docs/userguide/tags.md 被手动更新以包含新功能说明

### Requirement: Userguide not mixed with reference
docs/userguide/ 下的文件 SHALL NOT 出现在 docs/reference/ 目录中。

#### Scenario: No duplicate userguide files in reference
- **WHEN** 在 docs/reference/ 下搜索
- **THEN** 不存在 frontend-features.md、content-processing.md、reading-preferences.md 文件
