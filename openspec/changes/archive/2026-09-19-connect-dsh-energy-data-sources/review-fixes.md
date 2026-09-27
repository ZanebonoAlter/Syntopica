# review-fixes.md — 独立评审六组缺陷修复与回归（connect-dsh-energy-data-sources）

> 修复写线程：仅动 `tools/energy-mcp/{sources.py,server.py,tests/**}` + 本文件 + evidence；未改 preset/key/settings、未装新依赖、未提交。所有单测离线 mock 无真实网络；真联网仅最后 smoke_live 一次（匿名官方 CSV，零模型请求）。
> 评审意见与源码不符处：评审称「Fetcher._single_request 后 resp.content 全量读取才检查 32MB」属实；但提及 `httpx.Limits(max_read=...)` 方案被实测否定（httpx 无此 read 级 API），改走 `client.stream`+`aiter_bytes` 增量计数。其余意见与源码一致，直接实现。

## 1. EIA 未知数值静默 null（parse_number/_record_eia_row 丢 reason）

- **复现**：`parse_number("n.a.")` → `(None, None)`，`_record_eia_row` 丢弃 reason 当正常 null 输出。
- **修复**：`sources.py:462` `_record_eia_row` 保留 current/prior missing reason（`EiaRow.current_missing/prior_missing`，`sources.py:352`）；value=None 且 reason=None → SCHEMA_CHANGED（当前值 `sources.py:469`、上周值 `sources.py:474`）；en-dash 对/空值仍 null 并输出 `missing_reason`/`prior_missing_reason`（`sources.py:539-541`）；真实 0/负数/千分位不变。
- **测试**：test_eia.py `TestMissingMarkers.test_unknown_markers_are_schema_changed`（n.a./--/em-dash）、`test_en_dash_prior_value_is_null_with_reason`、`test_empty_cell_is_null_with_reason`、`test_en_dash_stock_value_is_null`（原测试扩展断言 missing_reason）。

## 2. float(Decimal) 接受 NaN/Infinity/溢出 + csv.reader 超长单元格裸抛

- **复现**：`float(Decimal("nan"))`→nan、`float(Decimal("1e999"))`→OverflowError 未映射；>131072 字符单元格 → `csv.Error` 裸崩。
- **修复**：`sources.py:261` 新增 `_decimal_to_float`（`Decimal.is_finite()`+`float()` 捕获 OverflowError+`math.isfinite`，非有限一律 None）；`parse_number`（:284）与 `_jodi_cell`（:653）共用；`parse_jodi_primary`（:604）与 `parse_eia_table1` 的 csv.reader 迭代包 `csv.Error → SCHEMA_CHANGED`（未提高 field_size_limit）；UnicodeDecodeError 已存在映射保留（server.py `_http_guard`）。
- **测试**：test_eia.py `test_non_finite_and_overflow_cells_are_schema_changed`、`test_oversized_cell_is_schema_changed_not_crash`（200000 字符）、`TestParseNumber.test_non_finite_and_overflow_rejected_not_numeric`；test_jodi.py `test_non_finite_and_overflow_cells_are_schema_changed`、`test_oversized_cell_is_schema_changed_not_crash`；两处 `test_*_json_never_contains_nan_or_infinity`（`json.dumps(allow_nan=False)` 不抛 + 无 NaN/Infinity 字样 + 缺失值序列化为 `"value": null`）。

## 3. 2xx 坏响应缓存 15min（HTML 与 malformed CSV）

- **复现**：Fetcher 缓存所有 2xx；HTML 检测在 server 层、parse 失败也在 server 层，坏体已入缓存。
- **修复**：fetcher 级——`sources.py:170` 仅 `200<=status<300 且非 HTML` 才写缓存（HTML 不进缓存）；server 级——`server.py:137/140`（EIA）、`server.py:218/221`（JODI）在解码失败或 SCHEMA_CHANGED 时 `fetcher.invalidate(请求URL)` 驱逐缓存 key。**关键**：驱逐用请求 key（EIA_WPSR_TABLE1_URL / JODI 年模板），非 `doc.url`——EIA 官方同 host 签名重定向（实测 302→`/secure/wpsr/table1.csv?Policy=…Signature=…`）使 doc.url≠key。TTL/from_cache/retrieved_at/hash 语义不变。
- **测试**：test_http.py `test_html_doc_not_cached_then_refetched`（fetcher 级，calls==2）；test_server_network.py `TestBadResponsesNotCached` 三例（EIA HTML→第二次修好重请求；JODI malformed CSV→SCHEMA_CHANGED 后驱逐重取；JODI 不可解码体→驱逐重取），均断言网络调用次数==2 而非仅 assertRaises。

## 4. 32MB 检查前全量读入（资源限制缺陷）

- **复现**：`_single_request` 用 `client.get()` 先 `resp.content` 全量缓冲再检查。
- **修复**：`sources.py:174` `_single_request` 改 `client.stream`+`aiter_bytes` 增量计数，超 `MAX_RESPONSE_BYTES` 立即抛 SOURCE_UNAVAILABLE（响应随 with 退出关闭，不再消费后续块）；Content-Length 仅预拒（可提前拒绝但不依赖，分块/无 header 也有界）；每跳（初始 URL 与重定向目标）强制 `https` 且 port ∈ {None,443}，防同 host 降级 http；45s 单请求超时、同 host 有界重定向（MAX_REDIRECTS=3）保留；httpx/OSError → SOURCE_UNAVAILABLE。未采用评审建议的 `httpx.Limits(max_read=...)`（非正确 API）。测试注入点：`Fetcher.__init__(transport=…)` 可注入 MockTransport（默认 None=真实网络）。
- **测试**：test_http.py `TestStreamingBounds` 8 例——`test_streaming_success`、`test_over_limit_aborts_before_end_of_stream`（300×128KB 流，consumed < total 且 ≤ 上限+1 chunk）、`test_content_length_pre_reject_consumes_nothing`（consumed==0）、`test_chunked_no_content_length_still_bounded`、`test_streaming_redirect_same_host`、`test_redirect_downgrade_to_http_blocked`、`test_only_https_default_port_443`（http/port80/8443 请求前即拒）、`test_streaming_timeout_maps_to_source_unavailable`（MockTransport handler 抛 ReadTimeout → SOURCE_UNAVAILABLE）。`test_timeout_bubbles_as_source_unavailable` 由 `assertRaises(Exception)` 强化为 `assertRaises(httpx.ConnectTimeout)`。

