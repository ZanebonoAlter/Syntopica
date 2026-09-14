# 聚焦 review 发现（6.4，2026-09-15，commandcode/deepseek-v4.1-flash 只读评审）

评审范围：8 个高风险文件精读 + 6 条 grep 不变量。原始结论如下，修复处置见文末。

## High

1. `GET /api/discovery/interests` 端点未注册（routes.go 无、前端 api/discovery.ts:261 在调）→ 兴趣记录页签 404 常驻。
2. `discovery_recall.go:524` 硬过滤 `kind='rsshub'` 且 JOIN rsshub_routes → 原生 RSS 候选结构性不可进推荐，「参与推荐」开关对原生条目失效。
3. `reader/handler/opml.go:133` 直写 `models.Feed{}`（不规范化、不 safefetch），RefreshFeed 走普通 httpclient → OPML 上传可触内网请求（本 change 之前即存在的历史洞，feed_create_service 文件头契约被绕过）。

## Medium

4. 模型切换无自愈：召回按全表 `model<>? OR dimension<>?` 计数不一致 → 换模型后每轮 configuration 卡死（4.6 遗留「旧模型行不清理」）。
5. refresh 查后插并发 + pending upsert 撞部分唯一索引 → 后到者整轮失败；request_key 撞唯一索引返回 500 而非复用运行。
6. service 单例共享 `lastPublish`/`run.Status` 无锁（数据竞争）。
7. 私网授权链路未接线：无写 `private_allowed` 的端点，check 恒传空 Options（design D9 的 access 确认端点未实现）。
8. `candidate_availability.status=broken` 不进召回资格过滤，卡片不显示 broken（catalog_extras CheckAvailability 为死代码）。
9. 同步 fail-closed 不完整：HTTP 200 + 合法 `{}` body → flattenNamespace 返回 0 条 → 全部路由误标 gone。
10. `feed_create_service.go:213` url.Parse 错误整体透传（含原文 URL/凭据）→ handler 400 回显。
11. `markRecommendationAccepted` 无状态条件，并发 accept 覆盖 accepted_feed_id。

## Low

12. 召回一致性校验对 model 为 NULL 的行恒通过。
13. safefetch 放行 6to4/Teredo 封装 IPv4（需 IPv6 路由才可利用）。
14. MatchBoardDetailed 未滤 NaN（pgvector 一般不接受 NaN，理论项）。
15. seed/lifecycle config 无 API 写入口（第一版无设置面板，设计内豁免，运维手写 SQL）。
16. `failRun` 用 fmt.Printf 而非 logging；重复注释行。
17. 同步内容变更 `Save(&row)` 把 broken 重置 unknown（当前无写 broken 者）。

## grep 不变量结论

| 命令 | 结论 |
| --- | --- |
| catalog_sync_service.go 候选 UPDATE 列 | 通过：仅 route status/route_id，人工四列零写入 |
| dismissed_at 新写入点 | 通过：仅 lifecycle 读判定，无新写入 |
| RouteParamOption 来源 | 通过：仅 manual/scraped，LLM 无写入口 |
| safefetch Proxy(nil) | 通过：恒定，无环境代理 |
| airouter LogCall | 通过：成败均落审计；configuration 失败落 run error_code（合设计） |
| /interests 路由 | 不通过（High 1） |

## 修复处置

- 修复批次一（review-fix）：High 1/2 + Medium 4/5/6/7/8/9/10/11 + Low 12/16/17。
- High 3（OPML SSRF）：**用户已拍板选最小修复**（走共享服务规范化+查重，保留原抓取；内网环境本就不可达）。归档前落地。
- Low 13/14/15：记录不修（理论性/设计内豁免），归档文档注明。
