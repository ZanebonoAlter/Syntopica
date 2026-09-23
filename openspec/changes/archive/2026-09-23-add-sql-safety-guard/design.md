## Context

见 proposal.md Why。与设计相关的现状：

- `.pi/extensions/test-scope-guard.ts` 已趟出完整模式：`pi.on("tool_call")` 前置拦截、三通道取命令文本（bash / ctx_execute shell / ctx_batch_execute）、返回 `{ block, reason }`、soft-hard-off 配置、逃生注释、`lib/policy-decision` 记账、`[前缀]` 防自噬。
- 事故根因是「验证语法」与「真执行」混淆：`psql -c` 无 dry-run。拦截面就在 `tool_call`——AI 拿 shell 当手的必经之路。
- 破坏性 SQL 的真实分布：日常合法操作大量含 `DELETE`（db-cleanup、数据修复），而 `TRUNCATE`/`DROP` 在本仓库只出现在清场/建弃场景——用户拍板只拦这两个词，把误伤面压到最小。

## Goals / Non-Goals

**Goals:**
- 事故金样例（`psql -c "DO $$ TRUNCATE…$$"`）在 hard 模式下 100% 被拦，且 smoke 固化为回归。
- 误伤面收敛：只读 psql、文档 grep、含 DELETE 的清理脚本零感知。
- 逃生口明确可打：`# allow-truncate-drop` 后缀注释 / `BEGIN…ROLLBACK` / `SQL_SAFETY_GUARD=off`。

**Non-Goals:**
- 不拦 `DELETE`/`UPDATE`/`ALTER`/`INSERT`（有意的范围收窄，用户拍板）。
- 不覆盖用户终端手敲、`pi.exec` 内部调用、mysql/sqlite（边界如实声明，见 proposal Impact）。
- 不做 PATH 级 `psql` 包装（更强但侵入用户全局环境，暂缓）。
- 不做近期会话语境豁免（自噬回路风险，spec 已 SHALL NOT）。

## Decisions

**D1：判定基于命令原文，不做引号/注释掩蔽。**
与 test-scope-guard 相反（它掩蔽引号防误报），本守卫要拦的 SQL 正文**就在引号里**（`psql -c "…TRUNCATE…"`）——掩蔽会把目标掩掉。代价是极端假阳性（如 `echo 'TRUNCATE' && psql -c 'select 1'` 会被拦），但方向正确（宁拦勿漏），且逃生注释一行解决。
备选（否决）：沿用 maskShellText 管线——直接漏掉事故金样例本身，不可用。

**D2：判定顺序 = 双键粗筛 → 放行条件 → `-f` 文件扫描 → block。**
1. 原文 `/psql/i` ∧ `/\b(TRUNCATE|DROP)\b/i` 双命中才进下一步（零开销：绝大多数命令第一键或第二键直接出局）；
2. 逃生注释（原文含 `# allow-truncate-drop`）→ 放行；
3. `BEGIN`+`ROLLBACK` 同现 / 一次性容器标志 → 放行；
4. `psql -f` 形态 → 读文件扫两词（读不到 → 保守拦）；非 `-f` 形态的双键命中已足够，直接拦；
5. hard=block + reason（三条出路）+ `policy.decision(block)`；soft=notify + `policy.decision(warn)`；off=粗筛前 return。
嵌套 `bash -c` 不需递归抽取：payload 本就在命令原文里，双键粗筛天然覆盖（spec 场景已固化）。

**D3：逃生口只认命令原文注释，不认语境。**
test-scope-guard 扫近 15 条会话找「归档」语境——那适用于「偶发合法」场景；数据安全规则若认语境，我先说一句「在做清理」就能自我放行，规则即失效。只认命令文本里的硬证据。

**D4：`-f` 文件读取用异步 fs（hook 内允许 IO，test-scope-guard 已有 `resolveRepoRoot` 先例）。**
相对路径按 `ctx.cwd` 解析；解析失败/文件不存在/无读权限 → 保守 block（无法证明无害即拦），reason 注明是「文件无法验证」型拦截。

**D5：默认 hard、逃生口齐全。**
用户明确「软约束没有用」。误伤代价 = 多打一个尾注释；漏拦代价 = 清库。soft 仅作应急降级（`SQL_SAFETY_GUARD=soft`）。

**D6：范围 = TRUNCATE/DROP 两词（用户拍板）。**
`DROP` 不限定宾语（TABLE/DB/SCHEMA/INDEX/DATABASE/VIEW 通吃，正则不枚举）——反正入口键已限定 psql。`/i` 大小写不敏感（`truncate` 小写也拦）。

## Risks / Trade-offs

- [假阳性：只读命令同文本携带两词] → 双键要求 psql 入口已滤掉绝大多数；残余走 `# allow-truncate-drop` 一行逃生，成本极低。
- [假阴性：绕过 psql token 的形态（如 `docker exec … sh -c 'echo DROP|psql'`）] → 命令原文仍含 `psql` token（第二键 TRUNCATE/DROP 也在原文）→ 双键仍命中；真正绕过的是不带 psql 字样的通道（psql URI、应用内 SQL）——如实列为边界，不追求完备。
- [`-f` 文件在判定后、执行前被并发修改（TOCTOU）] → 单用户单会话场景窗口极小；接受（与 test-scope-guard 同级的时序假设）。
- [逃生注释被滥用成常驻习惯] → 记 info 落 events.db，可经 harness-retro 查「逃生口命中率」回检（回检指标：逃生使用均须伴随任务语境，纯例行逃生 → 收紧讨论）。
- [与并发会话共享工作树] → 扩展是项目级加载，所有本仓会话同时受保护（这正是要的效果）；events.db 记账带 session 维度可归因。

## Migration Plan

1. 新增扩展文件 + smoke，不改任何既有扩展（test-scope-guard 仅作范本读取，不复制其会话语境逻辑）。
2. 扩展按 `.pi/extensions/*.ts` 目录约定加载——落地后**新会话生效**（当前已开会话需重载才挂上，属 harness 已知行为）。
3. 回滚 = 删除 `sql-safety-guard.ts`（独立文件，无状态迁移）。

## Open Questions

（无——拦截范围、逃生方式、默认模式均已由用户拍板并写入 spec。）
