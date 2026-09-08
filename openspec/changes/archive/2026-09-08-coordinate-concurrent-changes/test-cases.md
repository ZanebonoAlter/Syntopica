# test-cases — coordinate-concurrent-changes（complex 白盒用例）

> 故事主线：并发 change `foo` 与 `bar` 在共享树上工作——`foo` 的会话编辑文件（quality-gate 聚合归属落库）→ 归档前拉态势（concurrency-status）→ spec-gate 归档检查发现 `bar` 的归属脏文件（warn 不 block）→ doc-impact verify 只扫 `foo` 归属（`bar` 的 handler 文件不误报）→ `foo` 主体 commit 后树上只剩 `bar` 的文件。

## 主链路表

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
|---|---|---|---|---|---|
| 1 | 绑定 change 的会话编辑 `a.go`，turn_end 触发 | harness-fact-log「edit.map 随词汇扩展落库」 | events.db 新增 edit.map 行，change 列 = boundChange，payload.paths 含 a.go | 扩展单测 | .pi/extensions/tests/quality-gate.smoke.cjs |
| 2 | 下一回合再编辑 `b.go` | concurrent-change-coordination「编辑路径按绑定 change 聚合」 | 最新一条 edit.map 的 paths = {a.go, b.go}（累计快照） | 扩展单测 | .pi/extensions/tests/quality-gate.smoke.cjs |
| 3 | 另一会话绑定 `bar` 编辑 `x.go` | 同上 | `bar` 名下 edit.map 与 `foo` 互不串扰 | 扩展单测 | .pi/extensions/tests/quality-gate.smoke.cjs |
| 4 | 跑 `concurrency-status.sh foo` 人读模式 | 「归档验证前拉取态势」 | 三段输出：活跃清单 / 脏文件归属对照（bar 的 x.go 列「归属其他」）/ 近期验证流水 | 脚本 smoke | scripts/concurrency-status.smoke.sh |
| 5 | `concurrency-status.sh --check foo` | 同上 | exit 2 + stdout JSON 含 x.go 与 bar | 脚本 smoke | scripts/concurrency-status.smoke.sh |
| 6 | openspec archive foo（树上 x.go 未 commit） | 「树上有其他 change 归属文件时 warn」 | steer warn（含 x.go 与 bar），归档不被 block；policy.decision(action=warn, reasonCode=concurrent-dirty-tree) | 扩展单测 | .pi/extensions/tests/spec-gate.smoke.cjs |
| 7 | `doc-impact.sh verify <foo-dir>`，树上 x.go 是 handler 但 foo 未声明 api | doc-impact-gate「疑似遗漏按归属地图过滤」 | 反向启发式只扫 foo 归属集合，x.go 不触发疑似遗漏；退出码 0 | 脚本 smoke | scripts/doc-impact.smoke.sh |
| 8 | foo 主体 commit（git add 仅 foo 归属文件） | 「门禁绿即主体 commit」 | 树上只剩 bar 的文件；磁盘文件内容不变 | 人工 | 见 tasks 8.5 自证 |

## 白盒附加 — 分支表

### lib/edit-map.ts 纯函数

| # | 分支/输入 | 期望 |
|---|---|---|
| B1 | parseModeSet：合法行 `{mode:"implementation", boundChange:"foo"}` | `{boundChange:"foo"}` |
| B2 | payload JSON 损坏（截断/非 JSON） | null（跳过不落） |
| B3 | payload 合法但 boundChange 为 null（requirements 无绑定） | null |
| B4 | mergeIntoBase(["b","a"], ["a","c"]) | ["a","b","c"]（排序去重） |
| B5 | mergeIntoBase([], []) | []（空集边界） |
| B6 | buildPayload 500+ 路径 | n=paths.length，JSON 可逆解析 |

### quality-gate 聚合（模块状态机）

| # | 场景 | 期望 |
|---|---|---|
| Q1 | trigger 非空 + boundChange=foo + 库内无 foo 旧快照 | 首条 edit.map = base∅∪trigger |
| Q2 | 连续两回合（Q1 后再编辑） | 快照累计（并集语义） |
| Q3 | 同 session 无 mode.set 行 | 零 edit.map 事件（spec「无档会话不计入」） |
| Q4 | 纯对话回合（无 CODE_TOOLS、无粘性失败） | turn_end 早退，零事件 |
| Q5 | boundChange 从 foo 切到 bar | base 重读 bar 最新快照；foo 名下不再增长 |
| Q6 | session_start（reason=reload） | editMapBase/cachedChange 清零重置 |
| Q7 | session_start（reason=startup，子线程） | 状态不清（ownerSessionId 防御） |
| Q8 | 跨 session：session B 首次落 bar | base 从库读 session A 的最新快照起步（并集不丢） |

