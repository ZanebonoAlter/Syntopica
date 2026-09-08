# doc-impact-gate Delta — coordinate-concurrent-changes

## MODIFIED Requirements

### Requirement: 归档前对账（verify）

归档门禁 SHALL 运行 `bash scripts/doc-impact.sh verify <change-dir>` 对每个 active change 对账，以下任一条件 SHALL 判 FAIL：

- tasks.md 缺 `doc-impact` 声明注释
- 声明的文档文件未出现在本 change 文档改动集合中
- 反向启发式命中未声明域（如 `internal/*/handler/` 改动但声明中无 `api`）
- 声明 `none` 但任一启发式命中
- 声明的文档文件路径不存在

"疑似遗漏"反向启发式（第 3、4 条）的改动集合输入 SHALL 优先取归属地图：事实库中该 change 的 `edit.map` 累计路径集合存在且非空时，启发式仅对该集合执行（其他 active change 的脏文件不再干扰，消除跨 change 误报源）；归属地图为空（`edit.map` 词汇上线前的存量 change、或全程无绑定档会话编辑）时 SHALL 回退现状行为（全树 `git diff --name-only <base>` + 未跟踪文件），保持兼容。"声明了未更新"（第 2 条）对账保持全树 git 改动集合不变（文档文件本身始终以 git 为准）。

`doc-impact-excuse` 豁免机制退役：输入收窄后"其他 active change 脏文件干扰"误报源消失，不再新增 excuse 注释；既有 `doc-impact-excuse` 注释 SHALL 保持解析兼容（不报错、不判 FAIL），仅文档标注废弃。

#### Scenario: 声明了未更新

- **WHEN** tasks.md 声明 `docs/reference/api/feeds.md` 但 git 改动集合中无此文件
- **THEN** verify 输出 `声明了未更新: docs/reference/api/feeds.md` 且退出码非零

#### Scenario: 疑似遗漏

- **WHEN** 反向启发式扫描的改动集合（归属地图优先、空则回退全树 git 改动集合）含 `backend-go/internal/reader/handler/foo.go` 且声明中无 `api`
- **THEN** verify 输出 `疑似遗漏: 改了 handler 未声明 api` 且退出码非零

#### Scenario: 疑似遗漏按归属地图过滤

- **WHEN** change `foo` 归属集合仅含 `backend-go/internal/dataenrichment/service/a.go`，树上另有归属 change `bar` 的 `backend-go/internal/reader/handler/foo.go`，`foo` 声明中无 `api`
- **THEN** verify 的反向启发式仅扫 `foo` 归属集合，`bar` 的 handler 文件不触发 `疑似遗漏: 改了 handler 未声明 api`

#### Scenario: 归属地图为空时回退全树

- **WHEN** change `foo` 在事实库中无 `edit.map` 记录（存量 change），树上存在其他 change 的 handler 改动且 `foo` 声明中无 `api`
- **THEN** verify 回退全树 git 改动集合执行反向启发式（现状行为），可命中 `疑似遗漏`

#### Scenario: 既有 excuse 注释兼容

- **WHEN** tasks.md 含历史遗留的 `<!-- doc-impact-excuse: ... -->` 注释
- **THEN** verify 解析不报错、不因该注释存在判 FAIL

#### Scenario: 历史存量豁免

- **WHEN** change 归档日期早于 doc-impact-gate 生效 cutoff
- **THEN** check-standards.sh F 段跳过该校验
