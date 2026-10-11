<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## 入口与入口变更

唯一入口变更：设置页 `SettingsWorkspace` 既有「研究数据源」section（`pages/settings.vue` 注册）——数据驱动扩容三张源状态卡（`yahoo_finance_chart` / `fred_series` / `gdelt_doc`），并在 section 内新增「FRED API key」管理块（模式逐像素照既有 Comtrade key 管理块：脱敏回显 + 保存留空不改 + enabled 开关）。信号报告视图 `SignalReportView.vue` 零改动（as-of 行读已入库 payload，既有机制自动覆盖新源）。

## 受影响状态

| 状态 | 受影响点 | 行为 |
| --- | --- | --- |
| success | 三张新源卡 probe 成功 | 行内展示成功行数/耗时（复用既有 probe 结果行模式）；yahoo/gdelt 默认 enabled，fred 无 key 时 disabled + 缺失原因 |
| error | probe 失败 / FRED key 缺失 | probe 失败行内展示 error_code 与 detail（区分配置缺失/网络/结构漂移）；fred 卡 disabled 态带「配置 FRED_API_KEY 后启用」提示 |
| loading | probe 请求中 | 卡片级按钮 loading（复用既有 probe 按钮 loading 态），不阻塞 section 内其他卡 |
| empty | 无新增空态 | 目录恒有七源，无空列表场景 |

## 复用组件与布局模式

- 源状态卡：复用既有「研究数据源」section 的卡片组件与 `grid grid-cols-1 md:grid-cols-2` 网格（integrate-research-data-sources ui-design 既有契约），不新增卡片组件。
- FRED key 管理块：复用 Comtrade key 管理的表单模式（AppInput 脱敏回填 + AppButton 保存 + enabled 开关），不新增交互模式。
- 布局 mode：设置页既有 workspace 布局不变，无 dialog、无新页面。

## 验收映射

- 组件测试（Vitest）：设置页 section 渲染七源卡；fred 卡在无 key 时 disabled + 原因；FRED key 表单保存空串不清除（既有 comtrade 用例同款断言复制）；probe 成功/失败行内展示。
- 人工验证：配置 FRED key 后卡翻 enabled（无重启）；对 yahoo/gdelt 点「测试连通」返回成功耗时。
