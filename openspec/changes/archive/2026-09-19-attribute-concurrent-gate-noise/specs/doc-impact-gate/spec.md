# doc-impact-gate — delta（attribute-concurrent-gate-noise）

## ADDED Requirements

### Requirement: suggest 预勾选输入口径（并发工作树）

`scripts/harness/doc-impact.sh suggest` 的预勾选输入 SHALL 按归属优先取数，MUST NOT 无条件使用全树脏文件：

1. **change 名解析（三源，前者优先）**：显式 `--change <name>` 参数 → 当前会话绑定 change（`PI_SESSION_ID` 指向的事实库 `mode.set` 最新一条的 `boundChange`）→ 解析为空。
2. **归属轨优先**：change 名可解析且事实库存在该 change 的 `edit.map` 归属集合（非空）时，预勾选启发式 SHALL 只对该集合（经黑名单过滤后）执行，输出 SHALL 标注输入口径为归属轨。
3. **全树回退显式标注**：归属集合不存在/为空/事实库或 sqlite3 不可用时，suggest SHALL 回退全树 `git diff` 口径，并 SHALL 在输出中显式标注「回退轨：全树 diff，可能含其他会话/其他 change 的改动」。
4. **三桶分列，不静默过滤**：无论轨别，suggest SHALL 把当前脏文件按归属分为三桶显示——本 change 归属、归属其他 active change、无归属；预勾选只吃「本 change 归属」桶，另两桶 SHALL 列出路径供 Agent 自行认领，MUST NOT 静默丢弃（归属轨只可能少记，静默过滤会制造漏声明）。
5. **退出语义不变**：suggest MUST 保持退出码 0（预勾选是建议不是门禁），MUST 只读（不写事实库、不写 git）。

#### Scenario: 归属集合存在时预勾选只看归属轨

- **WHEN** 会话绑定 change `foo`（事实库有 `foo` 的 `edit.map` 归属集合），树上另有其他 change 的未提交 `backend-go/internal/reader/handler/bar.go`
- **THEN** suggest 的预勾选只由 `foo` 归属集合推导，输出标注归属轨，`bar.go` 不出现在预勾选理由中

#### Scenario: 归属集合为空时全树回退且显式标注

- **WHEN** change `foo` 无 `edit.map` 记录（首次 apply 或全程 bash 编辑），树上有其他 change 的脏文件
- **THEN** suggest 回退全树口径并输出「回退轨：全树 diff，可能含其他会话/其他 change 的改动」标注，同时把其他 change 归属的脏文件列在独立桶中

#### Scenario: 其他 change 归属的脏文件分列不计入预勾选

- **WHEN** 树上有归属 active change `bar` 的 `front/app/x.vue` 与归属本 change 的 `backend-go/internal/reader/service/a.go`
- **THEN** `x.vue` 只出现在「归属其他 change」桶（不参与预勾选），`a.go` 出现在「本 change」桶并参与 `flow` 域预勾选

#### Scenario: change 名解析优先级

- **WHEN** 显式传 `--change bar`，而同会话事实库最新 `mode.set` 的 `boundChange` 为 `foo`
- **THEN** suggest 以 `bar` 为归属查询目标（显式参数优先），输出标注实际使用的 change 名

#### Scenario: sqlite3 不可用时回退且不报错

- **WHEN** 运行环境无 `sqlite3` 或事实库文件不存在
- **THEN** suggest 回退全树口径、输出回退标注、退出码 0（不因缺依赖报错或非零退出）
