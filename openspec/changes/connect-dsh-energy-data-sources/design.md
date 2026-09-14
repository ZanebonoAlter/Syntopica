## Context

主线程已定架构（proposal + explore-findings/runtime-findings）：Windows 上 node24/uv0.7.9/Py3.14.3 可用；dsh-mcp-client 契约已精读（stdio/stdio_client，env scrub，工具名 `mcp__energy_data__<raw>`）；两源样本已落 evidence/。本设计只记录实现层决策。

## Goals / Non-Goals

Goals：两个只读工具可用且可溯源；dsh 新会话真实挂载；测试先行且端到端复刻精确启动命令。
Non-Goals：STEO/其他源、跨源合并、单位换算、仪表盘、数据库、预测、任意 URL 抓取、Go/Nuxt 改动。

## Decisions

1. **官方 `mcp` SDK FastMCP**，不手写 JSON-RPC。备选（手写协议）被否：维护成本与协议漂移风险。Python 版本：`requires-python = ">=3.11"`，用现有 3.14.3 解释器；若 `mcp` SDK 对 3.14 有上限装不上，报告而非升级工具链。
2. **模块布局**：`sources.py` 单文件承载 HTTP+解析（两源各一个 class + 共享 fetch 层），`server.py` 薄封装工具定义。拆包（`energy_sources/{eia,jodi,http}.py`）留作文件过大时的重构项；当前两源解析合计预计 <600 行，单文件更利于 review。
3. **HTTP 用 httpx**（mcp SDK 已依赖 anyio，httpx 异步兼容 FastMCP 事件循环）。备选 stdlib urllib（无超细粒度超时控制、重定向需手控）。
4. **数值处理**：csv.reader + Decimal 解析；输出 JSON 时 Decimal→float 或字符串？决策：value 输出为 float（json 序列化安全，能源数据量级无 float 精度问题），raw_value 保留原始字符串。
5. **CP1252 解码**：bytes.decode('cp1252') 后 csv.reader（内置 reader 不做编码层处理，正确处理引号/千分位）。
6. **EIA 列定位**：按表头日期列索引定位当周/上周（不按位置硬编码）；日期解析 M/D/YY。表头漂移（日期列数变化/重复列去重失败）→ SCHEMA_CHANGED。
7. **JODI 月份策略**：读取整个 CSV 时收集匹配维度全部行，取 max(TIME_PERIOD)；404 回退上一年仅一次并在 top-level 注明 `year_used`/`year_strategy`。
8. **缓存**：模块级 dict{cache_key: (expires_at, payload)}，TTL 900s，仅缓存成功响应；key=source+year（EIA 无年份维度，key=eia:table1）。
9. **测试**：unittest + unittest.mock（patch HTTP 层）；端到端测试用 `mcp.client.stdio.stdio_client` + `ClientSession` 以子进程方式启动 `uv run --frozen --no-sync --project <win path> python <win path>/server.py`——该测试仅在 Windows 侧可跑（uv 在 Windows PATH），测试文件标注 skipUnless(Windows)。fixture 从 evidence 样本裁剪。
10. **启动形态**（dsh 配置行）：`command: uv`，`args: [run, --frozen, --no-sync, --project, D:/project/Syntopica/tools/energy-mcp, python, D:/project/Syntopica/tools/energy-mcp/server.py]`，`cwd: D:/project/Syntopica/tools/energy-mcp`。正斜杠（bash→cmd 反斜杠被吞）。
11. **stderr 日志**：logging 配置 StreamHandler(stderr)；FastMCP 默认即如此，不 print 到 stdout。

## Risks / Trade-offs

- [mcp SDK 不支持 Python 3.14] → `uv sync` 阶段即失败，报告主线程（用户授权不擅自升级工具链；uv 可自动取托管 Python，若发生则记录实际解释器版本）。
- [EIA 结构周变（列数/标签漂移）] → SCHEMA_CHANGED 显式失败，persona 指示承认缺口；不做模糊容错匹配。
- [JODI 年度文件滞后 1.5~2 月] → 工具返回注明 TIME_PERIOD 是数据期、retrieved_at 是抓取时刻，不称实时。
- [32MB/45s 预算误伤] → JODI 2026 实测 4.76MB，余量充足；超限报 SOURCE_UNAVAILABLE。
- [quota/额度] → 本 change 不派子代理、无模型调用，无计费。

## Migration Plan

1. artifacts → apply → 实现 → Windows 侧 uv sync + 测试全绿。
2. preset 两文件改源 → diff live 一致性 → 覆盖前旧副本（非敏感）存 evidence/preset-backup/ → 复制到 live。
3. dsh 新建空白验证会话确认工具挂载。
4. 回退：还原 live 两文件（或删除 mcp-client 行重新部署）即恢复旧工具面；tools/energy-mcp 留在仓库无副作用。

## Open Questions

（无——口径均已在 explore-findings 实测定案。）
