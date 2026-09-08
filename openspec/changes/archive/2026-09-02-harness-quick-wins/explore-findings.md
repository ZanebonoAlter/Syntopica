
## 粘性失败真实形态与门禁优化依据

粘性失败真实形态（2026-09-01 事实库考古，session 01a0399b 08-26 实测）：
- 失败主流形态 = 大改动中间态的自然红：diag 逐次演变（typechecking error → isUniqueViolation → gofmt → unused），连续失败链 2-5 次即转绿，"平均 6.07 次失败/(会话×命令)"是全天多波次累计而非复读机。3 连败退避方案不采纳。
- 同根因短路的依据：build failed（编译不过）期间 golangci-lint / go vet / go test 三条全红同根因，每回合仍全套跑（≈7s 命令时间 + 3 条事件 + 3 条 steer）。lint 报 "[linters_context] typechecking error" 即编译失败的可靠信号，此时 vet/test 必红，可跳过（未执行不记账，符合 harness-fact-log spec"每条实际执行的门禁命令各记一条"与"未触发不记录"既有语义，无需改 spec 的记账条款）。
- steer 分级的依据：TestRandomHex 在三个波次各红一次（疑似修A破B回归），但绝大多数失败是"从未绿过"的新代码中间态；对中间态用强催修语气干扰 TDD 节奏。分级信号：本会话该命令是否有过 ok=1 记录（可查事实库或会话内状态）。
- 10 天门禁账目：pnpm lint 1455 次 × 22.3s ≈ 9h（占 65%），eslint 无 --cache（front/package.json "lint": "eslint ."）；gate.check 14,437 条占事件总量 81%（成功记账写放大，spec 明文"ok=true MUST 同样记账（失败率统计需要分母）"→ 采样需改该 requirement，分母用采样率还原）。

<!-- pinned 2026-09-01T14:05:02Z -->
