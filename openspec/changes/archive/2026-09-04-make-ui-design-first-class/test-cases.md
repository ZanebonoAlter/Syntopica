# 测试用例：UI 设计一等制品工作流

> 测试单元：一个 change 从 proposal 声明 UI 影响，到 UI 设计/原型审批、进入实现、完成交互视觉验收并归档的完整故事。黑盒契约见 `specs/ui-design-workflow/spec.md`；本文件补充主链路、变体与门禁纯函数白盒覆盖。

## 1. 主链路节拍

| 步 | 用户/系统动作 | 来源 Scenario | 期望 | 层 | 计划落点 |
|---|---|---|---|---|---|
| 1 | 创建纯后端 change 并声明 `ui-impact: none` | 纯后端 change 声明 none | 生成最小 N/A `ui-design.md`，无需原型和审批 | schema smoke | `.pi/extensions/tests/ui-design-gate.smoke.cjs` |
| 2 | 创建 minor UI change | minor 复用既有交互 | UI 制品记录入口、复用点和受影响状态，可继续生成 specs/design | schema smoke | `.pi/extensions/tests/ui-design-gate.smoke.cjs` |
| 3 | 创建新增面板的 major UI change | 新面板必须声明 major；major 形成完整 UI 契约 | 生成完整 UI 合同与 change 内原型，审批保持 pending | schema + 文件 smoke | `.pi/extensions/tests/ui-design-gate.smoke.cjs` |
| 4 | pending 状态尝试 apply/写项目代码 | 原型待确认时暂停规划；major pending 阻止实现写入 | propose 暂停；实现 mutation 与 Agent 派发被阻断，仍可编辑 UI 制品 | gate smoke | `.pi/extensions/tests/ui-design-gate.smoke.cjs` |
| 5 | 用户明确批准原型 | 用户明确确认后放行；完整规划正常放行 | marker 改为 approved，后续 artifacts 与 apply 放行，健康放行零事件 | gate smoke | `.pi/extensions/tests/ui-design-gate.smoke.cjs` |
| 6 | 使用统一 page shell/dialog 尺寸实现 | 治理列表选择 contained；弹窗拒绝随手宽度 | contained 在宽屏居中，dialog 使用 size tier 且旧 width API 不回归 | Vitest | `front/app/components/ui/AppPageShell.test.ts`、`front/app/components/ui/AppDialog.test.ts` |
| 7 | 完成 major UI 后执行验收 | major UI 双层验收通过 | opencli 主链路 + 两档视觉报告 + 原型差异均有证据 | archive smoke + 人工视觉 | `.pi/extensions/tests/ui-design-gate.smoke.cjs`、人工：按 `ui-verify` 截图检查 |
| 8 | 尝试归档缺视觉证据的 major change | 只有单元测试不得完成 major UI | spec-gate 阻止归档并给出缺失项 | gate smoke | `.pi/extensions/tests/spec-gate.smoke.cjs` |

## 2. 变体走查

### 2.1 输入变体

| 变体 | 预期答案 |
|---|---|
| proposal 为空/纯空白 | 新 schema 判 `ui-impact-missing` 并阻断 |
| marker 值为空、`Major`、`large`、多个冲突 marker | 均视为非法或歧义并阻断；只接受一个小写枚举值 |
| marker 前后有普通空白 | 容忍 HTML 注释内部空白，但输出仍规范化为标准格式 |
| `ui-design.md` marker 缺失/重复/冲突 | 对应等级合同无效并阻断 |
| 原型路径为空、绝对路径、`../` 越界、目录、断链 symlink | major 判 `ui-prototype-missing`；只接受 change 根内普通文件 |
| 超长理由/自由宽度说明 | 文档可保留，但 policy target/诊断必须有界且不得整段入库 |

### 2.2 前置数据变体

| 变体 | 预期答案 |
|---|---|
| 新 schema + none + N/A 完整 | 放行 |
| 新 schema + minor + 无原型 | 合同字段完整则放行 |
| 新 schema + major + 原型存在 + pending | 阻断 |
| 新 schema + major + approved + 原型存在 | 放行 |
| 新 schema + major + approved + 原型不存在 | 阻断，不相信孤立 marker |
| 旧 spec-driven change 无 marker/UI 制品 | 不硬阻断；触及前端时每 session/change 提醒一次 |
| 旧 spec-driven change主动补齐新制品 | 按制品校验，但不改变其原 schema 依赖图 |
| policy helper 尚未可用/事实库写失败 | 原裁决不变；写入错误 fail-open 仅指记账，不把被阻断操作放行 |

### 2.3 时间与状态变体

| 变体 | 预期答案 |
|---|---|
| requirements 档编辑 proposal/ui-design/prototype | 始终允许 |
| implementation 档 major pending 编辑当前 change 的 UI 文件 | 允许修复合同；其他项目 mutation 与 Agent 派发阻断 |
| 用户批准后同一 session 再尝试 | 重新读磁盘，不沿用旧 pending 缓存，正常放行 |
| approved 后重大修订 | marker 必须重置 pending；未重置由内容/哈希或 review 检查报告不一致 |
| approved 后仅改错别字/颜色微调 | 可记录差异，不强制重审 |
| archive 阶段证据先缺后补 | 第一次阻断有事件；补齐后通过且普通成功零记录 |

