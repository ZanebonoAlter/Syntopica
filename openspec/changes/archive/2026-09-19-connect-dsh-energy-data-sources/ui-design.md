<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## 入口与入口变更

本 change 不触及 Syntopica 前端（front/ 零改动）。唯一用户可见界面变化在 **dsh 自有 UI（127.0.0.1:3080）**：「能源研究」预设的会话能力面新增两个 MCP 工具（`mcp__energy_data__eia_wpsr_table1` / `mcp__energy_data__jodi_oil_primary`），入口仍是 dsh 现有的新建会话→预设选择器，无新页面/导航/布局。

## 受影响状态

- preset 挂载成功态：新会话加载 energy-research 预设后，工具目录出现上述两个工具。
- 启动失败态：`failOnStartupError: true` 使预设 apply 直接失败（不出现「看似已接其实失败」的静默降级）——这是配置层显式选择的行为，不是 UI 状态。
- 本 change 无 loading/empty 渲染路径变更（工具调用结果渲染由 dsh 既有 tool-result 展示承担）。

## 复用组件与布局模式

不适用——无 Syntopica 组件改动；dsh 侧仅消费其既有预设选择器与工具展示结构（不加自定义 UI）。

## 验收映射

人工验证（dsh 实际作用域确认，见 tasks 验证节）：新建空白验证会话挂载 energy-research 预设后，通过 dsh 自身暴露的只读工具目录/已安装包 RPC 确认两个工具真实加载；若无法无模型获取作用域目录，该项保留未验并如实记录（不以 MCP 独立进程连通冒充 dsh 挂载成功）。