### concurrency-status.sh --check 判定矩阵

| # | 输入态 | exit | 备注 |
|---|---|---|---|
| C1 | 脏文件空 | 0 | |
| C2 | 脏文件全归属本 change | 0 | 自己的文件不 warn |
| C3 | 含归属其他 active change 文件 | 2 | stdout JSON：files+changes |
| C4 | 脏文件全无归属 | 3 | 冷启动跳过（explore-findings §8） |
| C5 | 冲突文件（归属 foo 与 bar 两个 change） | 人读带 ⚠ 冲突标记 | spec「同文件双 change 触碰标记冲突」 |
| C6 | 归属 change 已 archive | 视同无归属 | 归档即 commit |
| C7 | sqlite3 缺失 / 库不存在 / 查询异常 | 3（--check）；人读降级输出提示 | fail-open |
| C8 | 并发写库时只读查询 | 正常返回不锁死 | mode=ro + busy_timeout |

### spec-gate 检查⑤'

| # | 输入 | 期望 |
|---|---|---|
| S1 | --check exit 2 | warn steer + policy.decision 记账，不进 failures 不 block |
| S2 | --check exit 0 / 3 | 零输出零记账 |
| S3 | 脚本超时/执行异常 | fail-open：不 warn 不 block，console.warn 留痕 |

### doc-impact.sh verify 双轨

| # | 输入态 | 期望 |
|---|---|---|
| D1 | foo 有非空 edit.map，树上 bar 的 handler 文件 | 疑似遗漏只扫 foo 归属，不误报（主链路步 7） |
| D2 | foo 无 edit.map（存量 change） | 回退全树 $changed（现状行为），bar 文件可触发疑似遗漏 |
| D3 | edit.map paths 为空数组 | 视同无记录回退全树（防御边界） |
| D4 | 既有 doc-impact-excuse 注释 | 解析不报错不判 FAIL |
| D5 | 声明了未更新（规则2） | 双轨下仍用全树 $changed 对账（不变） |
| D6 | sqlite3 不可用 | 回退全树（与 C7 同 fail-open 语义） |

## 变体走查（五组固定清单）

- **输入**：payload 损坏（B2）✓｜空集合（B5/D3）✓｜单文件（Q1）✓｜路径含中文/空格 → git core.quotepath=false 已有（doc-impact 既有），edit.map paths 原样字符串透传不解析 ✓｜超长（B6 500+）✓
- **前置**：库不存在（C7/D6 回退）✓｜单 change 无并发（C1/C2 exit 0）✓｜重复路径（B4 去重）✓｜引用已 archive change（C6）✓
- **时间窗口**：edit.map 31 天过期 → TTL 清扫后归属消失 → verify/--check 走回退路径（C4/D2 同形）✓｜跨 session 时序（Q8）✓
- **幂等**：同回合重复 turn_end？turn_end 每回合一次，无重复入口 ✓｜部分失败重试（logEvent false → 跳过，下回合 trigger 含新增时重落全量快照，自愈）✓｜并发写库（WAL + busy_timeout，C8）✓
- **可用性（UI 类，本 change 无 UI）**：误输入（--check 无参数 → usage + exit 3）✓｜空态（C1/无 active change → 人读输出空态提示）✓｜错误态（C7 降级提示）✓｜超长输出（流水摘要截断前 20 条）✓

## 继承与调整（⓪ 改契约了吗）

本 change MODIFIED 两个 Requirement：

- harness-fact-log「事件类型词汇与保留期」：旧断言"十类事件"→ 十一类；既有 smoke（TTL 分级清扫等 5 个 Scenario）**全部保留**，新增 edit.map 断言一条 + TTL 用例扩展 edit.map 行。旧测试无需改删除，只扩枚举期望
- doc-impact-gate「归档前对账（verify）」：既有 smoke（check-standards.smoke.sh F 段间接跑 verify）断言全树行为——**回退轨（D2）与现状同形，既有断言不破**；新增归属轨断言（D1）落 scripts/doc-impact.smoke.sh（新文件，不与既有 F 段 smoke 耦合）
