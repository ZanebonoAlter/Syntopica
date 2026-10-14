# Test Cases

复杂档（complexity: complex）白盒附加见文末。主链路故事：用户配置 FRED key → 设置页见七源 → 信号工作台点「深入分析」→ 研究 loop 以七源白名单调 yahoo_chart/gdelt_doc（价格与事件源当日可得）→ compose「数据可用性」行含日度期值 → facts 段「数据截至 2026-10-01」→ 阅读视图 as-of 行显示最大期，读者看出数据已到昨天而非两个月前。

## 主链路表

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | GET /api/datasources | research-data-sources:目录查询返回全部已注册源 | 七源全列，新源频度 daily/滞后 T+0~小时级 | handler | datasources handler 测试 |
| 2 | POST /api/settings/fred 保存 key 后再查目录 | research-data-sources:FRED key 界面配置-界面保存即时生效 | fred 翻 enabled 无重启；GET 脱敏（末 4 位） | handler | FredConfig 测试 |
| 3 | fred key 未配置时调 fred_series | research-data-sources:FRED 取数语义-key 未配置 | SOURCE_UNAVAILABLE 配置缺失型，不发上游请求 | 单元 | Fred 取数器测试 |
| 4 | 研究 loop 调 yahoo_chart symbol=CL=F range=1mo | research-data-sources:Yahoo 语义-合法 symbol | 逐日 OHLC 点+货币 USD，最新点近当前 | 单元(mock httptest) | Yahoo 取数器测试 |
| 5 | 研究 loop 调 gdelt_doc query="iran oil" mode=timelinevol | research-data-sources:GDELT 语义-热度时间线 | 带时间戳点序列，最新近当前 | 单元(mock) | Gdelt 取数器测试 |
| 6 | 研究 loop 调白名单外工具（web_search） | board-signal-reports:四十轮研究-重复与非法 | 拦截+反馈（文案按七源动态生成），计轮不执行 | 单元 | SignalResearch 拦截测试 |
| 7 | 账本含 yahoo(2026-10-01)+jodi(2026-07) 时 compose | board-signal-reports:时效声明-日度源参与 | 「数据可用性」两期并排；facts「数据截至 2026-10-01」日粒度比对通过 | 单元 | AsOf/Validate 测试 |
| 8 | 既有增强/QA/调查运行 | research-data-sources:工具面-注入后不变 | 既有六件套工具面不变，仅注册不等于授权 | 单元(既有) | 白名单回归 |
| 9 | 设置页渲染七源卡+fred 无 key | ui-design:受影响状态 | 七卡渲染；fred disabled 带原因 | 组件(Vitest) | settings 测试 |
| 10 | 真实触发一次含日度源的研究 | 主链路效果 | as-of 行「数据截至 2026-10-XX」（日度形态） | 人工 | 见效果核对 |

## 变体走查

| 组 | 变体 | 答案 | 落点 |
| --- | --- | --- | --- |
| 输入 | symbol 空/枚举外（AAPL） | INVALID_ARGUMENT 发网前拒，detail 指枚举 | Yahoo 测试 |
| 输入 | series_id 小写/特殊字符/65 位 | INVALID_ARGUMENT `^[A-Z0-9_]{1,64}$` | Fred 测试 |
| 输入 | query 含控制字符/121 字符；mode=artlist | INVALID_ARGUMENT 发网前拒 | Gdelt 测试 |
| 输入 | 纯空白 query | 拒（≤120 校验前置非空） | Gdelt 测试 |
| 前置 | FRED key 三链（DB/env/yaml/全空） | DB>env>yaml；全空 disabled+配置缺失错误 | FredConfig 测试 |
| 前置 | enabled=false 的 DB key | 跳 DB 走兜底链 | FredConfig 测试 |
| 时间窗口 | range=1y 边界；timespan=90d；FRED start/end 恰 10 年/超 1 天 | 枚举值过；窗口>10 年拒 | 三源测试 |
| 时间窗口 | 日度观测恰=cutoff 当日 | 期结束≤cutoff 含当日，通过（当日收盘属既成事实） | 包装层测试 |
| 幂等 | 同参重复取数 | TTL 命中保留原 retrieved_at；错误不缓存 | 缓存测试（既有模式复制） |
| 幂等 | 重复 tool+args 调用 | 既有 dedupKeyFor 拦截不变 | 既有回归 |
| 可用性 | yahoo 上游结构漂移 | SCHEMA_CHANGED+驱逐缓存，不模糊容错 | Yahoo 测试 |
| 可用性 | gdelt 零结果 | 空序列+零结果标注，不编造 | Gdelt 测试 |
| 可用性 | probe 失败 | error_code 可区分（配置缺失/网络/漂移） | handler 测试 |
| 可用性 | 设置页空态/加载态 | 目录恒七源无空态；probe 按钮加载不阻塞他卡 | 组件测试 |

不适用划除：单 token/大小写（非文本解析域）；跨窗口归一化（无窗口归一化语义）；并发（无新增线程安全声明，缓存沿既有并发语义）。

## 效果核对

