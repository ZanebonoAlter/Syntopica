# 设计 — UI stage 记账

## 1. 问题定位

`ui-approval` 是有效的，因为它守住了**唯一会破坏产品代码的入口**（`mode=implementation` 的写入与实现派发）。它的触发条件是 mode，这与它的守卫对象一致。

L1–L3 要守的对象不是「实现」，而是「原型的生产过程」——这件事发生在 `requirements` 档，甚至无档位。沿用 mode 触发，等于把门装在没有走廊的那一侧。实测 34 次派发 100% 落在门禁视野外，就是这个结构问题的量化结果。

## 2. 决策

### D1 — 触发条件用路径，不用 mode

**决定**：`edit` / `write` 的目标路径落在 `openspec/changes/<name>/` 下即触发 stage 检查，change 身份由路径解析（`changeNameFromPath`）。

**理由**：路径是文件系统事实，零歧义、零绑定依赖；而 `mode.set` 的 `boundChange` 在无档位时根本不存在。这是绕开第 132 行盲区的唯一无损方式。

**备选（否决）**：把 `ui-design-gate` 的 mode 判断放宽到 `requirements`。否决理由：会让 `ui-approval` 的 block 在 requirements 档也生效，存量 major pending change 会立刻无法修改原型——误伤面远大于收益。

### D2 — stage 是单调阶梯，不是开关

**决定**：`l0 → skeleton → interaction → data → approved` 五个取值。`approved` 等价「全放行」。

**理由**：`ui-approval` 是二元的（pending/approved），因为审批只有「过/不过」。而 L1–L3 是生产过程，需要表达「现在生产到哪一步、这一步允许写什么」。二元开关表达不了「在做骨架时不该出现业务文案」。

### D3 — 越域判定按「原型目录门禁」，不做 change 根全路径白名单

**决定**（与 `checkStageScope` 实现、delta spec 写域表三处保持一致）：

| stage | 允许写入（相对 change 根） |
|---|---|
| `l0` | change 根下任意路径，**除 `ui-prototype/`** |
| `skeleton` | 同上，另放行 `ui-prototype/skeleton/` |
| `interaction` | 全部 |
| `data` | 全部（与 `interaction` 同写域） |
| `approved` | 全部，且不记账 |

**理由**：change 根下还有 `human-feedback.md` / `explore-findings.md` / `test-cases.md` / `review.md` / `ui-verification/` 等随时会被写的规划制品。若按「白名单穷举」，每新增一类制品就得补一条，漏了就是误报；而误报会直接让观察期数据（D5 的前置条件）失真。因此只对**真正要设防的那个目录**设防。

**代价（明说）**：`data` 档在实现里与 `interaction` 同写域，因此**永远产不出** `ui-stage-scope-out`；`data` 目前只承担「记账意图」而无写域约束。这是刻意取舍——把 `data` 收紧成「只许写 fixture.json」会让填数阶段连原型里一个错字都改不了，误报率高于收益。若后续需要真约束，应在 B 阶段单独设计。

### D4 — 本次只 warn，不 block

**决定**：缺失 marker、越域写入、非法取值，一律记 `policy.decision` 的 `action=warn`，不中断会话。同一 session × change × reasonCode 只记账一次（复用既有 `warn-legacy` 的去重口径）。

**理由**：新触发条件会扫到存量 22 个 change 的 `ui-prototype/` 目录。在证明「不漏拦、不误拦」之前上 block，等于拿整个仓库做赌注。

**新增三个 warn 侧 reasonCode**（不进 `UI_REASON_CODES` 的九个阻断码白名单，对齐 `UI_PASS_REASON_CODES` 的既有先例）：

| reasonCode | 触发 |
|---|---|
| `ui-stage-missing` | `ui-design.md` 无 `ui-stage` marker（legacy） |
| `ui-stage-invalid` | marker 存在但取值不在五档内、或出现多次 |
| `ui-stage-scope-out` | 写入目标越出当前 stage 的允许前缀 |

### D5 — 分阶段口径（B 阶段的前置条件）

观察期结束转 block 时，需要同时满足三条，任一条不满足都不许上锁：

1. `ui-stage-missing` 占比 < 30%（说明 marker 真的在被写）；
2. `ui-stage-scope-out` 的误报率经人工抽查 < 5%；
3. `UI_REASON_CODES` 白名单扩容（`ui-stage-scope-out` 升为阻断码）与主 spec「九个稳定有界值」的措辞同步改。

### D6 — 三条防假阳性的硬约束（review 后补）

| # | 约束 | 原因 |
|---|---|---|
| 1 | 目标为 `ui-design.md` 的写入**不记账**（实际实现：只对 `ui-prototype/` 内写入记账，其余路径直接返回） | marker 在写前读取；「补标记」这个动作本身读到的是旧内容 → 必然假阳性 `ui-stage-missing` |
| 2 | change 目录必须真实存在才记账 | 纯路径正则会把仓库外路径（`/tmp/x/openspec/changes/ghost/…`）解析成不存在的 change |
| 3 | 去重键 = `change\|reasonCode`，**不含 sessionId** | 子线程与主线程共用模块实例；带 session 会让一次派发乘出 N 条，「低噪声」目标在主场景反而落空 |
| 4 | 三码必须在消费侧登记为情报型 | `harness-retro.sh` 的 `INFORMATIVE_REASONS` 未登记 → 默认按纠正型统计，累计 ≥5 就进「软提醒失效」；两处 `action IN ('block','warn')` 会把 stage warn 当催修负担算时距 |

## 4. 风险

| 风险 | 处理 |
|---|---|
| 新触发条件让每次原型写入都产生一条 warn，事实库噪声变大 | 每 session × change × reasonCode 去重一次；且只对落在 `ui-prototype/` 或 `ui-design.md` 的写入触发，规划别的文件不触发 |
| stage 被随手写 `approved` 绕过全部限制 | 本次不改裁决，所以无绕过后果；转 block 时须加「跳级检测」（`l0` 直接写 `approved` 记 `ui-stage-skipped`），已在 D5 前置条件里列明 |
| `ui-design.md` 不存在时无法读 marker | 读不到 = 无 marker = `ui-stage-missing`，与 legacy 同路 |
| 路径解析在 Windows 分隔符下失效 | `changeNameFromPath` 归一化 `\` 与 `/` 后再匹配，测试覆盖两种分隔符 |
| 与 `ui-approval` 的 block 裁决互相干扰 | 新逻辑插在第 132 行 **之前**，且只记 warn、直接 return，不修改 `decideUiToolGate` 的任何返回值 |
