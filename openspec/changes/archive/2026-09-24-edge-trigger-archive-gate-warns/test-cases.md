# test-cases — edge-trigger-archive-gate-warns

> 白盒用例文档（case-first-testing，complex 档）。主链路故事：**归档重试循环中 warn 只说一次**。
> 被测状态机：spec-gate 检查⑤/⑤' warn 投递边沿（deliver / silent / close 三态）。
> 断言判据主线程定；测试落点 `.pi/extensions/tests/spec-gate.smoke.cjs`（mock `tool_call` 两次同会话驱动）。

## 主链路：归档重试循环中 warn 只说一次

完整故事——归档是「block → 修一项 → 重试」循环，⑤/⑤' 的 warn 在清单/违例集合未变时只投一轮；修掉后转绿收尾一行；新会话/compact 后视同首见。

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 首次 `openspec archive foo`：⑤' exit 2（清单 `b.go`）+ ⑤ 扫描出 1 条违例 | 树上有其他 change 归属文件时 warn / 白盒用例缺失 warn 留痕 | warn 全量投递 + 每条一条记账；⑤'/⑤ 条目写入指纹表 | smoke（mock pi.exec） | spec-gate.smoke.cjs · 边沿用例 A1/W1 |
| 2 | 修了一项归档检查，重试：⑤' 仍 exit 2 同清单 + ⑤ 同违例集合 | 同指纹重试静默 / 同指纹重试不重复投递 | 零 sendMessage、零 warn 记账；console 一行同指纹静默留痕；block 判定不受影响 | smoke | A2/W2 |
| 3 | 指纹变化重试：⑤' 清单变（`b.go`→`b.go,c.go`）+ ⑤ 违例集合变（新增关键词命中） | 清单变化重新输出 / 违例集合变化重新投递 | 视同首见：全量重投 + 再记账，条目更新为新指纹 | smoke | A3/W3 |
| 4 | 修完重试：⑤' exit 0（树上无归属他 change 文件）+ ⑤ 零违例（已补 test-cases.md） | 转净收尾一行 / 违例清零收尾一行 | 各一行「✓ 已转净/已清零」收尾投递 + 删条目 + 零 warn 记账 | smoke | A4/W4 |
| 5 | 转净后再犯（又出现归属文件/违例） | 转净收尾一行（后续恢复首见语义） | 条目已删 → 回到首见全量投递 + 记账 | smoke | A5/W5 |
| 6 | 全新会话（新 sessionId）首次尝试，清单/违例与步 1 相同 | 新会话首见重新提醒 | 仍全量投递 + 记账（会话边界清零，不跨会话去重） | smoke | A6 |
| 7 | 同会话 `session_compact` 后同指纹再试 | （design D4：compact 重发一次） | 重投一次 + 再记账（compact 摘要掉早先 warning，重发是正确性要求） | smoke（直调 compact handler 清条目） | A7 |
| 8 | 真实归档 dry-run 自查 | （链路回归） | archive-readiness.sh 链路无回归，⑤/⑤' 检查脚本本身零改动 | 人工 | V5 |

## ⓪ 继承与调整表（test-assets.sh 反查 + 逐行处置）

| 旧 Scenario（资产） | 现行为 | 本 change 处置 | 防回归判据 |
| --- | --- | --- | --- |
| 树上有其他 change 归属文件时 warn（spec-gate.smoke.cjs 7e'） | ⑤' exit 2 → warn 不 block + 记账 | **继承**：首见路径即此语义 | 首次尝试 messages 含「检查⑤'」+ rows 恰 1 条 concurrent-dirty-tree（A1） |
| 冷启动跳过（spec-gate.smoke.cjs 7e' exit 3） | exit 3 零输出零记账 | **继承**：exit 3 不触碰指纹表（不收尾不投递） | exit 3 尝试零 messages 零 rows（既有断言保持绿） |
| 无违例静默放行（spec-gate.smoke.cjs 1/7e） | ⑤ 扫描空数组 → 零输出 | **继承**：无条目时零违例仍零输出（不冒出空收尾） | passExec 全过 → undefined + 零 policy.decision（既有断言保持绿） |
| 白盒用例缺失 warn 留痕（spec-gate.smoke.cjs 2/7d） | ⑤a/⑤b 命中 → warning + 记账 | **继承**：deliver 路径逐条投递逐条记账（粒度不变） | 首次尝试 messages 含「检查⑤」+ rows 恰 1 条 acceptance-wording（W1） |
| 纯函数任务提 SQLite warn 留痕（spec-gate.smoke.cjs 3） | ⑤b 分层错配 → warning | **继承**：纯函数 `scanAcceptanceWording` 零改动 | 全部既有纯函数断言保持绿 |
| 检查⑤异常不阻断归档（spec-gate.smoke.cjs 7d 语境） | 检查⑤自身异常 fail-open | **继承**：异常路径不进指纹表（D5/D7） | 既有异常 fail-open 断言保持绿 |
| 豁免通道兼容（7b/7c/8h；豁免路径短路先于检查⑤） | `--force`/`SPEC_GATE_BYPASS=1` → bypass 留痕，⑤/⑤' 不执行 | **继承**：bypass/fail-open/无名 fail-open 的 warning **不进指纹表**（D5，保持每尝试一条） | 既有 bypass 断言保持绿 + diff 核对（2.4） |