## 5. JODI 同维度同 period 双观测静默返回（100/999）

- **复现**：`jodi_result` 的 selected 可含两条同 (geo,product,flow,unit,period) 记录。
- **修复**：`sources.py:711` `jodi_result` 按全维度 key 分组：完全一致（value/raw_value/missing_reason/assessment_code 全等）合并为一条观测、`source_row` 取首行并追加 `source_rows` 全行溯源；任一语义冲突 → SCHEMA_CHANGED（消息含两源行号）；显式 month 与 latest 模式都检测；不同 unit/period 因 key 含 unit+period 且 selection 先行过滤，绝不误合并（输出按 key 排序确定）。
- **测试**：test_jodi.py `TestDuplicates` 7 例——identical 合并+`source_rows=[2,3]`、冲突值/冲突 assessment/冲突缺失标记→SCHEMA_CHANGED、identical 缺失合并、不同 unit 不合并、不同 period latest 不合并、latest 模式冲突亦检测。

## 6. test-cases 步 9 的 404 落点不实 + server 层 timeout 覆盖缺失

- **证据**：旧 test_jodi.py 无任何 server 分支（纯 U 层 jodi_result/parse 测试），404 回退逻辑实际在 server.py 工具函数内，未被测试触达；旧 timeout 测试用 `assertRaises(Exception)` 弱断言且非 server 层。
- **修复**：新增 `tests/test_server_network.py`（10 例，全部 mock `server._get_jodi` 或 `fetcher._single_request`，用现有 `_tool_fn` 兼容方式 + 官方 ToolError）：
  - `TestJodiFallbackPolicy`：`test_current_year_404_falls_back_once`（calls==[2026,2025] 恰一次、year_used=2025、strategy 含 previous-year）、`test_current_year_ok_no_fallback_request`、`test_explicit_month_404_no_fallback_year_in_message`（calls==[2025]、消息含 "2025"）、`test_both_years_404_source_unavailable`、`test_html_200_does_not_fallback`（仅 404 触发回退，HTML 200 不回退）；时间用 `_FixedNow`（subclass 钉 2026-09-09）防跨年脆弱。
  - `TestServerTimeoutMapping`：`test_jodi_read_timeout_is_source_unavailable`、`test_eia_connect_timeout_is_source_unavailable`（真 server 层，断言 ToolError 含 [SOURCE_UNAVAILABLE]，非弱断言）。
  - `TestBadResponsesNotCached`：见第 3 组三例。

## 回归结果

- 命令：`cmd.exe /C "cd /d D:\project\Syntopica\tools\energy-mcp && uv run --frozen python -m unittest discover -s tests"`
- **100 tests，OK (skipped=1)**（修复前 63 → 修复后 100，+37 新用例：test_server_network.py 10 + test_eia.py 7 + test_jodi.py 11 + test_http.py 9；skip 为 live E2E 默认 opt-in）。原 63 用例零删除零缩水。
- 中间态：首轮新增用例 5 失败（测试自身缺陷：CountingStream 包装错误×2、fixture 记录构造×2、HTML 测试绕过真实 _get_jodi×1）→ 修测试后全绿；核心实现未再改动。

## 真联网 smoke（修复后复验）

- 命令（Windows uv，临时 importlib 驱动把 EVIDENCE 重定向到 post-review 子目录，**旧证据 `evidence/live-smoke/live-smoke-2026-09-09.json` 原路径原内容未动**，驱动脚本已删除）：`uv run --frozen python _smoke_driver.py` → 自动化 `import smoke_live; smoke_live.EVIDENCE=…/post-review; asyncio.run(smoke_live.main())`
- 输出：`evidence/live-smoke/post-review/live-smoke-2026-09-09.json`（2026-09-09 11:00/11:01 UTC+8 实测）
- 校验（读返回 JSON 非仅退出码）：tools_listed 恰 `["eia_wpsr_table1","jodi_oil_primary"]`；EIA stocks 3 观测/MMbbl/period 2026-08-28/711.064（商业 424.460、SPR 286.604）、supply 3 观测/Mb/d/产量 13862（去千分位 13,862）；JODI production 1 观测/KBD/2026-06/13818.0/source_row 109464、closing_stocks 1 观测/KBBL/2026-06/668715.0；四结果 url 均 https 官方 host、source_sha256 64 字符、retrieved_at 真实;eia_supply 与 jodi_closing_stocks 为 TTL 内缓存命中（from_cache=true，原元数据保留）；error_case isError=true 含 [INVALID_ARGUMENT]；全部 value 有限、JSON 无 NaN/Infinity。EIA 302 签名重定向经流式路径真实走通（佐证第 3/4 组修复）。实时 JODI 数据无同维度重复行，故 `source_rows` 字段未出现（去重行为由单测覆盖，合并键仅在有重复时输出）。