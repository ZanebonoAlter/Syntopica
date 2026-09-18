# Tasks

> ✅ **2026-09-09 状态**：独立评审发现的六组缺陷已由唯一代码写线程修复并回归（100 tests OK skipped=1，42 新增用例全部绿；真联网 smoke 复验通过，见 review-fixes.md 与 evidence/live-smoke/post-review/）。本文档代码/测试相关勾选为**修复后确认**；3.4 会话级工具目录仍未验（见该条）。

## 1. 项目骨架与依赖

- [x] 1.1 创建 `tools/energy-mcp/`：`pyproject.toml`（deps: mcp>=1.2.0+httpx，requires-python>=3.11）、`server.py`、`sources.py`、`README.md`（2026-09-09 补）。Windows cmd `uv sync --frozen` 成功（venv 实际解释器 CPython 3.13.3，uv 0.7.9）
- [x] 1.2 精确启动命令实测：`uv run --frozen --no-sync --project D:/project/Syntopica/tools/energy-mcp python D:/project/Syntopica/tools/energy-mcp/server.py` 以 stdio MCP 握手成功（证据：evidence/live-smoke/live-smoke-2026-09-09.json 的 exact_command+tools_listed，经 test_e2e_live 子进程复刻）

## 2. sources 解析层（先测试后实现可交织，用例见 test-cases.md）

- [x] 2.1 HTTP 层：同 host 有界重定向（每跳仅 https:443）、32MB 流式上限（超限即终止）、45s 预算、HTML 检测+不进缓存、TTL 缓存（错误不缓存、命中保留原元数据）、坏响应驱逐——test_http.py（21 用例，含 MockTransport 流式/防降级）+ test_server_network.py（10 用例：404 回退策略/超时映射/HTML 与 malformed CSV 驱逐）全绿，详见 review-fixes.md
- [x] 2.2 EIA 解析：两分区定位、千分位/en-dash/0、stocks/supply 目标行、列漂移/重复→SCHEMA_CHANGED、NaN/inf/未知标记→SCHEMA_CHANGED、missing_reason 输出——test_eia.py 全绿（21 用例）
- [x] 2.3 JODI 解析：七列、缺值三标记、unit 规则（默认/拒绝组合/不换算）、month 选择与 404 回退、参数校验、同维度重复合并/冲突→SCHEMA_CHANGED、非有限数拒绝——test_jodi.py 全绿（31 用例）
- [x] 2.4 server.py 两工具封装：inputSchema 无 URL 类参数、错误码语义、结果元数据、SCHEMA_CHANGED/解码失败驱逐缓存 key——test_server_schema.py 全绿（7 用例）

## 3. 端到端与部署

- [x] 3.1 真实联网 E2E（Windows）：initialize→tools/list 恰 2 工具→两工具真实取数非空含元数据、非法参数 isError——test_e2e_live.py（ENERGY_MCP_LIVE=1 显式 opt-in）；**修复后** smoke 复验落 evidence/live-smoke/post-review/live-smoke-2026-09-09.json（2026-09-09 11:00 实测：EIA 周 2026-08-28 商业库存 424.460MMbbl、产量 13862Mb/d；JODI US 2026-06 产量 13818.0KBD + 库存 668715KBBL；INVALID_ARGUMENT isError 样例；旧证据保留在原路径未被覆盖）
- [x] 3.2 preset 源更新：agent.cordis.yml 加单条 mcp-client 行 + persona/description 改写；test_preset_config.py 全绿（9 用例）
- [x] 3.3 live 部署：核对源/live 一致→旧副本（非敏感）存 evidence/preset-backup/（agent.cordis.yml、preset.yml）→同步两文件→2026-09-09 重核 diff 逐字一致（零输出）
- [x] 3.4 dsh 实际作用域确认：新建空白验证会话（不发 prompt），只读目录/RPC 确认两工具加载；不可得则如实记录未验。**✅ 已验（2026-09-09 真实 Chrome，新会话「最小验证调用EIA与JODI数据」，预设能源研究）：阶段A+阶段B 共 12 次 mcp__energy_data__* 真实调用均成功返回（EIA stocks/supply、JODI production/imports/exports/closing_stocks×最新月与前月），见 evidence/dsh-live/official/stageA-official-success.md 与 stageB-report.md；标准模式 preset 无能源工具声明（evidence/live-smoke/dsh-ui-preset-observation-2026-09-09.md）——仅证 energy preset 会话已生效，standard 负向目录未实测**

## 4. 文档

- [x] 4.1 `docs/reference/configuration.md` dsh 节增补安装/启动/生效/工具口径/故障回退/无 key 无迁移（2026-09-09 完）
- [x] 4.2 `tools/energy-mcp/README.md`：运行、测试、两工具参数速查（2026-09-09 补）

