# test-cases.md — connect-dsh-energy-data-sources（complex 档，用例先行）

测试单元 = Requirement 的用户故事，由本表串成完整故事。层级：U=纯函数单测（unittest）、H=HTTP 行为（mock 注入假响应）、E=端到端（真实联网，Windows uv 启动）、M=人工/live 验证。全部落 `tools/energy-mcp/tests/`（unittest，无新框架）。

## ⓪ 改契约了吗

无 MODIFIED/REMOVED Requirements（`dsh-research-preset` 主 spec 未同步，冲突在归档对账）——无旧测试资产需要反查，本表无「继承与调整」行。

## 主链路（每 Requirement 一个故事）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | mock EIA 样本，调 eia_wpsr_table1(stocks) | stocks 分区返回三类库存 | 3 条观测：711.064/424.460/286.604 量级，unit=MMbbl，当周+上周值+各自周结束日期，raw_value/source_row 齐全 | U | test_eia.py |
| 2 | 同源调 eia_wpsr_table1(supply) | supply 分区返回三项供需 | 13862/6770/4483（去千分位），unit=Mb/d，标签 "(1)…Production"/"(8) Imports"/"(12) Exports"，无 4周均/YTD 列值，不含 SPR 子项 | U | test_eia.py |
| 3 | en-dash 单元格 | 缺失标记映射 null | "– –"→null 非 0 非乱码；真实 "0.019"→0.019 | U | test_eia.py |
| 4 | 去掉必需表头列/行重复 | 结构漂移显式报错 | SCHEMA_CHANGED，不静默取错列 | U | test_eia.py |
| 5 | mock JODI 样本，US/production/2026-06 | 指定月份取对应年度数据 | value=13818.0、unit=KBD、assessment_code="1"、period/frequency/溯源/元数据 | U | test_jodi.py |
| 6 | 不传 unit 的 production 与 closing_stocks | 默认流量单位与库存单位 | production→KBD、closing_stocks→KBBL | U | test_jodi.py |
| 7 | OBS_VALUE ∈ {-, .., x} | 三种缺值标记 | value=null+raw_value+missing_reason；CLOSTLV/KBD='x' 不转 0 | U | test_jodi.py |
| 8 | month 未给，最新期缺值 | 最新期缺值不回退 | 返回最新期 null 观测，不取旧期有效值 | U | test_jodi.py |
| 9 | 本年 404 | 本年 404 回退上一年一次 | 用上一年文件，结果含 year_used/strategy；再 404→SOURCE_UNAVAILABLE | H | test_server_network.py（server 层 TestJodiFallbackPolicy：本年 404→仅 [2026,2025] 一次、显式 month 404 不回退且消息含实际年份、双 404 稳定 SOURCE_UNAVAILABLE、time 固定防跨年；HTML 200 不回退） |
| 10 | 非法 geo/month/closing_stocks+KBD | 非法参数拒绝 | INVALID_ARGUMENT，本地判定不发网络 | U | test_jodi.py |
| 11 | 成功返回顶层结构 | 成功结果带完整元数据 | source/url/retrieved_at/last_modified(可空)/sha256/observations 逐字段断言 | U | test_eia.py+test_jodi.py |
| 12 | HTTP 200 但 HTML 体 | HTML 冒充 CSV 被拒 | SOURCE_UNAVAILABLE/SCHEMA_CHANGED，不解析 | H | test_http.py + test_server_network.py（HTML 不进缓存：同 key 下次修好会重新请求） |
| 13 | TTL 内二次请求 | 缓存命中保留原元数据 | 无第二次网络调用，retrieved_at/sha256 与首次相同 | U | test_http.py |
| 14 | schema 无 URL 参数 | 不接受任意 URL | 工具 inputSchema 无 url/path 类字段 | U | test_server_schema.py |
| 15 | E2E：Windows uv 精确命令启动 | （集成故事） | initialize→tools/list 恰 2 工具→EIA/JODI 各一次 tools/call 真实非空含元数据 | E | test_e2e_live.py（skipUnless win） |
| 16 | E2E：非法参数 | （集成故事） | isError=true 含 INVALID_ARGUMENT | E | test_e2e_live.py |
| 17 | preset 行解析 | 配置行形态正确 | 恰 1 条 mcp-client，字段断言 | U | test_preset_config.py（仓库侧 yaml 文本解析） |
| 18 | persona/description 文本 | persona 不再虚报也不夸大 | 无旧声明；含新纪律句；无 STEO 已接/预算/沙箱宣称 | U | test_preset_config.py |
| 19 | dsh 新会话工具目录确认 | （live） | 目录见两工具；不可得则留未验记录 | M | evidence/live-smoke（未验；post-review 真联网 smoke 证据在 evidence/live-smoke/post-review/live-smoke-2026-09-09.json） |

