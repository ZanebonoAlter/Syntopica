# Design — harness-retro-sql-fixes

## Context

凌晨复盘（`docs/research/harness-effectiveness-metrics/retro-analysis-2026-09-18.md`）实锤三处报告口径 bug 与一处契约稀释，本 change 全部为纠错，不新增能力。关键事实：

- `inj_dom` CTE 的域名提取 `substr(..., length(path)-instr(path,'flow/')-5-3)` 多减 1（`instr` 返回 `'flow/'` 起始位，其前还有 `instr+4-1` 个字符），产出 `readin`/`ai-summar` 等截断域名，与 dom_map 完整域名永不相等 → A2 恒 0%；
- 通道枚举 `reason IN ('declaration','keyword','edit')` 中 `edit` 全库 0 条，实际 JIT 通道自 2026-08-23 起叫 `jit-path`（7 天窗口 501 条）；
- ①段家族分类的兜底模式 `%lint%` 把 `golangci-lint: command not found`（315 条）吸入「lint 规则」族；
- dev-process-guard 直写 `policy.decision` 用 `decision` 键（design D6 注释为有意权衡），但 harness-fact-log 契约未登记此豁免 → 词汇表与实现不一致。

## Goals / Non-Goals

**Goals**
- A2 命中率与①段家族归并恢复可信（可由 fixture 与真实库复算验证）
- 账本词汇表契约与实现重新一致（明豁免替代暗豁免）
- 观察项立账，防疑点丢失

**Non-Goals**
- 不改注入通道埋点本身（`jit-path` 埋点是正确行为，错在报告侧枚举）
- 不做 sqlite 容量治理（见下方备忘）
- 不动 harness-telemetry 的 rollup `turns`/`final` 逻辑（仅观察）
- 不补 subagent cost 埋点（前置条件「确有解读子线程成本占比的需求」未满足）

## Decisions

**D1 域名提取修正：`-5-3` → `-4-3`**
公式推导：`instr(path,'flow/')` 返回 `'flow/'` 首字符位置 i，`'flow/'` 长 5，故域名段（`docs/reference/flow/<domain>.md` 的 `<domain>`）起点为 `i+5`；终点为 `length(path)-3`（去掉 `.md`）。域名段长度 = `(length(path)-3) - (i+5) + 1 = length(path) - i - 7`，即 substr 长度参数应为 `length(path)-instr(path,'flow/')-4-3`——现式 `-5-3` 多扣 1，产出 `readin`/`ai-summar` 等截断域名、与 dom_map 完整域名永不相等。以真实库复算（`hit=18/edited=30`，2026-09-18 当日复算 19/33=58% 同量级）与 fixture 断言双重验证。
*备选*：改用 `replace(path,'docs/reference/flow/','')` 再去后缀——更可读但改动面大，且 dom_map 提取端也要同步，纠错 change 不值得；留作未来重构项。

**D2 通道枚举对齐账本实况：补 `jit-path`，并加「未识别通道显式计数」兜底**
枚举改为 `reason IN ('declaration','keyword','jit-path')`。同时在 A2 段输出 `未识别通道 N 条`——差集**限定在 flow 文档注入范围内**（`path LIKE '%/flow/%.md'` 的 `constraint.inject` 的 reason 全集减去匹配枚举），不按全库 reason 全集算：全库另有 `mode-base`/`change-file`/`index` 三个合法通道但从不注入 flow 文档（2026-09-18 实测三者 flow-doc 命中均为 0，合计非 flow-doc 注入 ~330 条/7天），按全量算会把它们误标「未识别」、制造新假信号；限定后当前实测未识别恰为 0 条（flow-doc reason 全集 = 枚举三者）。满足 delta spec 的「不产生假零」Scenario——未来 flow 文档注入通道再改名时报错性可见而非静默归零。
*备选*：枚举改为「白名单取反」——无法穷举且易误吸非注入类 reason，放弃。

