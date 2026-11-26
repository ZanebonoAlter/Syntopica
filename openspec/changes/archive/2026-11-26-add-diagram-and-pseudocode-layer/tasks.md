<!-- stage: implement -->

# Tasks

## 1. 契约（spec delta）

- [x] 1.1 `图块的最小可读约束` MODIFIED：加「图 SHALL 在对话中呈现」，保留原两个 Scenario 并补「图在对话中可见」
- [x] 1.2 `三类图的适用判据` MODIFIED → 五类（补 `erDiagram` 与 用例图 flowchart），判据只回答「画哪几类」不再当闸门
- [x] 1.3 新增 `图的存在由 marker 决定，默认画`（二值 marker + 理由必填 + 不判理由正确性）
- [x] 1.4 新增 `伪代码成为 design.md 的强制节`（三件事 + 粒度判据 + N/A 写法）
- [x] 1.5 新增 `diagrams 门禁只查形式`（三查三不查 + legacy 不追溯）
- [x] 1.6 `openspec validate` → valid

## 2. 门禁实现

- [x] 2.1 `scripts/harness/archive-readiness.sh` 加第 ⑧ 项：marker 存在 / 二值合法 / none 有理由 / full 有图且含 mermaid 块
- [x] 2.2 legacy 豁免：无 `.openspec.yaml` 或已归档 change 不查
- [x] 2.3 负例测试：造 3 个临时 change（marker 缺失 / none 空理由 / full 无图）确认各自判红

## 3. 文档

- [x] 3.1 `docs/reference/开发执行规范.md` §13 重写：四类图 + marker + 对话呈现 + 伪代码节
- [x] 3.2 同文件 §11 归档门禁表加一行「diagrams marker 与图/理由一致性」
- [x] 3.3 `docs/reference/standard/shared/doc-authoring.md` 补伪代码节与 marker 的写法约定
- [x] 3.4 §0.6 步骤 1 的「涉及多步流程时先写 design-diagram.md」改为按 marker 判断

## 4. 样板

- [x] 4.1 给已归档的 `2026-11-26-merge-lane-ops-specs` 补 `design-diagram.md`（只增不改，§12 补档范畴）
  - ER 图：spec 族 → capability → Requirement → Scenario
  - 顺序图：delta 生成 → 守恒验证 → 归档三步
  - 伪代码：`mergeSpecFamily()` 的控制流与守恒断言
- [x] 4.2 该样板的图在对话里贴出来（验证「对话中可见」这条约束自己成立）

## 5. 验证

- [x] 5.1 `bash scripts/harness/archive-readiness.sh add-diagram-and-pseudocode-layer` 全绿（本 change 自己要有 marker + 图）
- [x] 5.2 负例测试通过（任务 2.3）
- [x] 5.3 已归档 change 不因缺 marker 判红（回归：随便挑 3 个归档 change 跑门禁）
- [x] 5.4 `check-standards.sh` 无新增失败

## 6. review

- [x] 6.1 派 review（`minimax-cn/MiniMax-M3.1-Flash:max`）
- [x] 6.2 处置 review 发现

## 7. 归档

- [x] 7.1 六项门禁全绿
- [x] 7.2 `openspec archive add-diagram-and-pseudocode-layer --yes`
- [x] 7.3 `openspec validate --specs` 全过

## 8. 测试

门禁脚本的负例测试（临时目录造 change，不入库）：

| 场景 | 构造 | 期望 |
| --- | --- | --- |
| marker 缺失 | 有 `.openspec.yaml`，proposal 无 marker | 判红，提示写 full/none |
| none 空理由 | `<!-- diagrams: none() -->` | 判红，提示理由必填 |
| none 有理由 | `<!-- diagrams: none(只改措辞) -->` | 通过 |
| full 无图 | `<!-- diagrams: full(有表结构变更) -->` 但无 `design-diagram.md` | 判红 |
| full 有图无块 | 有文件但无 ` ```mermaid ` | 判红 |
| full 合规 | 有文件且含 mermaid 块 | 通过 |
| legacy | 已归档 change 无 marker | 通过（不追溯） |

## 9. 文档

<!-- doc-impact: dev -->

- `docs/reference/开发执行规范.md` §13 / §11 / §0.6（3.1–3.4）
- `docs/reference/standard/shared/doc-authoring.md`（3.3）
- `docs/research/process-arch-rework/issues.md`：ISS-01 回填决策与落地（见 10.1）

## 10. 验收

- [x] A marker 二值唯一、理由必填、机器不判理由正确性
- [x] B 五类图判据写进 §13，含 ER 与用例图（用例图注明是 flowchart 模拟）
- [x] C 「图 SHALL 在对话中呈现」是硬约束，不是建议
- [x] D 伪代码节默认要有、N/A 写理由
- [x] E 门禁三查三不查，legacy 不追溯
- [x] F 样板 change 补图完成且图在对话里出现过
- [x] G ISS-01 的 Q1–Q6 六个待定问题各有结论（记进 ledger）

## 11. ISS-01 六个待定问题的结论（本 change 一并落定）

| Q | 结论 |
| --- | --- |
| Q1 必画判据 | **反转**：默认画，`none` 需理由；判据只回答「画哪几类」 |
| Q2 工具链 | **mermaid 直出**，不用 chromium/PNG（实测 pi-web 渲染，零脚本） |
| Q3 进制品链 | `design-diagram.md` 非 schema 制品不变；伪代码进 `design.md` 强制节 |
| Q4 渲染产物入库 | **不入库**——对话里看，文件只留 mermaid 源 |
| Q5 伪代码层 | `design.md` 的 `## 核心逻辑（伪代码）` 节，粒度到「能发现问题」 |
| Q6 与 ui-design.md | L0 引用图时写指针，不复制 |