### 2.4 幂等与并发变体

| 变体 | 预期答案 |
|---|---|
| 同一 tool_call 被重复检查 | 每次裁决一致；不得修改制品 |
| 同 session 多次触发 legacy 提示 | 只提醒和记 warn 一次 |
| 多 session 触发 legacy 提示 | 每 session 可各提示一次，便于恢复上下文 |
| 同一 major pending 连续触发不同 mutation | 每次危险操作均阻断；事实记录可全量保留 block |
| policy 写入并发 | 依赖既有 WAL/busy_timeout，失败旁路不改变 block |
| prototype 用户修订与 apply 几乎同时发生 | 每次 gate 从磁盘读取最新 marker/文件，不用跨回合陈旧快照 |

### 2.5 可用性变体

| 变体 | 预期答案 |
|---|---|
| 用户不知道为何暂停 | 提示明确列出缺失项、原型路径、如何返回 requirements 与如何确认 |
| none change 被迫写长文 | 不发生；最小 N/A 模板控制在 marker + 理由 |
| minor 被误判 major | 提供修改声明与理由的正常路径，不要求 bypass |
| major 原型打开失败 | 不允许口头确认；先修复可访问原型 |
| 布局例外确有必要 | `ui-design.md` 写自由宽度理由、目标视口和溢出策略后允许 review |
| 宽屏内容被无限拉长 | 1920×1080 视觉验收应报阻断，除非合同选择 workspace 并说明用途 |

## 3. 效果核对

- **触发原因**：流程价值无法只靠单元断言证明，还需确认新 schema 真正改变 artifact 顺序与 agent 行为。
- **方法**：创建临时 none/minor/major change，分别运行 `openspec status/instructions`；对 major 原型走一次 pending→用户确认→approved→apply preflight；从 `.pi/harness/events.db` 查询显著裁决。
- **量化口径**：三档 smoke 全部符合预期；major pending 的项目 mutation/Agent 派发阻断率 100%；健康放行 `policy.decision` 增量 0；legacy change 硬阻断数 0；schema validate 与 OpenSpec strict validate 退出码均为 0。
- **视觉口径**：page shell 在 1440×900 与 1920×1080 下符合所选模式；contained 不超过 1120px 且居中，workspace 使用可用宽度，dialog 四档均不超过 92vw。

## 4. 白盒附加

### 4.1 UI 合同解析分支表

| schema | proposal 声明 | ui-design | prototype | approval | 判定 |
|---|---|---|---|---|---|
| 新 | 缺失/非法/重复 | 任意 | 任意 | 任意 | block: `ui-impact-missing` |
| 新 | none | 缺失或非 N/A | none | not-required | block: `ui-design-missing` |
| 新 | none | N/A 完整 | none | not-required | pass |
| 新 | minor | 必填节缺失 | 可无 | not-required | block: `ui-design-missing` |
| 新 | minor | 完整 | none | not-required | pass |
| 新 | major | 缺失/节不全 | 任意 | 任意 | block: `ui-design-missing` |
| 新 | major | 完整 | 缺失/越界 | approved/pending | block: `ui-prototype-missing` |
| 新 | major | 完整 | 存在 | pending | block: `ui-approval-pending` |
| 新 | major | 完整 | 存在 | approved | pass |
| 旧 | 缺失 | 缺失 | 缺失 | 缺失 | warn once，pass |

### 4.2 工具动作分支表

| mode | 合同判定 | 工具/目标 | 预期 |
|---|---|---|---|
| requirements | 任意 | change UI 规划文件 | allow |
| requirements | 任意 | 普通读取/验证 | allow |
| implementation | pass | 任意正常工具 | allow |
| implementation | block | Agent 派发 | block |
| implementation | block | edit/write 项目代码 | block |
| implementation | block | edit/write 当前 change 的 `ui-design.md` 或 `ui-prototype/**` | allow |
| implementation | block | 普通 read/status/check | allow |
| implementation | block | 显式 bypass | allow + policy bypass |
| implementation | 检查异常 | 任意 | fail-open + 明显告警 + policy fail-open |

### 4.3 布局边界值

| 对象 | 边界 | 预期 |
|---|---|---|
| reader | viewport <760 / =760 / >760 | 小屏 100%-gutter；其余不超过 760 且居中 |
| contained | viewport <1120 / =1120 / 1440 / 1920 | 小屏 100%-gutter；宽屏不超过 1120 且居中 |
| workspace | 1280 / 1440 / 1920 | 使用父级可用宽度，不套 contained max-width |
| dialog sm/md/lg/xl | viewport 小于档位 / 等于 / 大于 | 宽度受 92vw 限制，无横向溢出 |
| split | 侧栏最小值、主栏 min-width=0、超长内容 | 主栏可收缩，溢出策略明确，不顶破页面 |

### 4.4 不适用项留痕

- 数据库/SQL 变体：不适用；本 change 不新增产品数据表，facts 库沿用既有 append-only 契约。
- 外部 API 失败：不适用；原型必须脱离真实 API，门禁只读本地文件。
- 业务时间窗口：不适用；仅 harness 事件 TTL 沿用既有 30 天，不在本 change 修改。
- 用户可见业务字段盘点：不适用；新增字段均为开发流程 marker，不进入产品界面或业务 API。