## 变体走查（五组固定清单）

- **输入**：geo 大小写（"us"→拒，仅 2 大写字母）｜geo="USA"（3 字母拒）｜geo 含 "/" 或 "http"（拒，防 URL）｜month="2026-13"/"2026-00"/"2026-1"/"26-01"（拒）｜month 未来（拒）｜section/flow 非法枚举（拒）｜EIA 千分位 "13,862"（去逗号）→ 覆盖于步 2/10。
- **前置**：EIA 样本只含单分区（另一分区缺→只报该分区错或 SCHEMA_CHANGED，断言不崩溃）｜JODI 空 observations（geo 真实但该维度全缺→no_data 空数组）｜重复匹配行（SCHEMA_CHANGED）→ 步 4/10 扩展。
- **时间窗口**：month=2026-06 精确命中（含 4 位年）｜month 未给取 max(TIME_PERIOD)｜2026-01 vs 2026-12 排序｜EIA M/D/YY 解析（8/28/26→2026-08-28）｜年份边界 2002（下限收）与当前年+1（拒）→ 步 5/8/10。
- **幂等**：同参数重复调用（缓存命中同结果）｜TTL 过期后重新下载（mock 计数）｜错误响应后重试（不缓存错误，mock 两次网络）→ 步 13 扩展。
- **可用性（无 UI，前三检映射到错误反馈）**：误输入反馈（错误码+人读 message）｜空态（no_data 明确）｜错误态（网络/格式区分）→ 步 10/12 覆盖。
- 其余不适用项（UI 空态渲染、超长文本展示）：划除——本 change 无 Syntopica UI。

## 效果核对

效果不依赖断言外因素（确定性解析 + 实网一次取数验证），无需真库量化核对；E2E 断言限定「结构+非空+元数据」，不断言具体日期数值（源周更/月更会漂移）。

## 白盒附加（分支表 + 边界值）

| 分支 | 判据 | 边界值 | 测试 |
| --- | --- | --- | --- |
| EIA 分区判定 | 表头列数 2 列（STUB_1+…） vs 13 列（STUB_1,STUB_2+…） | 恰好 2/恰好 13/其他→SCHEMA_CHANGED | 步 1/2/4 |
| en-dash 检测 | cell.strip() 去引号后是否全为 0x96 序列 | "– –"/"–"/空串/“0.019” | 步 3 |
| EIA 日期列定位 | header[i] 匹配 M/D/YY 正则 | 8/28/26、12/31/99、1/1/26 | 步 1 |
| JODI unit 默认 | flow∈{production,imports,exports}→KBD；closing_stocks→KBBL | 传 KBD+stocks 拒；传 KBBL+production 允许（不换算按请求单位查） | 步 6/10 |
| JODI 404 回退 | status==404 且未回退过 | 首次 404→上一年；上一年 404→SOURCE_UNAVAILABLE；非 404 错误不回退 | 步 9 |
| 缓存过期 | now < expires_at | TTL 边界前后各一 | 步 13 |
| 大小上限 | 流式增量计数 ≤ 32MB（Content-Length 仅预拒、分块/无 header 也有界、超限立即终止不消费后续块） | 恰超 32MB 后首个 chunk 即中止；超长 Content-Length 零消费 | test_http.py TestStreamingBounds |
| HTML 检测 | body.lstrip() 以 "<" 或 "<!DOCTYPE" 开头 | 空体/HTML/CSV | 步 12 |

不适用划除：并发缓存竞态（单进程 stdio 串行调用，无并发声明）。
