# runtime 核查发现（connect-dsh-energy-data-sources）

> 只读取证于 2026-09（dsh 0.1.2-rc.1 npm 根 `_npx/1e7f6d9597241db0`）。与 explore-findings.md 分文件维护，避免争用。

## 1. Windows 解释器（cmd.exe 实测）

命令：`cmd.exe /C "node --version & uv --version & python --version"`

- node v24.14.0 / uv 0.7.9 / Python 3.14.3 —— 三者均在 Windows PATH 可用，EXIT=0。
- 路线可行：UV + Python + 官方 MCP Python SDK；解析用 stdlib csv/zipfile。
- 注意：Python 3.14 较新，若 `mcp` SDK 上限低于 3.14，用 `requires-python` + uv 自动取合适解释器（uv 可自管 Python），不必预装。

## 2. dsh-mcp-client 契约（lib/index.js + README.zh.md 精读）

**stdio Config union 字段**（Schemastery 定义，lib/index.js 末段）：

- `transport: "stdio"`（const 判别）
- `serverName`：必填，pattern `^[A-Za-z0-9_-]{1,32}$`，同一注册作用域内唯一（Agent 作用域间可复用）
- `command`：必填；`args`: string[]，默认 `[]`
- `env`: dict<string,string>，默认 `{}`，**合并到 scrubbedParentEnv() 之上**：父环境里匹配 `/KEY|PASSWORD|SECRET|TOKEN/i` 的名字与所有 `DSH_*` 名字先被删除。→ 若以后使用需 key 的 API，不能依赖父环境直接继承；本次使用 EIA 匿名公开 CSV，不需要 EIA_API_KEY，也不添加任何密钥配置。
- `cwd`: string，默认 `""`（传给 StdioClientTransport）
- `toolCallTimeoutMs`: 默认 60000
- `failOnStartupError`: 默认 false
- `reconnect`: { enabled: true, initialDelayMs: 500, maxDelayMs: 30000, maxAttempts: 10 }，字段可覆盖，未知键报错

**行为**：
- `inject: ["tools"]`，只注入 tools 服务、不 provide 服务 → 多实例无需 isolate。
- 工具公开名 `mcp__<serverName>__<rawName>`，归一化到 `[A-Za-z0-9_-]` 且 ≤64 字符；有损时追加 12 位 SHA-256 hash。wire 上只发 rawName。
- 启动失败：`failOnStartupError:false` 时 harness 照常启动、工具不出现、记 error，按退避重连（500ms→30s，10 次耗尽后注销工具停止）；`true` 时 apply() 抛错中止插件激活。
- 连接/发现超时继承 MCP SDK 默认 60s（不暴露配置）。
- 每台服务器一条插件实例配置；多服务器 = 多条 mcp-client 行。
- env 明细：`buildChildEnv(extra) = { ...scrubbedParentEnv(), ...extra }`，显式 env 优先。

## 3. repo 内 Python MCP 复用

- codegraph *.py 仅 4 个脚本（scripts/ 下，非 MCP）；唯一 pyproject 在 `tests/data_enrichment_poc/`（akshare+requests PoC，无 `mcp` 依赖）。**无可复用项目。**
- 建议：新建 `tools/energy-mcp/` 独立 pyproject（deps: `mcp` 官方 SDK + httpx/requests），不改 Syntopica 应用运行时。
- 启动形态（cordis.yml mcp-client 行）：`command: uv`，`args: ['run', '--project', 'D:/project/Syntopica/tools/energy-mcp', ...]` —— 绝对 Windows 路径用正斜杠（bash→cmd 反斜杠会被吞；cmd/uv 均接受正斜杠）。

## 4. 预设现状（两处 diff 确认逐字一致）

`config/dsh/presets/energy-research/` 与 `~/.dsh/.agent-presets/energy-research/` 的 agent.cordis.yml、preset.yml 均 IDENTICAL。

**现状**：forward-composed，无 shell/fs/editor/委派；无 MCP 行（注释声明 "no MCP rows…EIA/JODI/STEO are not implemented yet"）；persona 有红线句「当前未接入 EIA/JODI 等专业数据接口，也没有 MCP 工具；不得声称已接入，也不得假装查询过这些数据源」；preset.yml description 含「专业数据接口待接入」。

**要改的点**：
1. agent.cordis.yml 追加一条 mcp-client 行（单个 energy_data stdio 服务器提供 EIA、JODI 两类工具）+ 头部注释同步改。不为两个来源重复启动两套服务器。
2. persona：删除/改写上述红线句 → 使用真实工具、陈述数据时尊重工具返回的 metadata（统计期/地域/单位/来源以工具元数据为准）。
3. preset.yml description 的「专业数据接口待接入」同步更新。
4. 无显式 MCP denylist 行（是"缺失"而非"拉黑"），但旧 change 的 verify-structure.mjs 断言了"无 MCP rows"旧状态——历史测试不能当新契约，新 change 自带结构测试。
