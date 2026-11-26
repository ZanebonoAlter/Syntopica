# UI Design — retire-heuristic-tag-fallback

<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->


## 1. 变更面

**无。** 本 change 不新增、不删除、不改动任何组件、样式或交互。

## 2. 契约影响

- **无新界面元素**：失败态的可见性是**既有能力**——标签队列面板 `front/app/features/settings/components/TagQueuePanel.vue:282` 已经渲染 `task.last_error`（失败原因红色文字 + title 提示）。本 change 只是让「抽取失败」这个原因能出现在那里（此前被 heuristic 兜底吞掉，任务被误标 completed，面板上根本看不到失败）。
- **无既有界面元素移除**：没有任何界面元素依赖 `source === 'heuristic'`（已核实全前端零 `heuristic` 引用，唯一的 "heuristics" 是 `detective-wall/utils.ts:44` 的无关英文注释）。
- **无布局/弹窗/浮层影响**：不涉及 `AppPageShell` 四模式、`AppDialog` 尺寸档、`Z_INDEX` 浮层档位表。
- **无窄屏断点影响**：不新增窄屏降级路径。

## 3. 验收

**none 档 = N/A 一致**：本 change 无 UI 制品可验。

- 界面侧唯一可观察的变化是**信息增强**（队列面板开始显示真实的抽取失败原因），属既有渲染路径，非本 change 的界面产物；
- 界面行为回归由既有前端测试覆盖（`TagQueuePanel` 所在 settings 域），本 change **不触碰任何前端文件**，无需新增前端用例。

## N/A Reason

本 change 声明 `ui-impact: none`，理由：

1. **零前端文件改动**——`front/` 一字未动；删除的是后端规则词兜底链路与存量数据。
2. **无界面元素增删**——唯一的可见变化是**间接的数据变化**（打标失败时文章不再带 Coding/AI Agent 泛词），非界面改形。
3. **失败可见性复用既有渲染**——`TagQueuePanel.vue:282` 本来就在渲染 `task.last_error`；本 change 只是让「抽取失败」这个原因能出现在那里（此前被兜底吞掉，任务被误标 completed，面板上根本看不到失败）。
4. **无布局契约影响**——不涉及 page shell 四模式、`AppDialog` 尺寸档、`Z_INDEX` 浮层档位表、窄屏断点。
