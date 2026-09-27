# test-cases — add-sql-safety-guard（complex 档，⑤a）

测试单元 = 一个 Requirement 的用户故事；交付账本在故事层，故事绿才算交付。层选择：**纯逻辑 → 判定纯函数 + 扩展 smoke（`.cjs`，无真实 DB、无真实拦截面副作用）**——最便宜层，不需要 testcontainer/opencli（无 HTTP、无前端交互）。

## 主链路表（事故复盘主故事）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
|---|---|---|---|---|---|
| 1 | 构造事故原文 `docker exec syntopica-postgres psql -c "DO $$ … TRUNCATE TABLE categories … $$;"` 喂 `tool_call`(bash) | 事故金样例被拦截 | 双键命中（psql ∧ TRUNCATE） | smoke | block===true，reason 含 `[sql-safety-guard]` 与三条出路 |
| 2 | 检查无逃生条件（无 `# allow-truncate-drop`、无 ROLLBACK） | 后缀授权逃生（反向） | 不放行 | smoke | 走到 block 分支 |
| 3 | hard 模式记账 | 模式配置与记账 | `policy.decision(block, sql-safety)` | smoke | 拦截 mock 收到 block + reasonCode=sql-safety |
| 4 | 只读命令对照 `psql -c "select count(*) from articles"` | 只有 psql 无动词放行 | 放行 | smoke | 无 block 返回 |
| 5 | 文档操作对照 `grep -rn TRUNCATE docs/` | 只有动词无 psql 入口放行 | 放行 | smoke | 无 block 返回 |

## 变体走查（五组固定清单）

| 组 | 变体 | 答案 |
|---|---|---|
| 输入 | 空串 / 纯空白命令 | 双键粗筛不过 → 放行 |
| 输入 | 大小写（`truncate`/`Truncate`/`DROP`/`drop`） | `/i` 不敏感 → 全部 block |
| 输入 | 特殊字符（`TRUNCATE;`、`TRUNCATE\n`、`DROP INDEX`、中文注释包围） | 词边界+换行仍命中 → block |
| 输入 | 超长命令（1MB heredoc 含两词） | 正则线性扫描 → block；耗时 <50ms（smoke 计时断言） |
| 输入 | 单 token / 纯分隔符（`;`、`&&`、空白） | 不适用双键 → 划除留痕（无 psql 或无动词必居其一） |
| 前置 | 空集（无 psql 入口） | 放行 |
| 前置 | 单元素（仅 psql 或仅动词） | 放行 |
| 前置 | 部分满足（`-f` 有入口有动词但文件不可读） | 保守 block（spec：无法证明无害即拦） |
| 前置 | 逃生注释 / ROLLBACK / 一次性容器 三选一 present | 放行（各一例） |
| 时间窗口 | 边界两端 / 空窗口 / 跨窗口 / 归一化 | 不适用——判定无时间语义，划除留痕 |
| 幂等 | 重复执行同命令 | 每次独立判定，结果一致（block 幂等） |
| 幂等 | 部分失败重试 / 并发 | 不声称线程安全——扩展为事件回调无共享可变状态，划除留痕（并发正确性由 pi 事件循环保证，非本扩展职责） |
| 可用性 | 误输入反馈 | reason MUST 含三条出路（block 文案即反馈）——smoke 断言 |
| 可用性 | 空态 / 加载态 / 重复提交 / 超长文本 | 空态=空命令放行 ✓；其余不适用（无 UI、无异步状态），划除留痕 |
| 可用性 | 错误态 | soft 模式 notify 不阻断、`-f` 不可读保守拦——各一例 ✓ |

## 效果核对（④）

- **效果**：事故金样例 100% 拦截。**依赖断言外因素**：扩展实际被 harness 加载（目录约定加载，落地会话重载后生效）。
- **核对方法**：① smoke 内 fixture 断言（自动化，本文件主落点）；② 落地后新会话手工执行事故原文（人工：验证方式——期望被 block 且 events.db 出现 `policy.decision(block, sql-safety)`，经 `bash scripts/harness/harness-retro.sh` 或 harness-facts 查询）。
- **量化结果**：见落地后回填（smoke 断言数 / 金样例 1/1 block）。

## 展示字段盘点（⑤）

不适用——无数据结构/用户可见字段变更。

## 白盒附加（分支表 + 边界值）

| # | 条件组合 | 分支结果 |
|---|---|---|
| B1 | ¬psql ∨ ¬动词 | 粗筛放行（off 模式在粗筛前 return，不进本表） |
| B2 | psql ∧ 动词 ∧ 逃生注释 | 放行 + info |
| B3 | psql ∧ 动词 ∧ BEGIN ∧ ROLLBACK | 放行 |
| B4 | psql ∧ 动词 ∧ 一次性容器标志 | 放行 |
| B5 | psql ∧ 动词 ∧ `-f` 文件可读且无两词 | 放行 |
| B6 | psql ∧ 动词 ∧ `-f` 文件含两词 | block（reason 注明文件来源） |
| B7 | psql ∧ 动词 ∧ `-f` 文件不可读/路径解析失败 | block（保守） |
| B8 | psql ∧ 动词 ∧ 无任何放行条件（非 -f 形态） | block |
| B9 | 各 block 分支 × mode=soft | warn + notify，不阻断 |
| B10 | 各分支 × mode=off | 零评估 |

边界值：逃生注释出现在命令**首**部（`# allow-truncate-drop\n psql -c "DROP…"`）——契约是「命令文本内」而非严格尾部，spec 措辞「原文含」→ 放行（smoke 断言）；`--file=` 连写形态与 `-f file` 分写形态各一例；`--command` 长形态一例。

不适用划除：时间窗口组全组、并发组、UI 加载/重复提交组（无对应状态机）——已在上表留痕。
