# dsh Web UI 预设作用域观察（真实 Chrome，2026-09-09）

> 归因：connect-dsh-energy-data-sources 3.4「dsh 实际作用域确认」的部分证据。
> 工具：opencli v1.8.7（真实 Chrome，profile puvphb56 持久登录态），opencli doctor 全 OK。
> 只读观察：打开页面、读 state、点击预设选择器，**未发送任何 prompt、未触发模型调用、未读写凭据**。

## 观察记录（动作序列）

1. `opencli browser energy-dsh open http://127.0.0.1:3080/` → 200，页面加载成功（登录态可用）。
2. Composer 页面：工作区 `syntopica-profile`；Agent 预设选择器（按钮 aria-label「即将开始的这个会话所用的 Agent 预设」）显示**能源研究**为当前选中；模型 DeepSeek V4 Flash · High；访问模式「工作区内修改」。
3. 点击预设选择器展开菜单，菜单列表含：**标准模式**（描述「功能完整的编码 Agent，支持文件编辑、Shell、文件与网页检索、Skills、计划、目标、子代理和工作流。」——**不含**能源 MCP 工具声明）、PTC 模式、极简模式、创造模式、**能源研究**（选中态 ✓，描述：「原油供需研究助手：证据优先，先查事实再解释；**已接 EIA WPSR（美国周度）与 JODI（各经济体月度）原油数据 MCP 工具**，另有网页检索与抓取；无 Shell、文件编辑与委派能力。」）。

## 结论（范围如实标注）

- ✅ **host 预设目录已生效**：运行中的 dsh（127.0.0.1:3080）真实读到 live 部署的 `~/.dsh/.agent-presets/energy-research/preset.yml`（描述含「已接 EIA/JODI MCP 工具」），且 `standard` 的描述不含该声明——与仓库源/`test_preset_config.py` 断言口径一致。此为真实 host 目录证据（Web UI 读 live 文件），非仓库侧自证。
- ⚠️ **会话级工具目录未验**：新建会话挂载后 agent 工具目录中的 `mcp__energy_data__eia_wpsr_table1` / `mcp__energy_data__jodi_oil_primary` **未在本次观察中确认**（composer 页无工具目录入口；不发 prompt 不触发模型的前提下未能取得会话级工具清单）。**因代码评审修复进行中（tools/energy-mcp 将再改），会话挂载与工具目录复核由修复后复验完成**，本观察不冒充 3.4 通过。

## 后续复验建议（修复后）

- 新建空白验证会话（不发 prompt）→ 只读工具目录/同源 RPC 确认两工具加载、standard 无此两工具。