- 触发原因：报告时效从「JODI 月度下限」抬到「日度」依赖新源真实可用。
- 方法：`DATASOURCES_LIVE=1 go test ./internal/datasources/sources/ -run Live -count=1`（yahoo/gdelt 匿名实测）+ 部署后用户显式触发一次含价格候选的研究。
- 量化记录（2026-10-02 实测，树莓派）：**yahoo live PASS：CL=F 1mo 22 行，最新期 2026-10-02（=当日，T+0 精确成立），currency=USD，耗时 0.78s**；eia/jodi/wdi 匿名 PASS 不回退；gdelt 受免费档限流（官方文案 1 次/5 秒，**实测更严**：滑动窗口内连续第二次请求即 429；smoke 节流 6s 后仍 429 时 skip 留痕）；直连取证（curl + 与取数器同款 UA）：`query="iran oil" mode=timelinevol timespan=7d` → **HTTP 200，1 series × 120 小时点，最新时间戳 2026-10-02T080000Z（小时级准实时确认），响应结构与解析器契约一致（timeline[].data[].date/value）**；限流/代理分流异常时生产语义 = 显式 SOURCE_UNAVAILABLE 记 gap 不阻断（研究 loop 调用稀疏 + TTL 900s 同参去重，实际撞限流概率低）；comtrade/fred 无 key skip（key 路径单测已盖）。
- 新报告 as-of 行对比：待用户配置 FRED key 并显式触发一次含价格候选的研究后，与 2026-09-23 报告（数据截至 2026-07，滞后 2 个月）对比——预期新报告「数据截至 2026-10-XX」日度形态。人工：信号工作台点「深入分析」后读报告头 as-of 行。
- 若 yahoo 匿名接口在实现期已漂移：按 Risks 走 FRED 备份路径并在此记录（本次实测未漂移，结论无需变更）。

## 继承与调整（⓪ 契约变更）

两 capability 均有 M/R 变更，`bash scripts/harness/test-assets.sh <capability>` 反查清单（research-data-sources 11 行映射 + board-signal-reports 58 行映射，2026-10-02 实跑）回填下表；受影响的锚点逐行处置完毕，其余映射行行为不变不动：

| 旧 Scenario/断言 | 处置 | 旧测试 | 动作 |
| --- | --- | --- | --- |
| 目录返回四源（research-data-sources） | 改七源 | datasources/handler_test.go TestHandlerListCatalog | 更新断言（want 4→7）✅ |
| 目录 seed 清单与幂等行数四源 | 改七源 | datasources/repository_test.go TestUpsertCatalogSeedsAllSources | want 列表七源升序 ✅ |
| un_comtrade key 缺失独占 disabled 场景 | 扩 fred 同语义 | repository_test TestUpsertCatalogKeyMissingDisables*、init_test TestInitEmptyKey* | 两测试扩 fred（各自 FRED_API_KEY 原因）+ 目录七源断言 ✅ |
| key present 只 enable comtrade | 扩双 key | repository_test TestUpsertCatalogKeyPresentEnables* | resolver 同时给两 key，全源 enabled ✅ |
| allowedTools 仅四工具（research-data-sources + board-signal-reports） | 改七枚举 | signal_research_test TestSignalResearch_AllowedToolsPinnedAndLegacyUnchanged、wiring/tools_test TestResearchToolsToolSurfaceUnchanged | 两钉子改七源/七工具（构造顺序同步 toolCatalogCodes）✅ |
| CheckCall 反馈文案「只能调用四源工具」（board-signal-reports） | 动态生成 | signal_research_test 拦截用例 | 文案按 len(白名单) 动态；反馈行断言更新 ✅ |
| 工具描述硬限制「无价格/裂解价差」（源能力声明） | 改能力边界表述 | wiring/tools_test TestBuildToolsDescriptionsCarryCatalogMetadata、capability_test TestSourceCapabilityTextDerivesFromCatalog | 新口径「价格只有 yahoo/fred、事件层只有 gdelt、量类不含价格」断言 ✅ |
| validateSignalFactsAsOf 纯月粒度比对 | 加日度分支 | signal_compose_test ⑤⑥②等 + 新增⑩-⑬ | 分形匹配（日度精确/月度月粒度）；六处 research fixture as-of 改精确日（QuestionRequired/CutoffFilter/Sixty/CalculateAction/ProgressPerRound/NoCoverageStreakResets）✅ |
| 空手源早停「四源皆无」终局 | 七源终局 | signal_research_test TestSignalResearch_NoCoverageFinaleOnceAfterAll* | 七源 21 轮空手 + 终局文案动态（「7 个数据源」）✅ |
| live smoke 四源 | 扩七源 | sources/live_smoke_test.go | yahoo/gdelt 匿名子测试 + fred key-gated；gdelt 429 限流特判 skip ✅ |
| host 白名单回归仅 EIA 归档 | 扩三 host | datasources/catalog_allowlist_test.go | 三新源全部上游 URL 过真 Fetcher gate ✅ |
| resolver 单链（COMTRADE） | 双链 | wiring/resolver_test.go TestComtradeKeyResolverChain | 改名 TestDataSourcesKeyResolverChain + FRED 链独立/互不干扰用例 ✅ |
| 探索期备注「四源注释」（explore-findings） | 勘误 | 不属测试资产 | tasks.md 1.2 行内勘误（symbol 17→15 计数笔误）✅ |

## 白盒附加

| 分支 | 边界值 | 用例 |
| --- | --- | --- |
| Yahoo 解析 | timestamp/quote 长度不齐；quote 数组 null close；meta.currency 缺失；空 result 数组 | 四个负向+透传断言 |
| FRED 解析 | `.` 值；units/frequency 字段缺失；观测窗口空（无行）；start>end | null 映射/透传/错误 |
| GDELT 解析 | data 点 value 缺失；series 空数组；时间戳不可解析 | 如实 null/空序列/SCHEMA_CHANGED |
| as-of 日粒度 | 可用性行仅月度（旧路径）；仅日度；混合；模型写跨粒度编造期（月度报告写日度值） | 旧路径字节不变+三分支+一拒绝 |
| symbol 枚举 | 全 15 个逐一过校验；枚举外 1 个 | 全枚举表驱动 |

不适用划除：并发缓存击穿（沿既有缓存实现，未改动）；重定向链（fetch.go 既有逻辑未改动）。
