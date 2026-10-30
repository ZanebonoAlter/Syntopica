# test-cases: fix-nested-dialog-layering

## 故事 S1: 用户在治理工作面里点「创建并挂载」，创建组合标签的弹窗必须看得见、点得动（锚 Requirement: 浮层层级契约）

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 打开版块「本板块上下文 / 治理」工作面（`AppDialog size="xl"`，遮罩层级取基础档） | 弹窗内嵌套浮层高于宿主遮罩 / 默认遮罩层级取基础档 | 工作面正常打开，遮罩层级 = `Z_INDEX.dialog`（1000） | 组件 | front/app/components/ui/AppDialog.test.ts |
| 2 | 工作面切到「组合」页签，点「创建并挂载」 | 弹窗内嵌套浮层高于宿主遮罩 | 组合标签创建弹窗出现，且其 z-index **严格大于**宿主遮罩的 z-index | 组件 | front/app/features/tags/components/CompositeLabelEditDialog.test.ts |
| 3 | 弹窗内填名称 + 选 2-5 个组件 → 点「创建」 | 层级档位单一事实源（创建链不回归） | 创建成功、挂载到当前版块、弹窗关闭（既有行为不变） | 组件 | front/app/features/tags/components/CompositeLabelEditDialog.test.ts |
| 4 | 实机复核：治理工作面内点「创建并挂载」，弹窗中心点命中弹层本体 | 弹窗内嵌套浮层高于宿主遮罩 | `elementFromPoint` 命中组合弹窗本体（非宿主工作面遮罩），肉眼可见弹窗与表单 | 浏览器（agent-browser） | 人工：见 tasks 5.4 验证节 |

### 变体走查

| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 输入变体 | 组合名称空 / 组件少于 2 / 多于 5 | 既有校验行为不变（提交按钮禁用 + 行内计数错误），层级修复不得顺带改这些 | 组件 | front/app/features/tags/components/CompositeLabelEditDialog.test.ts（既有用例） |
| 前置变体 | 宿主弹窗未打开（标签池页直接开） | 弹窗照常可见（页面级场景，层级高于页面内容即可） | 组件 | front/app/features/tags/components/CompositeLabelEditDialog.test.ts |
| 前置变体 | 宿主弹窗在更高档（阅读层 9100） | 嵌套浮层按 `nestedOverlayZ(hostZ)` 抬升后仍高于宿主 | 组件 | front/app/components/ui/layout-contract.test.ts |
| 时间窗口变体 | 弹窗打开后切换宿主页签 | 不适用（宿主页签切换不关闭嵌套浮层，非本次改动面） | — | 划除：宿主页签切换与浮层生命周期无关，层级修复不涉及 |
| 幂等变体 | 重复点「创建并挂载」 | 同一个弹窗实例（visible 已是 true），不叠第二层 | 组件 | front/app/features/tags/components/CompositeLabelEditDialog.test.ts（既有行为） |
| 可用性变体 | 误输入反馈 | 名称/组件不足时提交按钮禁用并给行内提示（既有行为） | 组件 | front/app/features/tags/components/CompositeLabelEditDialog.test.ts（既有用例） |
| 可用性变体 | 空态 | 候选组件列表为空时显示「暂无可用辅助标签」（既有行为） | 组件 | front/app/features/tags/components/CompositeLabelEditDialog.test.ts（既有用例） |
| 可用性变体 | 错误态 | 候选接口失败降级为 aux 全量、再失败显示空列表（既有行为） | 组件 | front/app/features/tags/components/CompositeLabelEditDialog.test.ts（既有用例） |
| 可用性变体 | 加载态 | 候选加载中显示 spinner（既有行为） | 组件 | front/app/features/tags/components/CompositeLabelEditDialog.test.ts（既有用例） |
| 可用性变体 | 超长文本展示 | 组合名称超长时不撑破弹窗（`cld-card` 宽度 `min(560px, 92vw)` + 已有 ellipsis 规则，层级改动不得改布局） | 组件 | front/app/features/tags/components/CompositeLabelEditDialog.test.ts |

### 效果核对

- 触发原因：层级修复的效果在「真实浏览器里浮层是否真的压过宿主遮罩」——单测只比较 z-index 数值，数值对但被 stacking context 锁死仍会被盖（`transform` / `backdrop-filter` 会创建层叠上下文）。
- 核对方法：静态发布实例（`http://localhost:5100/tags/1974`）上打开治理工作面（右侧「查看候选与依据」）→「组合」页签 → 点「创建并挂载」→ 用 `elementFromPoint` 探测弹窗中心命中元素是否为本弹窗本体，并截图留档（`ui-verification/A-1-nested-dialog-visible.png`）。
- 量化结果：命中弹层本体 1/1（命中 `.cld-picker`，`hitInsideNested=true`；宿主遮罩内联层级 1000、嵌套浮层内联层级 1100）。**A/B 对照**：同页把嵌套层级改回修复前的手写值 60 再探测 → 命中 `.bhd-section`（宿主工作面内容，`hitInsideNested=false`），即故障态当场复现——此前命中弹层本体 0/1。
- 结论：达标交付（层级修复在真实浏览器生效，无 stacking context 锁层）。

### 继承与调整（MODIFIED `unified-dialog` 必答）

| 旧 Scenario | 处置 | 旧测试文件 | 动作 |
| --- | --- | --- | --- |
| 新弹窗使用四档尺寸 | 继承 | front/app/components/ui/AppDialog.test.ts | 照跑（尺寸档行为不变） |
| 主题自动适配 | 继承 | front/app/components/ui/AppDialog.test.ts | 照跑（主题 token 行为不变） |
| 组合标签手动创建（组件选择器 / 去重 / 数量约束 / 版块挂载） | 继承 | front/app/features/tags/components/CompositeLabelEditDialog.test.ts | 照跑（创建链零改动） |
| 组合标签在治理工作面的挂载/摘除展示 | 继承 | front/app/features/tags/components/console/BoardHealthPanel.test.ts | 照跑（组合区渲染零改动） |

### 白盒附加

不适用（simple 档：无状态机 / 无算法 / 无跨模块协议；本次只引入常量档位表与一个相对抬升纯函数，边界值由 `layout-contract.test.ts` 数值断言覆盖）。
