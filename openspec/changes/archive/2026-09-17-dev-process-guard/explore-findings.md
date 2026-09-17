
## 归档门禁脚本机器格式约定（spec-gate 四检查实操）

归档门禁三个脚本的机器格式约定（2026-09-17 归档 fix-spa-nav-loading-ux 实测踩坑）：

1. **doc-impact.sh verify 的声明解析**：`grep -m1 '<!-- doc-impact:'` 取全文第一个匹配——tasks.md 的「## N. 文档」节**之前**（含任务描述正文）不能出现 `<!-- doc-impact:` 字样，否则任务行里的模板字样被当声明，域解析成垃圾导致 flow/standard 全报「未声明」。域列表只能是固定 8 域名（flow/api/database/architecture/standard/configuration/deployment/none），不认 `standard/frontend` 这类子路径。启发式：`front/app/features/**` 改动命中 flow 域、`docs/reference/standard/**` 命中 standard 域；归档轨用事实库 edit.map 归属集合（命中即 FAIL），声明域补齐或 doc-impact-excuse 二选一。

2. **scenario-trace.sh 映射表**：要求「## <数字>. 验证」节（正则 `^## [0-9]+\. *验证`）内表头恰为 `| Scenario | 测试文件 |` 的表；映射行第一列须与 delta spec 的 Scenario 标题**逐字相等**（不能带 `capability: ` 前缀）；单元格要么「人工」开头（整格放行为人工留痕），要么纯路径——空格/逗号分隔多路径逐一校验存在性，`path + 人工（x）` 这种混合写法会因 `+`/括号 token 不是路径而 FAIL。MODIFIED requirement 保留的既有 Scenario 也在对账范围。

3. **tasks.md 尾三节**（§11.2）：`## N. 测试` / `## N+1. 文档`（首行 doc-impact 注释 + checkbox 列文件）/ `## N+2. 验证`（映射表 + 「可执行命令 → 结果」），三节标题各自独立正则匹配，缺一不可。

4. **归档幂等验证**：手动同步 delta 进主 spec 后跑 `openspec archive <name> --yes --json`，返回 `specsUpdated: false, totals 全 0` 即为手动同步与 CLI 合并语义一致的证明。

5. **归档后 check-standards --change 会报「目标 change 不存在」**（对账对象已移入 archive），归档后改跑全仓巡检；E 段对归档未满 3 天的 change 免检（溯源行已补则无虞）。

<!-- pinned 2026-09-17T14:23:38Z -->

## dev-process-guard 实现要点：btime 截断与 now 亚秒精度

B-12 真实进程版调试发现两个 /proc 时钟陷阱，已落地为实现决策：
1. /proc/stat 的 btime 是整数秒（向下截断），starttime ticks 精确 → computeStarttimeSec 结果比真实启动时刻低估 0~1s。后果：会话开始后 ~1s 内起的进程可能被算到窗口外（只漏杀不误杀，保守方向，符合红线）；冒烟 B-12 真实版在 session_start 与 spawn 间垫 1.3s 保证确定性。
2. deps.now 默认必须用亚秒精度（Date.now()/1000），不能 floor 到整秒：floor 会让闭区间右端 [start, now] 的 now 小于同秒内新起进程的带小数 starttime（实测 B-12 真实版被此卡掉 kills=[]）。桩世界注入整数不受影响。
另：冒烟 run-harness-smoke.sh 工作树 diff 里 session-rollup 行为并行子线程增量（基线 HEAD 无），非本线程改动。B-12 真实版经 setsid bash <tmp>/.cache/go-build/smoke/main（脚本内 trap "" TERM）验证：cwd 用临时目录做 repoRoot，只让测试进程命中五条件，不误伤真实仓库服务；TERM 被忽略 → ~2s 轮询 → KILL 清组，事件落 tmp 下 events.db（rmSync 清理）。

<!-- pinned 2026-09-17T14:58:06Z -->
