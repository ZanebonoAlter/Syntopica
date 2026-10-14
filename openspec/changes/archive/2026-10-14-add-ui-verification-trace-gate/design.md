## Context

spec-gate 检查⑥' 现状：`missingArchiveUiEvidence()`（`.pi/extensions/lib/ui-design-gate.ts`）对 major 档做五项检查——ui-approval marker、原型文件存在（唯一用到 `changeDirFiles` 递归清单的地方）、tasks 尾节含 `opencli` 字符串、含 `1440`+`1920` 字符串、ui-design.md 含「差异」二字；minor 档为验收节四关键词正则匹配。判定逻辑内嵌扩展 lib，无独立脚本，archive-readiness 聚合自查不含 UI 验收项（「UI 验收证据项不在自查范围（仅门禁校验）」）。

对照 review-trace 机制：独立 bash 脚本（正则全解析、只读、退出码 0/1）+ spec-gate 调用共用单一事实源 + 三道防线（建档注入 → apply 入口 → 归档兜底）。本设计将该模式同构复制到 UI 验收，强度取轻量版（用户决策）。

实战基准：`add-topic-key-event-markers/ui-acceptance/report.md`（agent-browser 无头验收，A1–A11 逐条判定 + 时间线 + 18 张截图 + 终态对账，抓到 P1 死按钮）——格式契约从它归纳，不自创结构。

## Goals / Non-Goals

**Goals:**
- 归档门禁的 UI 验收判定从「关键词字样」升级为「实物对账」，判定逻辑单一事实源在独立脚本
- major/minor 分档判定规则机器可执行、可冒烟测试
- 证据目录名、验证工具口径（agent-browser）、文档引用三处分裂一次性收敛

**Non-Goals:**
- 不做双视口文件名锚点（1440/1920 出现在截图文件名）等强档检查——轻量版先行，验证机制存活率后再加码
- 不改 ui-design-gate 前置门禁（原型审批流程）与 quality-gate/constraint-injection
- 不要求 minor 档建 `ui-verification/` 目录（组件测试路径即实物；仅走 agent-browser/人工路径才需 report.md）
- 不追溯改造旧 schema（spec-driven）change——fail-open 兼容
- 不引入视觉回归基线/像素 diff 等重型视觉验证设施

## Decisions

### D1: 独立脚本而非强化 lib 纯函数（用户决策：防过度耦合）

`scripts/harness/ui-verification-trace.sh` 仿 `review-trace.sh`：POSIX bash、只读、中文修复指引、退出码 0/1/用法错误。spec-gate 检查⑥' 改为调用脚本（spawn bash 取退出码与输出），`missingArchiveUiEvidence` 中 tasks 字样匹配与 minor 关键词匹配两段判定删除；ui-approval/原型存在两项保留在扩展内（属规划制品完整性，与验收实物对账是两类检查）。
- 备选：判定逻辑全部留在 lib 纯函数并加强——被否：archive-readiness 无法单独复用、脚本层无独立冒烟面、与 review-trace 的「单一事实源」先例相悖。

### D2: 轻量版强度边界（用户决策：先轻量）

major 通过条件 = `ui-verification/` 目录存在 + `report.md` ≥1 条合法判定条目 + 条目点名截图真实存在 + 「未覆盖」条目带原因。**不检查**：截图双视口锚点、截图最低数量、条目覆盖 ui-design.md 主故事全集（防填表游戏反向演成「为过检而凑条目」，强度递增留观察期）。

### D3: minor 指名道姓（用户决策：选型 B）

验收节映射必须挂实物：组件测试 → 测试文件路径存在（路径解析为仓库根相对）；agent-browser/人工 → `ui-verification/report.md` ≥1 条合法条目。opencli 字样从词表退役（工具口径统一 agent-browser），历史 change 写了 opencli 的按人工验证同通道处理（有建账即过，不强制改写历史文本）。

### D4: report.md 格式契约自实战形态归纳

条目 = `### A-<N> <场景标题>【pass|fail|未覆盖】` + 紧随依据行；未覆盖依据行含原因；截图文件名在条目/清单中出现即入实物对账。以 add-topic-key-event-markers 的 report.md 为冒烟 fixture 之一（解析现有文本零改造通过 = 契约归纳正确的验收信号）。与 review.md R-N 契约同构但编号 A- 前缀区分域。