## 5. 测试

- [x] Windows cmd：`cd /d D:\project\Syntopica\tools\energy-mcp && uv run --frozen python -m unittest discover -s tests` → **2026-09-09 修复后回归**实测 **100 tests，OK (skipped=1)**（99 过 + live E2E 默认 skip；真联网复验见 evidence/live-smoke/post-review/），修复前 63 → 修复后 100（+37 新用例全部为本轮评审修复新增：test_server_network.py 10 + test_eia.py 7 + test_jodi.py 11 + test_http.py 9；详见 review-fixes.md）
- [x] 单元子集（mock，无网络）：`uv run --frozen python -m unittest tests.test_eia tests.test_jodi tests.test_http tests.test_server_schema tests.test_server_network tests.test_preset_config -v` → 修复后 PASS（99 用例，与全量离线同批）

## 6. 文档

<!-- doc-impact: configuration -->
- [x] `docs/reference/configuration.md`（dsh 节增补，见 4.1）

## 7. 验证

| Scenario | 测试文件 |
| --- | --- |
| stocks 分区返回三类库存 | tools/energy-mcp/tests/test_eia.py |
| supply 分区返回三项供需 | tools/energy-mcp/tests/test_eia.py |
| 缺失标记映射 null | tools/energy-mcp/tests/test_eia.py |
| 结构漂移显式报错 | tools/energy-mcp/tests/test_eia.py |
| 指定月份取对应年度数据 | tools/energy-mcp/tests/test_jodi.py |
| 默认流量单位与库存单位 | tools/energy-mcp/tests/test_jodi.py |
| 三种缺值标记 | tools/energy-mcp/tests/test_jodi.py |
| 最新期缺值不回退 | tools/energy-mcp/tests/test_jodi.py |
| 本年 404 回退上一年一次 | tools/energy-mcp/tests/test_server_network.py（server 层：本年 404→仅回退一次、显式 month 404 不回退且消息含年份、双 404→SOURCE_UNAVAILABLE、HTML 200 不回退）；解析层 test_jodi.py |
| 非法参数拒绝 | tools/energy-mcp/tests/test_jodi.py |
| 成功结果带完整元数据 | tools/energy-mcp/tests/test_eia.py + tools/energy-mcp/tests/test_jodi.py |
| HTML 冒充 CSV 被拒 | tools/energy-mcp/tests/test_http.py + test_server_network.py（HTML 不进缓存，同 key 下次重请求） |
| 不接受任意 URL | tools/energy-mcp/tests/test_server_schema.py |
| 缓存命中保留原元数据 | tools/energy-mcp/tests/test_http.py（另有：坏响应驱逐——test_server_network.py TestBadResponsesNotCached） |
| E2E 真实取数/错误语义 | tools/energy-mcp/tests/test_e2e_live.py |
| 配置行形态正确 | tools/energy-mcp/tests/test_preset_config.py |
| persona 不再虚报也不夸大 | tools/energy-mcp/tests/test_preset_config.py |
| 配置文档与验证留痕 | 人工：configuration.md dsh 节（2026-09-09 已增补）对照 tools/energy-mcp/README.md 口径一致；live smoke 留痕 evidence/live-smoke/live-smoke-2026-09-09.json |
| dsh 作用域加载确认 | 人工：**部分验证**——真实 Chrome 观察（evidence/live-smoke/dsh-ui-preset-observation-2026-09-09.md）：live preset 描述已生效、standard 无能源声明；**会话级工具目录未验**（不发 prompt 不触发模型下不可得），代码修复后挂载复验 |

- `cmd.exe /C "cd /d D:\project\Syntopica\tools\energy-mcp && uv run --frozen python -m unittest discover -s tests -v"` → 0 failed（**2026-09-09 修复后**实测 100 tests OK skipped=1，详见 review-fixes.md）
- `bash scripts/harness/doc-impact.sh verify openspec/changes/connect-dsh-energy-data-sources` → 2026-09-09 实测：通过（声明 configuration，文件 1 个）
- `bash scripts/harness/check-standards.sh --change connect-dsh-energy-data-sources` → 2026-09-09 实测：通过 139 / 失败 0（含 A-D/F/G）
- `openspec validate connect-dsh-energy-data-sources` → 2026-09-09 实测：Change is valid
- `diff config/dsh/presets/energy-research/agent.cordis.yml /mnt/c/Users/Admin/.dsh/.agent-presets/energy-research/agent.cordis.yml && diff config/dsh/presets/energy-research/preset.yml /mnt/c/Users/Admin/.dsh/.agent-presets/energy-research/preset.yml` → 零输出（2026-09-09 实测 IDENTICAL）
