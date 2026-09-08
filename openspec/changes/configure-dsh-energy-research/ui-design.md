<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

# UI 设计契约（minor：复用/状态契约）

## 入口与入口变更

本 change 不改 Syntopica 前端（`front/` 零改动）。唯一 UI 面在外部 dsh 自有 Web UI（本地 3080 端口）的**现有预设选择器**：新会话创建时该选择器多出一个「能源研究」选项（显示名 + 描述来自 `preset.yml`），复用 dsh 选择器既有条目结构（与内置「标准模式」同构），不新增页面/弹窗/导航/交互模式。

## 受影响状态

Syntopica 前端状态（loading/empty/error/success）：全部 N/A——无 Syntopica 页面触碰。dsh 侧状态由 dsh 自身渲染，不属于本仓库 UI 契约；本 change 只保证配置文件正确（broken preset 会由 dsh roster 以原因列出，验证节覆盖）。

## 复用组件与布局模式

无新组件、无布局模式选择：dsh 选择器条目由 `preset.yml`（name/description/order: 20）驱动，`order` 排在 standard（order: 1）之后。Syntopica layout mode 枚举不适用。

## 验收映射

- 验收方式：opencli（复用用户真实 Chrome 登录态，只读 roster 或空白验证会话；不发 prompt、不触发模型/工具调用）——确认「能源研究」出现在预设选择器且「标准模式」仍在、默认不变。
- 受限回退：若浏览器操作预算（≤12 次 / ~3 分钟）内无法完成，保留为未验项，以静态结构验证（YAML 解析 + 包名可解析 + roster 发现契约）作部分证据，不冒充运行验收。

## N/A 说明

- 双视口（1440×900 / 1920×1080）视觉检查：N/A——外部工具自有 UI，无本仓库视觉契约；且本 change 对该 UI 的改变仅限选择器多一条目。
- 组件测试 / Syntopica e2e：N/A——`front/` 无改动。
