<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

# UI 设计 — N/A

## 影响等级

`none`。本 change 只改 `.pi/extensions/` 下的门禁扩展与其 smoke 用例，**不产生任何用户可见界面**，不触及产品前端与后端。

## N/A 理由

- 无前端代码改动。
- 无新增组件、无交互路径变化、无容器宽度决策。
- 被本 change 约束的对象是「agent 写原型文件的时机」，不是产品界面本身。

## 验收方式

无法用 agent-browser 或视口截图验证（无可渲染对象），改为：

1. `bash .pi/extensions/tests/run-harness-smoke.sh` 全绿（含本 change 新增用例）；
2. 纯函数直测覆盖 stage 解析、路径域判定、change 名提取三组分支；
3. 集成用例：真 tmp 仓库 + 真 events.db，在 **`requirements` 档**写入 `ui-prototype/`，断言产生 `warn` 事件且**未产生 block**。

## 原型

`ui-prototype: none`。
