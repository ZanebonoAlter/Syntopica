
## 健康门判定机制与路由绑定实况

健康门判定：backend-go/internal/platform/aihealth/aihealth.go 末尾 runProbeLocked —— healthy = hasReachableCapability(entries,"embedding") && hasReachableCapabilityOtherThan(entries,"embedding")（宽松或运算，只探每条路由的主 provider（priority 最先的启用链路），兜底完全不探；capability 分类而非 model_kind）。探测 = airouter.TestConnection GET {base_url}/models，status>=400 即判不可达（test_connection.go:55）。applyProbeVerdict 有 degradeFailures=2 去抖。analysispause/gate.go：有效暂停 = UserPaused() || !aihealth.Healthy()。

路由绑定实况（2026-09-28，库 ai_route_providers）：topic_tagging 主=qwen(10.11.12.111:8080) 无兜底（用户刚删掉 deepseek p2 链路，唯一变化）；digest_polish 与 data_enrichment_analysis 主=deepseek-flash(云端 api.deepseek.com) 兜底=qwen——这两条云端主力使"本地 qwen 全挂时门仍开"（场景矩阵结论）。8 条 enabled 路由：summary/topic_tagging/digest_polish/embedding/data_enrichment_news/data_enrichment_analysis/feed_discovery/open_notebook。

用户决策（2026-09-28）：健康门改为「打标路由（topic_tagging 主 provider）必须可达」为硬条件（理由：打标主要影响文章入库）。已开 change ai-health-gate-tagging-required。

重要归因事实：10.11.12.111 不在 no_proxy（仅 localhost/127/::1），局域网 AI 主机流量走 127.0.0.1:7890 代理；目标端口死时代理返回 HTTP 502 + 空 body（实测 2.64s），openai_compatible.go:187 json.Unmarshal("") → parse_error（2026-09-28 上午 675 次假 parse_error 全是这个，均值 3082ms）——连接失败被伪装成解析失败，日志归因需警惕；TestConnection 对 502 正确判不可达，探测未被骗。

**引用**：backend-go/internal/platform/aihealth/aihealth.go:runProbeLocked、backend-go/internal/platform/aihealth/aihealth.go:applyProbeVerdict、backend-go/internal/platform/airouter/test_connection.go:TestConnection、backend-go/internal/platform/analysispause/gate.go:IsPaused、backend-go/internal/platform/airouter/openai_compatible.go:187

<!-- pinned 2026-09-28T03:37:17Z -->
