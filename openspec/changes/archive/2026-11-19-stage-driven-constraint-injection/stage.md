<!-- stage: implement -->
<!-- constraint-domains: -->

# stage — stage-driven-constraint-injection

本变更只涉及 harness，不声明业务域。其他变更可在头部的 `constraint-domains` 标记中填写逗号分隔的 flow 文件名（不含 `.md`），与 `proposal.md` 的声明取并集。

> **本文件是什么**：openspec change 的**阶段载体**（非 schema 制品，与 `explore-findings.md` /
> `human-feedback.md` / `test-cases.md` 同性质）。头部一条 `<!-- stage: <值> -->` marker 是
> constraint-injection extension 的**唯一进度真相源**——扩展据此决定注入哪些红线层、装配哪份
> 文档路由清单；`openspec` schema 的制品 DAG 不因本文件改变。
>
> **契约**：`openspec/changes/stage-driven-constraint-injection/specs/constraint-injection/spec.md`
> 「stage 进度标识（替代档位机制）」；实现在 `.pi/extensions/lib/stage.ts`（`parseStageMarker` /
> `readStageFile` / `buildStageRouting`）。

## 阶段取值（6 档单调阶梯，顺序即进度）

| stage | 语义 | 阶段族（派生值） |
| --- | --- | --- |
| `explore` | 探索/脑暴期，尚无制品 | planning |
| `propose` | proposal 制品产出期 | planning |
| `design` | design / ui-design / specs 制品产出期 | planning |
| `implement` | 实现执行期（tasks 勾选推进）——**本 change 当前处在此档** | implementation |
| `verify` | 验收期 | implementation |
| `archive` | 归档期 | implementation |

阶段族是**派生值**（`explore`/`propose`/`design` → planning，其余 → implementation），不可单独书写。
marker 缺失（无本文件或无 marker）= legacy change，扩展不绑定阶段、不阻断任何流程（含归档）；
marker 重复或值不在上表 = invalid，同样降级不阻断（只记一条 warn）。

## 本阶段的文档路由清单（人读版，与扩展实际装配的清单同源）

清单由 `.pi/constraint-injection.json` 的 `stages.<stage>.docs` 派生（路径）+ `.pi/extensions/lib/stage.ts`
的 `ROUTING_DOC_CATALOG`（节名）；扩展注入的是**路径 + 节名 + 取数方式**，**不含正文**——需要细节自行 `read`。

| stage | 清单构成（累加） |
| --- | --- |
| `explore` / `propose` | `constraints-index.md`「执行规范」+ 声明域 flow 文档「业务约束与不变量」（红线摘要命令 + 细节层 read 指引） |
| `design` | 上述 + `standard/frontend/layout.md`、`standard/frontend/interaction.md` |
| `implement` | 上述 + 代码与测试规范（按栈命中 `standard/backend/`：code-style / lint / package-layout / testing，或 `standard/frontend/`：code-style / lint / testing） |
| `verify` / `archive` | 上述 + `standard/shared/test-design.md`、`standard/shared/commit-pr.md`、`standard/shared/doc-authoring.md` |

另有两类条目按会话事实动态追加（同样只给路径，不注正文）：对话关键词命中（**域限定**：仅基础清单内的
文档被标注「本次会话提及，优先」）、编辑路径命中 `doc-impact-applies` 标签的文档（文件系统事实）。

取红线摘要（不读整篇）：

```bash
bash scripts/harness/constraint-digest.sh --brief --domains <域>
bash scripts/harness/constraint-digest.sh --brief --paths <目标路径>
```

## 阶段推进约定

- **写 marker 即推进**：切到哪一档就改头部 marker 那一行（本文件可以只有这一行 + 说明）；
  扩展每个回合重读，无需重启会话。
- `explore` → `propose`：proposal 落地后推进；`propose` → `design`：开写 design/specs/ui-design 时推进；
  `design` → `implement`：tasks 开始勾选、进入代码实现时推进；`implement` → `verify`：实现全绿、开始验收时推进；
  `verify` → `archive`：归档就绪（§11 门禁全绿）时推进。
- 实现期新发现的业务域**只补在 `constraint-domains`**（与 proposal.md 取并集），不要靠改 stage 表达。
- 存量 change 无需补本文件（缺失 = legacy，不阻断）；补了才会获得阶段路由清单。
