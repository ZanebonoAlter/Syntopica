# Design

## D1 为什么不判「任务该不该派」

「这个任务够不够大、值不值得派子线程」是主观判断，两个人会吵，同一个人两次也会不一致。而且它需要理解任务全貌——那本身就要读代码，等于先污染再判断。

所以判据换成**行为事实**：读了多大文件、同一文件改了几次。这两个数字客观、可计数、阈值可调，且**恰好对应最该外包的两种形态**：

- 读大文件 = 探索（该派 explore，拿 `file:line` 结论）
- 连续改同一文件 = 调试循环（该派 general-purpose，拿"改到全绿"的结论）

## D2 为什么软提醒而不是硬阻断

硬阻断的代价：真·小修（一行 typo、改个常量）会被拦，而人被拦住的反应是找绕过路径——最后变成一排 `--force` 或干脆用 `bash` 改文件绕过门禁。那比不拦更糟，因为它把「留痕的违规」变成「不留痕的违规」。

软提醒的代价：可以无视。但无视的成本是**可见的**——提醒里带着「主线程 984 次调用烧 $44.96 / 子线程 $0.015」这个对比，无视需要主动决定。

这与本仓库其他门禁的分工一致：机器管「有没有发生」，人管「该不该」。

## D3 豁免为什么必须有理由且进事实库

没有理由的豁免等于「静音开关」，一按就永久安静，事后无法判断是合理豁免还是偷懒。

带理由的豁免：
- 理由非空才生效（`--excuse ""` 直接报错）
- 理由进 `policy.decision` 的 `detail` 字段
- per-session，不写就用不上，不会变成常态

这是 `ui-skip` / `doc-impact-excuse` 同一纪律的第三次复用：**显式留痕优于隐形跳过**。marker 可以不写（隐形），豁免必须写出来（显形）。

## D4 阈值选定的依据

| 阈值 | 取值 | 依据 |
|---|---|---|
| `read` 行数 | 200 | 规范原文「主线程 read 仅限约 200 行内定点核对」——直接抄规范，不新造数字 |
| 连续 `edit` | 3 | 第 1 次是正常修改，第 2 次是补充，第 3 次开始通常是「上一版没解决问题」——即调试循环 |

两个数都是经验值，改常量不需要走 change。

## D5 与既有机制的关系

| 既有机制 | 关系 |
|---|---|
| `session.rollup`（成本记账） | 本 change 的提醒文案**引用**它的实测值，不重复记账 |
| `spill.write`（大输出落盘） | 那是**输出侧**（工具结果太大），本 change 是**输入侧**（读的文件太大），互补 |
| `constraint-injection` 的关键词路由 | 那管「送什么约束进上下文」，本 change 管「谁去干活」 |
| `dev-process-guard` | 那是进程治理，无关 |

## 核心逻辑（伪代码）

### 关键数据结构

```
SessionNudgeState = {
  readWarned:  bool          // 本会话是否已就读大文件提醒过
  editWarned:  bool          // 本会话是否已就连续 edit 提醒过
  editStreak:  Map<path, n>  // 只记当前这轮连续编辑的文件
}
Excuse = { reason: string, ts: string }   // .pi/tmp/router-discipline/<session>.json
```

### 主流程

```
onToolCall(event):
    st = stateFor(sessionId)

    excuse = readExcuse(cwd, sessionId)          # 豁免优先于一切
    if excuse is not null:
        if not logged(sessionId):
            record(policy=router-discipline, action=bypass,
                   reasonCode=router-excused, detail=excuse)
        return                                    # 静音，不提醒

    if event.toolName == "read":
        willRead = input.limit ?? 201             # 未指定 = 读全文，视作超限
        if willRead > 200 and not st.readWarned:
            st.readWarned = true
            nudge("主线程读大文件 → 派 explore，结论 pin_finding 落盘")
        return

    if event.toolName == "edit":
        path = input.path
        if st.editStreak 非空 且 不含 path:  st.editStreak.clear()   # 换文件即重置
        n = (st.editStreak[path] ?? 0) + 1
        st.editStreak[path] = n
        if n >= 3 and not st.editWarned:
            st.editWarned = true
            nudge("连续第 n 次 edit 同一文件 → 派 general-purpose（附成本对比）")
        record(policy=router-discipline, action=warn, reasonCode=router-edit-streak)
```

### 失败与边界分支

- **豁免文件坏掉**（JSON 解析失败/理由空）：`readExcuse` 返回 null → 照常提醒。宁可误提醒，不可误静音
- **`edit` 无 path**：直接返回，不计数
- **换文件**：清空整张 streak 表，只关心「当前这轮连续编辑哪个文件」。不做全局计数——那会把「一天内改了 10 个文件各一次」误判成调试循环
- **扩展自身异常**：整体 `try/catch` fail-open，console.warn 留痕，不影响工具执行
- **session_shutdown**：清掉该 session 的 state 与 excusedLogged，避免长跑进程内存累积
- **`bash` 通道绕过**（`cat` 读大文件、`sed -i` 改文件）：不在本扩展覆盖范围，**明确接受**。要覆盖它得解析 shell 命令，成本远高于收益，且与仓库既有的「shell 通道不在多数门禁覆盖」边界一致