新增 9 个边沿 Scenario（⑥'×6 + ⑤×3，见 delta specs）为**扩展**：同指纹重试静默（⑥'）、清单变化重新输出（⑥'）、转净收尾一行（⑥'）、新会话首见重新提醒（⑥'）、同指纹重试不重复投递（⑤）、违例集合变化重新投递（⑤）、违例清零收尾一行（⑤）×主链路步 2/3/4 覆盖，其余见白盒附加。

## 白盒附加（边沿判定矩阵与状态表边界）

**纯函数 `decideWarnEdge(prevFp, currFp)` 三态矩阵**（无 DB 依赖，导出供 smoke 直测）：

| prev（上次指纹） | curr（本次指纹/干净） | 输出 | 语义 |
| --- | --- | --- | --- |
| null（无条目） | fp | `deliver` | 首见 |
| fp1 | fp1（相同） | `silent` | 同指纹 |
| fp1 | fp2（不同） | `deliver` | 指纹变化视同首见 |
| fp1 | null（干净） | `close` | 转绿收尾 |
| null | null | `silent` | 防御：无条目且干净无动作（不冒空收尾） |

「转净后再犯」= close 已删条目 → prev 回到 null → 回首见行。步 5/A5、W5 覆盖。

**指纹取值稳定性**（⑤' 清单行规范化：trim→滤空→排序去重→join 后 sha256 取 16 字节 hex 前缀；⑤ 违例文案 join 后同哈希）：

| 变体 | 输入 | 期望 |
| --- | --- | --- |
| 空清单 | `""` | 指纹 = sha256("") 前缀，不抛异常 |
| 单行 | `"b.go"` | 稳定指纹 |
| 重复行 | `"b.go\nb.go"` | 与单行同指纹（去重） |
| 顺序颠倒 | `"a.go\nb.go"` vs `"b.go\na.go"` | 同指纹（排序） |
| 行首尾空白 | `" b.go "` vs `"b.go"` | 同指纹（trim） |
| 纯空白行/空行混入 | `"\n  \nb.go\n"` vs `"b.go"` | 同指纹（滤空+trim） |
| 超长清单（>20 行） | 1000 行路径清单 | 不抛异常，指纹仍 32 hex 字符（无行数上限） |
| ⑤ 文案顺序保留 | `["w1","w2"]` vs `["w2","w1"]` | **不同**指纹（join 不排序——扫描顺序由常量词表序决定，保留「新增关键词排前」语义差异进指纹，design D2） |
| 16 字节前缀 | 任意输入 | 指纹长度 32 hex 字符（16 字节） |

**状态表边界**（`warnStates: Map<sessionKey, { at, items: Map<cacheKey, fp> }>`，LRU 上限 32）：

| 变体 | 场景 | 期望 |
| --- | --- | --- |
| 空 sessionId 兜底槽 | `getSessionId` 返回 undefined/空串 | 落 `__no-session__` 单一兜底槽，行为等价隔离前（首见投递） |
| 同会话跨 change 键隔离 | 同 sessionId 对 change foo warn 后归档 bar（同指纹输入） | bar 仍首见投递（cacheKey=`<change>:<kind>` 隔离） |
| 同会话跨 kind 隔离 | 同 change ⑤ 已静默、⑤' 首次 exit 2 | ⑤' 仍首见投递（kind 段隔离） |
| LRU 超限淘汰 | 制造 33 个会话条目 | 最久未写会话条目被淘汰，其余不受影响；淘汰后该会话视同首见 |
| compact 清空后重发 | 同会话同指纹第二次静默 → 触发 `session_compact` → 第三次同指纹 | 第三次重新投递 + 再记账 |
| 异常 fail-open（D7） | 指纹计算/Map 操作抛异常（如注入畸形 stdout） | 按无状态全量投递 + console.warn 一行，不阻断归档不改 block 裁决 |

**不适用项划除留痕**（case-first-testing 变体走查要求：不适用的变体显式划除，不静默跳过）：

- ~~时间窗口/TTL 过期~~——状态是会话内存态，生命周期天然由会话边界/compact 界定，无跨会话持久化即无过期语义（design D1，Non-Goal）。
- ~~幂等并发/竞态双写~~——单进程单线程事件循环，tool_call handler 串行执行，无并发写指纹表路径。
- ~~指纹碰撞合并~~——16 字节前缀在会话内 ≤32 会话 × 每 change 2 kind 的状态下碰撞概率可忽略，不做全文比对。
- ~~磁盘持久化/跨进程恢复~~——Non-Goal（design D1：新会话首见警告必要）。
