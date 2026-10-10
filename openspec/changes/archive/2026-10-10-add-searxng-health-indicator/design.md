# Design — add-searxng-health-indicator

## Context

页边注联网链路（预检 → LLM 生成 query → SearXNG 并行搜索 → 结果注入 prompt）按「联网是增益不是依赖」设计为全程静默降级，唯一观测面是 `backend-go/logs/app.log` 的 WARN。2026-10-01~10-03 Clash 国外出口失效事故中该链路静默失效两天无任何用户可见信号（诊断存档 `docs/research/margin-note-websearch-silent-degrade/`）。本 change 补两处观测面，不改链路行为本身。

关键现状约束：`airouter.ChatRequest` 无 tools 字段（回答调用是单次 chat）；`searxng_config` 每次现读（界面改即时生效，探测端点同样必须现读）；SearXNG 引擎故障形态是 HTTP 200 + `results:[]` + `unresponsive_engines` 非空（进程探活无法发现）。

## Goals / Non-Goals

Goals：面板可主动探测四态健康；问答轮落库并透出 `web_search_status`；`empty` 态卡片可见标记。

Non-Goals：不改搜索链路与静默降级行为；不做后台轮询/定时探活；不做回答内容层面的「未联网」声明注入（标记归 UI 层）；不动 `web_sources` 白名单校验。

## Decisions

**D1 探测=真实探针搜索，非进程探活。** 复用 `marginNoteSearxngEndpoint` 配置解析 + `searxng.Search` 客户端，发固定探针 query（如 `connectivity probe`）判活。理由：事故形态是「进程活、引擎死」，`/healthz` 类端点无法暴露；同客户端同配置=同源判活，避免「面板绿了链路还红」。代价：一次真实引擎调用（~秒级，用户手动触发可接受）。

**D2 四态判定边界。** `disabled`=预检不过（配置解析与 QA 链路完全同一函数）；`unreachable`=HTTP 层错误/超时（dial refused、8s 超时）；`degraded`=HTTP 200 但解析后 results 为空；`ok`=results 非空。返回 `{status, latency_ms, detail}`，detail 含人话摘要（如引擎无响应数），不透传原始 JSON。

**D3 状态采集点=搜索函数签名扩展。** `searchWebForMarginNoteWithQueryGen` 返回值从 `[]searxng.Result` 扩为 `(results []searxng.Result, status string)`：预检不过→`disabled`；有合并结果→`ok`；链路启用但全部失败/空→`empty`。`AskMarginNote` 将 status 写入 `MarginNoteQAResult` 并落库。最小侵入，单一采集点（与降级树同函数，不会漏分支）。

**D4 落库三态，面板四态。** 落库不区分 `unreachable`/`degraded`——对读者语义相同（「联网没帮上」），合并为 `empty`；面板才需要细分（排障分流：查 endpoint vs 查引擎/代理）。枚举值即字符串常量，不用数字码（可读性、日志友好）。

**D5 迁移与旧行语义。** `ALTER TABLE annotation_qas ADD COLUMN web_search_status text NULL`（非破坏；NULL=历史数据，前端不渲染标记，不回填）。GORM 模型加 `WebSearchStatus *string`（指针区分 NULL 与空串）。

**D6 手动触发、不轮询。** 单用户系统，后台轮询会持续打真实引擎（配额/延迟），面板「测试连接」+ 保存后自动一次即可。探测请求带 8s 超时（对齐 `searxng.Search` 客户端），handler 层再包一层 context 超时防悬挂。

## Risks / Trade-offs

- [探测打真实引擎，慢查询拖 UI] → 手动触发 + 按钮 loading/disabled 防重复；8s 超时上限
- [SearXNG 返回体异常（无 results 字段/非 JSON）] → 解析失败归 `unreachable`（保守判定：不能确认可达）
- [status 与回答引用不一致的误解（ok 但未引用）] → 标记语义锁定「搜索有无结果」而非「回答是否引用」，spec Scenario 已显式覆盖
- [QA 响应加字段破坏旧前端] → 加法字段（`omitempty` 语义上不用——非空即有义），旧消费方忽略未知字段

## Migration Plan

1. 部署即自动迁移（ALTER ADD COLUMN，可逆：DROP COLUMN 无数据损失——列仅新增数据）
2. 回滚：后端回退版本即可，列残留无害；前端标记随 status 缺失自然不渲染

## Open Questions

无（探针 query 文案、标记视觉细节可在实现微调，不影响契约）。