### D5: 三道防线落点

1. **建档注入**：`check-tasks-tail.sh` 增规则——`ui-impact: major` 的 change，tasks 验证节（或 4.x 人工任务行）MUST 含指向 `ui-verification/report.md` 的任务行（类比 complex 档 Review 节前移）；缺行 apply 入口补上再实现。
2. **apply 入口**：openspec-apply-change skill 第一步已跑 check-tasks-tail，规则合并即自动覆盖，无需新入口。
3. **归档兜底**：spec-gate 检查⑥' 调脚本结果；archive-readiness 聚合新增本脚本（消除「UI 验收证据不在自查范围」盲区）。

### D6: 工具口径迁移 opencli → agent-browser

全仓收敛点：开发执行规范 12 处、test-design.md 主体+JIT 摘要 3 处、layout.md 验收方式引用、spec-gate ⑥' `/opencli/i` 检查（随 D1 删除）、ui-design.md 模板 Acceptance 节注释、openspec skill 内 opencli 字样、悬空 skill 引用（`ui-verify`/`opencli-browser` → 指向存在的 agent-browser skill 或改措辞）。迁移后 grep `opencli` 仅允许出现在历史档案（archive/、experience/）与本 change 自身的迁移说明里。

### D7: 存量迁移与兼容

- `add-topic-key-event-markers/ui-acceptance/` → `git mv` 为 `ui-verification/`，tasks.md 4.4 行与 report.md 内部引用同步；该 change 仍 active（4.5 未完成），迁移后其归档即按新门禁走通（作为首个实战验证）。
- 旧 schema change：脚本读 `.openspec.yaml` schema 行，非 `syntopica-ui` 直接退出码 0（与 ⑥' 现有 fail-open 语义对齐）。

### D8: ⓪ 继承与调整（改契约义务）

`test-assets.sh ui-design-workflow` 反查存量资产；spec-gate/lib 的既有冒烟断言（字样匹配放行类用例）逐行处置为「继承改写」（断言新契约：同 fixture 走脚本判定）。test-cases.md 填表，不静默删旧测试。

## Risks / Trade-offs

- [填表游戏：report.md 沦为过检仪式] → 轻量版强度克制（不凑条目数、不锚视口）+ 契约自实战形态归纳 + retro 观察 waived/空壳占比（复用 review-trace 的 retro 统计口径）
- [spec-gate spawn bash 的性能/失败面] → 脚本只读且秒级；spawn 失败按既有 fail-open 语义（记 warning 不误伤），冒烟覆盖
- [在飞 change 迁移破坏 4.5 后续工作] → git mv 保留历史、tasks 引用同步为机械改动、迁移后立即以该目录跑脚本冒烟
- [agent-browser 口径残留导致 spec-gate 判定漂移] → 迁移收尾 grep 对账（见 D6 白名单）+ check-standards 不涉及此项，单独在 tasks 里落一条 grep 验收
- [report.md 契约正则与实战文本漂移（未来报告写法演化）] → 冒烟 fixture 钉住当前实战形态；契约变更须走 spec 修改（同 review-trace-gate 先例）

## Migration Plan

1. 新脚本 + 冒烟先落（含以 add-topic-key-event-markers 现目录为 fixture 的预演）
2. 存量 `ui-acceptance/` → `ui-verification/` 迁移（此刻脚本与门禁都还没接线，无窗口期风险）
3. spec-gate ⑥' 接线 + lib 旧判定删除 + 冒烟同步改写
4. check-tasks-tail / archive-readiness 接线
5. 文档口径批量迁移（规范/test-design/layout/skill 引用）
6. 部署影响：pi 扩展热载（spec-gate 改动即时生效）；无数据库/前端产物变化；回滚 = revert 提交即可（脚本/文档/扩展均无状态迁移）

## Open Questions

- 三道防线第一道（check-tasks-tail 增规则）若实现时发现 major 档任务行匹配误报率高，可降级为「apply 入口 warn 提示 + 归档硬阻断」两道——不改变 spec 判定行为，仅防线位置调整，实现期可定。
