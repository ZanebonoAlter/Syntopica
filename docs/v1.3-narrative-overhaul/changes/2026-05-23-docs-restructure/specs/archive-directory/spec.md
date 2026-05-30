## ADDED Requirements

### Requirement: Archive directory structure
docs/archive/ SHALL 存放从 docs/reference/ 迁出的功能说明文档，保留原文不做修改。

#### Scenario: Archive directory listing
- **WHEN** 列出 docs/archive/
- **THEN** 可见 frontend-features.md、content-processing.md、reading-preferences.md、frontend-components.md 4 个文件

### Requirement: Archive documents are original copies
归档文档 SHALL 是原文档的完整移动（非复制），不做内容修改。

#### Scenario: Archive preserves original content
- **WHEN** 对比归档后的 docs/archive/frontend-features.md 和 git 历史中的 docs/reference/frontend-features.md
- **THEN** 内容完全一致

### Requirement: Archive not indexed in README
docs/README.md 的索引 SHALL NOT 列出 docs/archive/ 下的文件。

#### Scenario: README does not list archive
- **WHEN** 阅读 docs/README.md
- **THEN** 不出现 archive/ 目录的链接或文件引用
