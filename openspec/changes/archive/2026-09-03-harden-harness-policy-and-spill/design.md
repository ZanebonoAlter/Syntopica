## Context

见 proposal.md。现有 `events.db` 已由 `lib/harness-log.ts` 统一写入，`kind` 为 TEXT、payload 为 JSON，因此扩充事件词汇不需要 DDL。spec-gate、quota-gate、test-scope-guard 都在 `tool_call` 挂点独立裁决；它们的匹配和执行语义不同，不适合合并为一个策略引擎。tool-output-spill 在 `tool_result` 中以 `<timestamp>-<tool>.txt` 写入会话目录，已有 30 天清理和写失败原样放行语义。

## Goals / Non-Goals

**Goals:**

- 在不改变各 gate 裁决逻辑的前提下，为显著策略结果提供统一、低噪声、可 SQL 聚合的事实。
- 用共享边界收敛 payload，避免各扩展各自记录自由文本。
- 消除 spill 并行文件名覆盖窗口，并在 POSIX 平台落实最小权限。
- 保持事件库、旧 spill 文件和现有查询向后兼容。

**Non-Goals:**

- 不建立集中式 Policy Engine，不迁移各 gate 的匹配规则。
- 不记录普通成功放行，不替代既有 `gate.check`。
- 不新增定时 Health Review、父子 Trace、spill 大小上限或旧文件批量迁移。

## Decisions

### D1：共享记账契约，不共享裁决引擎

新增轻量 `lib/policy-decision.ts`，只负责类型约束、字段收敛和调用 `logEvent(kind="policy.decision")`；各扩展仍在自己的挂点、原有控制流中决定何时调用。helper 接收 cwd、sessionId、change 和结构化 decision，不解析命令、不查额度，也不返回可影响放行/阻断的结果。

选择该方案而不是集中式策略表，是因为三个 gate 的输入、失败策略和副作用完全不同。共享执行引擎会把远端额度查询、shell 检查和软提醒耦合在一起，形成新的单点故障。

### D2：只记录用户可感知或降级型结果

固定 action 为 `block | warn | bypass | fail-open`，reasonCode 由各 producer 的常量表提供：

- spec-gate：归档检查失败、显式 bypass、非阻断措辞 warning；
- quota-gate：额度不足阻断、查询异常 fail-open、既有一次性风险 warning；
- test-scope-guard：soft 模式 warning、hard 模式 block。

一次工具调用若同时产生不同的用户可见结果，可以追加多条不同 action 的事件；普通通过、未命中和 quota 健康放行不写事件。`target` 只允许 change/provider 等短摘要，禁止复制完整命令、密钥、远端响应或长错误文本。change 归属优先使用 spec-gate 已解析的归档 change；其余场景复用 `lib/active-change.ts` 的检测结果，无法确认则为 null。

选择“显著结果才记”而不是为 allow 建分母，是因为 Health Review 关注策略干预与降级频率；全量 allow 会重复 `gate.check` 曾出现的事件写放大。未来若需命中率分母，应单独采样而不是改变本事件语义。

### D3：策略记账严格旁路化

`policy.decision` 写入失败只走 `harness-log` 既有 fail-loud/fail-safe 路径，调用方忽略返回值并继续原裁决。不得把“记账成功”放在返回 block、发送 warning 或 fail-open 的前置条件上。

事件词汇和 30 天 TTL 直接扩充 `HarnessEventKind` 与保留期映射；events 表结构和 `user_version` 不变。这样旧库可直接接收新 kind，旧查询也会自然忽略未知 kind。

### D4：spill 文件名使用调用标识哈希

用 Node 标准库 SHA-256 对 `toolCallId` 求哈希并截取固定前缀，文件名采用：

```text
<timestamp>-<safeTool>-<callHash>.txt
```

不使用原始 toolCallId，避免文件名泄露内部标识；不引入第三方依赖。写文件使用排他创建，意外同名时不得覆盖已有归档，而是进入既有 spill 失败降级。

### D5：POSIX 权限收敛，Windows best-effort

创建会话目录时指定 `0700`；目录已存在时，在非 Windows 平台显式 chmod 到 `0700`。新文件以 `0600` 排他创建，并在非 Windows 平台校验/收敛权限。若 POSIX 权限步骤失败，尽力删除本次新文件后抛入既有 catch：记录失败 `spill.write`，原始工具结果进入上下文。

Windows 不承诺 POSIX mode 语义，因此跳过严格 chmod 校验，仅保留创建时 mode 参数；平台不支持权限位本身不算 spill 失败。旧文件不遍历、不 chmod，仍由既有 30 天清理处理。

### D6：测试分层

- harness-log smoke：验证 `policy.decision` 可写、可查、30 天 TTL 生效。
- policy helper/三个 gate smoke：验证各 action、稳定 reasonCode、安全 target、正常 allow 零记录和 DB 故障不改裁决。
- spill smoke：固定同一时间戳、使用两个 toolCallId，验证路径不同、内容不覆盖、文件名不含原始 ID；POSIX 断言目录/文件 mode，Windows 显式跳过 mode 断言。
- 继续运行现有 `.pi/extensions/tests/run-smoke.sh` 与 `run-harness-smoke.sh`，防止扩展装载和既有行为回归。

## Risks / Trade-offs

- [新增事件造成写放大] → 只记录 block/warn/bypass/fail-open，普通放行零记录，TTL 固定 30 天。
- [各 gate reasonCode 漂移] → 在共享类型旁维护稳定枚举，smoke 对 producer 输出做精确断言，文档列为查询契约。
- [记账 I/O 影响门禁] → helper 保持同步小 payload、仅显著结果触发；失败旁路，不改变裁决。
- [短哈希理论碰撞] → 使用足够长度的 SHA-256 前缀并排他创建；碰撞时安全失败而不是覆盖。
- [Windows 权限语义不一致] → 平台分支明确 best-effort，严格权限验收只在 POSIX 执行。
- [POSIX chmod 失败后残留文件] → catch 前尽力 unlink；即使清理失败也不把不安全路径暴露为成功 spill，后续 TTL 清理兜底。

## Migration Plan

1. 扩充事件 kind/TTL 与共享 policy helper，不升级 SQLite schema。
2. 逐个接入三个 gate，并用 smoke 锁定原裁决返回值不变。
3. 加固 spill 命名和权限，验证既有取回路径、记账和清理行为不变。
4. 更新 harness 事实查询文档和扩展全景；加载新扩展代码需在本地 Pi 会话执行 `/reload` 或重启会话。

回滚时恢复扩展代码即可；数据库中已写入的 `policy.decision` 是兼容的 TEXT kind 行，可等待 TTL 清理。新命名的 spill 文件仍是普通文本文件，旧代码的 30 天目录扫描可继续清理。