**D3 环境噪声前置归并：`%command not found%` 判定提到关键词族之前**
在家族分类 CASE 的最前加一档「命令/解释器缺失 → 并发/环境冲突」。规则次序即语义：进程级失败（命令不存在）优先于命令内容归因。真实库预期：lint 族 423→~110 量级，环境族占比升到 ~60%（有风暴窗口时）。
*备选*：只把 `golangci-lint` 加黑名单——反 overfit（黑名单对下一个命令失效），放弃；通用 diag 模式对任何 `command not found` 成立。

**D4 dev-process-guard 契约豁免：显式收编而非改写形状**
保持 `decision` 键旁路形状不变，在 harness-fact-log 契约中登记为唯一显式豁免 + skill 文档同步。豁免形状按实现现状写全：payload 含 `decision`（`orphan-killed | orphan-warn`）与进程摘要（`cmd`/`offenders`/`inWindow` 等有界字段），不含 `action`，也不要求 `policy`/`reasonCode`（实测 5 条均无此二键——豁免的是整个键形状）。理由：
1. 原 design D6 的误吸担忧经核实成立——若改成 `action=block/warn`：`orphan-killed`(block) 无对应 `(session,cmd)` 转绿锚点，会污染催修时距中位数（永不闭合的 open block）；`orphan-warn`(warn) 会进④段软提醒聚类，与「提醒被无视」语义勉强相容但会混入非用户操作事件；
2. `action` 白名单语义是「对用户操作的裁决」，杀进程组不是用户操作，硬塞语义歪；
3. 契约稀释的病根是「暗豁免」（实现了但契约没写），明豁免（写进 spec + 词汇表）即治愈，零行为回归。
*备选*：改写为契约内形状（retro 报告改进项 #4 的第一选项）——需要重评③④段过滤 + 改扩展 + 重跑 fixture，收益仅是形状统一，否决。

**D5 观察项登记方式：写进 retro 报告的改进项台账，不建独立机制**
rollup `turns`/`final` 疑点、subagent cost 缺口按报告 #5/#7 的指标与窗口登记（turns 观察至 ~10-01，cost 挂起待需求）。载体用现有 `docs/research/harness-effectiveness-metrics/retro-analysis-2026-09-18.md` 的改进项清单 + tasks 验证节引用，不新建台账文件。

## sqlite 容量备忘（2026-09-18 讨论结论，免重复讨论）

events.db 现状 12.3MB（payload 实际 ~4MB，其余为空闲页碎片）+ WAL 4.2MB；31,180 行/26 天（平日 200-900 行，峰值 4,137）。三层防线已内置：TTL 按 kind 分级清扫（热事件 30 天 / rollup·session.start 90 天 / pin.write 永久，挂开库路径增量执行）、100MB 保险丝（超限删最老一半）、WAL 单写者。稳态估算 ~15-20MB ≪ 保险丝，性能在 <10 万行量级为毫秒级。**结论：无需任何动作**。未来触发点：①真到几十 MB 再开 `PRAGMA auto_vacuum=INCREMENTAL`；②需季度级效能对比时做按月冷归档（csv/parquet）而非拉长 TTL。

## Risks / Trade-offs

- [修复后 A2 命中率口径变化导致基线不可比] → 旧基线 `metrics7` 键值标注口径版本，`--baseline` 比对时对 A2 输出「口径已修正」提示；本次修复本身即用 `--save-baseline` → 修 → `--baseline` 流程验收
- [`command not found` 前置规则吞掉真实 lint 配置错误] → 规则只匹配 diag 含 `command not found` 字面（命令不存在），lint 规则错误（编译/配置类 diag）不受影响；smoke fixture 断言两类并存时各归各族
- [豁免条款被未来新扩展效仿滥用] → 契约文字锁定「唯一显式豁免」，新增旁路形状必须先改本契约（走 change）
- [通道枚举再次漂移] → D2 的「未识别通道显式计数」兜底使漂移可见；skill 文档同步登记枚举清单

## Migration Plan

纯工具链修复，无部署依赖：改 `scripts/harness-retro.sh` + smoke fixture → 跑 smoke → 真实库 `--days 7` 复算 A2 与①段 → `--save-baseline` 留新基线。dev-process-guard 改动随扩展文件即时生效（下次会话加载）。回滚 = git revert 单提交，账本数据不受影响（报告只读）。

## Open Questions

（无——观察项已立账，无需本 change 期间决策）
