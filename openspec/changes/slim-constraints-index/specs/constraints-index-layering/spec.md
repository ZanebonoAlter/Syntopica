# constraints-index-layering

<!-- domain: harness -->

## ADDED Requirements

### Requirement: 约束索引只承载路由信息

常驻注入的 `docs/reference/constraints-index.md` SHALL 只包含「去哪儿找约束」的路由信息（域清单、规范目录、流程文档指针），SHALL NOT 包含操作性说明（载体样例、多步取数流程、完整 marker 枚举）。

允许保留**单行格式提示**（如 `<!-- constraint-domains: 域名, ... -->` 的写法骨架）——它是「往哪写」而非「怎么用」。

#### Scenario: 域清单完整

- **WHEN** 列举 `docs/reference/flow/` 下所有含「业务约束与不变量」节的文档
- **THEN** `constraints-index.md` 的域表逐条列出它们的 basename
- **AND** 无遗漏、无指向不存在文件的链接

#### Scenario: 操作说明不在索引内

- **WHEN** 在 `constraints-index.md` 中搜索 `stage.md` 载体样例或六取值枚举
- **THEN** 不存在这些内容
- **AND** 索引只给一句指向 `开发执行规范.md` §13 与 `AGENTS.md` 的指针
- **AND** `<!-- constraint-domains: ... -->` 的单行写法骨架允许保留（属于「往哪写」而非「怎么用」）

#### Scenario: 体积上限

- **WHEN** 统计 `constraints-index.md` 的字节数
- **THEN** 不超过 3000 字节（原为 6032 字节）

### Requirement: 头部声明格式集中在开发执行规范

`docs/reference/开发执行规范.md` §13 SHALL 说明 change 的三种头部声明（`stage` / `constraint-domains` / `ui-impact` 与 `diagrams`），包括各自写在哪、缺失后果、以及域声明的两条规则（两处并集、stage.md 只增不减）。

#### Scenario: 路由清单能指到

- **WHEN** change 处于 `explore` 或 `propose` 阶段
- **THEN** 阶段路由清单包含 `开发执行规范.md`
- **AND** `.pi/extensions/lib/stage.ts` 的 `ROUTING_DOC_CATALOG` 里该路径的节名与文档实际节名一致

#### Scenario: 三种声明齐全

- **WHEN** 读者打开 §13
- **THEN** 能看到 `stage` / `constraint-domains` / `ui-impact`+`diagrams` 三者的写法与缺失后果
- **AND** 能看到取红线的命令 `constraint-digest.sh --brief`

### Requirement: 机制说明在 AGENTS.md 保底

`AGENTS.md` SHALL 保留「阶段驱动 + 自主导入」段，说明阶段 marker、域声明两处并集、模型自主 read 约定与派发 brief 仍必做。

#### Scenario: 索引与 AGENTS 不重复

- **WHEN** 对比 `constraints-index.md` 与 `AGENTS.md` 中关于阶段/域声明的内容
- **THEN** 索引只有指针，完整说明只在 `AGENTS.md` 一处
- **AND** 修改协议时只需改约定位置（`AGENTS.md`）+ 格式细节（§13），不必改索引
