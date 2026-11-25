# Human feedback

## 原型与实现口径

### FB-1 2026-11-20【产品|frontend】原型只说明改动方向，不作 1:1 实现规格
- **状态**：已消化
- 用户反馈：「可以，不要1：1按原型实现，这只是让我了解改动的方向，通过了，可以派发子线程opsx-apply了」。
- 处理：`ui-prototype/`（index.html + fixture.json + README.md）保留为方向说明与验收参照；`ui-design.md` 的 `## Prototype` 节明确「原型不是像素级实现规格」。实现子线程按代码库既有风格落地（形态对即可：角标/胶囊、中性色、不占规则色语义），不逐一对齐原型的类名与像素，也不得为对齐原型扭曲既有组件结构。
- 回检：用户已确认原型方向通过并授权进入实现；本 change 为 `ui-impact: minor` 档、`ui-approval: not-required`，无需 major 档原型审批。
