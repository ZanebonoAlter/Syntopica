<!-- complexity: complex -->
<!-- ui-impact: none -->

## Why

`constraint-injection` 扩展的设计前提已经过时。它建立在一个假设上：**模型能力不足以自行找到并阅读约束文档，所以 harness 必须把正文主动推进上下文**。这个假设在两个方向上都已被证伪：

- **注入形态**：实测 2862 次 `constraint.inject`，单次最坏 **27.8KB**（jit-path 命中整节全文）；关键词通道跨域误伤（聊到 "discovery" 即注入 8KB 整节），而"用户提到什么词"与"这个 change 需要什么约束"本就是两回事——2026-09-16 已因同类原因关闭 change 文本关键词通道。
- **注入时机**：harness 在**每回合**按关键词/编辑路径猜测"该给什么约束"，本质是用**文件系统路径**猜**业务语义**。而 openspec 侧其实**已经有**这个语义信号——change 处在制品流水线的哪一步——却从未被任何扩展消费（`openspec status --json` 的 `artifacts[].status` 是现成的机器可读事实，CLI 内部用它判阶段，但模型读不出来）。

同时，当前"档位（mode）"机制与 openspec 进度**并存两套、各说各话**：mode 由斜杠命令/skill 路径激活（`/opsx-apply` → implementation），openspec 由制品完成度判阶段，两者之间没有任何桥接。结果是：档位与制品进度脱节（读了 apply skill 但 tasks.md 尚未产出也算实现期）、子线程继承靠跨进程事实库兜底、事实库里 `mode.set` 记的是"用户敲了什么命令"而非"项目做到哪一步"。

用户决策：**参考原型驱动开发流程（`ui-design-workflow` 的"声明 → 制品 → 门禁 → 审批"四段链），在 openspec 进度中用标识的方式表达当前阶段，模型按需自主导入相关文档。** 即：harness 从"推正文"改为"报路由"，模型从"被投喂"改为"自主 read"。

## What Changes

### 1. 引入 stage 标识（替代 mode 档位）

新增 `openspec/changes/<name>/stage.md`，文件头一条机器可读 marker：

```
<!-- stage: apply -->
```

**stage 取值域（6 档单调阶梯）**：`explore` → `propose` → `design` → `implement` → `verify` → `archive`。

- marker **缺失** = legacy change = 不阻断、不改行为（对齐 `ui-stage` 的既有三态先例）。
- marker **非法/重复** = invalid，只记 warn 不阻断。
- stage 取代 mode 成为**唯一进度真相源**；`mode.set` 事实库事件退役为 `stage.set`（payload 改 `stage` 字段，读方过渡期双读）。

### 2. 注入形态：首推红线层 + 路由清单，正文改为自主 read

**废弃"整节正文主动注入"**（keyword / jit-path 两条全节通道）。新形态：

- **阶段首推**（进入某 stage 时推一次）：声明域的**红线层**（顶层列表项首个加粗句逐行，55 条 ≈ 1.8KB）+ 本阶段**相关文档清单**（文档路径 + 节名 + 取数方式），引导模型自行 `read`。
- **清单随阶段变化**：`explore/propose` 只给业务红线；`design` 加 UI 契约；`implement` 加代码与测试规范；`verify/archive` 加验收与归档规范。
- **正文永不主动推**：需要细节时模型自己 read。`constraint-digest.sh`（现成自取原语）在清单里以命令形式给出。

### 3. 读没读过：记账 + 归档对账

新增事实库事件 `constraint.read`：`read` 工具事件拿到路径 → 记录"哪篇约束文档何时被读过"。归档门禁（`spec-gate`）新增一项对账：**本 change 声明的每个业务域，其 flow 文档「业务约束与不变量」节 MUST 在本 change 的实现期内被 read 过**（未读 → 红，给出 `constraint-digest.sh` 取数指引）。

这条把"自觉"变成"可核验"：注入不再强塞，但漏读会被归档门禁拦下。

### 4. mode 机制退役

`requirements`/`implementation` 二分档删除，改为 stage 阶梯 + `planning`/`implementation` 两个**阶段族**（派生，非独立状态）：

- `planning` 族 = `explore` / `propose` / `design`
- `implementation` 族 = `implement` / `verify` / `archive`

族只服务三处纯布尔判定（entry-gate 白盒用例提醒、ui-design-gate 实现期门禁、mtime 兜底绑定），其余消费点（quality-gate / edit-map / doc-impact.sh）**本来就只用 boundChange 名**，改造成本最低。

### 5. 兼容策略

- 存量 change（无 `stage.md`）：按 legacy 处理，行为不变，**不阻断归档**。
- 存量 `mode.set` 事实库行：读方双读（先 `stage.set`，无则 `mode.set` 并映射回族），保留期 30 天自然消退。
- 跨进程子线程继承：`stage.set` 保留 `boundChange` 字段，子线程读父会话最新 `stage.set`（复用既有 `inheritFromParentHistory` 结构，只把 `mode` 换成 `stage`）。

## Capabilities

### New Capabilities

- 无。

### Modified Capabilities

- `constraint-injection`：档位机制改为 stage 标识；注入形态改为"红线层 + 路由清单"；新增读文档记账与归档对账要求。
- `harness-fact-log`：`mode.set` 退役为 `stage.set`；新增 `constraint.read` 事件类型。

## Impact

**新增文件**
- `openspec/changes/<name>/stage.md`（每个活跃 change 一份；**非 openspec schema 制品**，与 `explore-findings.md` / `human-feedback.md` 同性质）
- `.pi/extensions/lib/stage.ts`（stage 解析 / 阶段族派生 / 路由清单装配纯函数）
- `scripts/harness/constraint-read-trace.sh`（归档对账脚本）

**改造文件**
- `.pi/extensions/constraint-injection.ts`（2354 行 → 注入管线重排：删全节通道，加 stage 驱动路由）
- `.pi/extensions/lib/harness-log.ts`（事件 kind 增删）
- `.pi/extensions/entry-gate.ts` / `ui-design-gate.ts` / `quality-gate.ts` / `lib/edit-map.ts`（mode → stage 读方）
- `.pi/extensions/spec-gate.ts`（归档对账接入）
- `.pi/constraint-injection.json`（配置：stages 路由表替代 modes 二分）
- `scripts/harness/doc-impact.sh` / `harness-retro.sh`（读 `stage.set`）
- `docs/reference/harness/pi-extensions.md` / `docs/reference/constraints-index.md` / `AGENTS.md`（口径同步）

**影响面**
- **无前端/后端业务代码改动**（`ui-impact: none`），不动 `front/` 与 `backend-go/`。
- 注入体量：单次从最坏 27.8KB 降到红线层 + 清单（红线层为空/过短时回退取数指引行而非整节，正文永不主动推；声明域与路由条目断言上限 ≤4000B/条）。
- 前缀缓存：`stableSnapshotKey` 的 mode 段改 stage **族**（仍二值，避免快照频繁重建）。
- 事实库：新增 `constraint.read`（30 天保留期），`mode.set` 停写改 `stage.set`。
