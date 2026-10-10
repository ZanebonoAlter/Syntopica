<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## 入口与入口变更

两处既有入口内加元素，无新页面/弹窗/导航：

1. **SearXNG 配置面板**（`SearxngConfigPanel.vue`，设置工作台「数据源与网络」复合 section 内既有 dialog）：endpoint/enabled 表单下方新增一行「连通性」区——「测试连接」按钮 + 探测状态标记。dialog 尺寸档与触发入口不变。
2. **页边注批注卡**（`MarginNoteCard.vue`，日报页边栏既有组件）：问答轮 answer 元信息区（与 cited_web_sources chips 同族位置）新增条件渲染的「联网搜索无结果」小标记。

## 受影响状态

SearxngConfigPanel 连通性区（四态 + 加载态）：

| 状态 | 呈现 |
| --- | --- |
| idle（未探测） | 仅「测试连接」按钮可用，无状态行 |
| loading | 按钮 disabled + 转圈，防重复提交 |
| ok | 中性绿点 + 「连接正常 · {latency}ms」 |
| degraded | 琥珀点 + 「服务可达但搜索无结果（引擎可能故障）」 |
| unreachable | 红点 + 「无法连接（endpoint 不可达或超时）」 |
| disabled | 灰点 + 「未启用/未配置」 |

MarginNoteCard 标记（纯展示态，无交互）：

- `web_search_status=empty` → 灰色小标记「联网搜索无结果」，title 提示「本次问答联网搜索未获得结果，可在设置-数据源与网络中测试连接」
- `ok` / `disabled` / 旧行 NULL → 不渲染任何标记

## 复用组件与布局模式

- 按钮复用 `AppButton` 既有尺寸/变体档；探测中复用既有 spinner/loading 模式
- 状态点复用设置面板既有中性色点 + 文案样式（与 bocha/firecrawl 配置卡片的状态呈现同族，不新造视觉语言）
- 卡片标记复用 `mn-qa__webs` chips 的中性色系（D7 外链 chips 同族弱化样式），纯文本 + title，不引入新图标
- 不改动任何 layout mode、dialog 尺寸档、导航结构

## 验收映射

- `SearxngConfigPanel.test.ts`（Vitest）：四态 + loading 渲染断言、点击测试连接调用 API、保存后自动探测
- `MarginNoteCard.test.ts`（Vitest）：`web_search_status=empty` 渲染标记；`ok`/`disabled`/`NULL` 不渲染
- 健康端点 handler 层测试（Go）：四态判定 + 配置解析复用
- 主交互链路（设置面板探测）opencli 落点或人工豁免留痕（见 test-cases.md）
