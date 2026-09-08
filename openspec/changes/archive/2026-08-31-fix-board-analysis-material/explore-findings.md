
## 素材断供证据链（生产库实测）

生产库实测（2026-08-27）：
1. topic_lifeline_context 粒度分布：month 67 泳道 / year 67 泳道 / week 仅 2 泳道（8/24 手动测试痕迹）。scheduler_tasks 中 lifeline_weekly total_executions=0 从未执行（下次排期 8/31 03:00）；lifeline_monthly/yearly 7/13 各跑过一次全量 67。
2. 态势卡 facts_source 断供链：situation_cards.go laneFactsDigest 优先级 week→section指纹→description→none。生产 97% 泳道无 week，降级产物为 "[08-26] 泳道标题 (4篇)"——daily_report_sections 的 cluster_label 就是泳道名，同义反复零信息量。实际 board_interpret 调用（ai_call_logs id~1459300+，2026-08-27 00:07）prompt 尾部态势卡全是该形态。
3. month 档无人消费：D9 新鲜度门（freshness_gate.go）8/27 00:07 花 5 次 summarize_context LLM 调用补齐 topic 2/5/9/471 的 month 档到 08-26（1256~2483 字符），但装配器 laneFactsDigest 只查 ListTopicLifelineContextsByGranularity(topicID, "week")，不读 month。get_lane_detail（tool_registry.go → production_wiring.go dbLifelineReader.GetTopicLifeline）只查 board_persistent_topics + daily_report_sections + daily_report_threads，也到不了 topic_lifeline_context。整条链路（态势卡+下钻工具）没有任何路径访问历史记忆。
4. analyze 输出（response_snippet）非纯模板：thesis/四层论证/lane basis 结构齐全，但 basis note 细节（如"08-20 美国宣布史上最严厉制裁"）不在注入素材内——素材空泛导致 LLM 靠检索/脑补填肉，观感即"纯按模板输出"。
5. 前端 BoardEnrichmentPanel.vue（781 行）：顶部旧「话题选择条」与「聚焦分析」折叠区内第二个泳道下拉绑定同一 selectedTopicId（双 UI 控制同状态）；QAPanel 藏折叠区；底部单 tab「新闻背景」导航。
6. 模型路由：用户已手动配置（deepseek-v4-flash 挂 data_enrichment_analysis），不进本 change。
7. 测试盲区根因：situation_cards_test.go 自造 INSERT week 档测分支，从未对照生产数据形态（week 97% 缺失）；enrich_board 集成测试同样自造数据；UI 端到端 opencli 豁免。修复用例必须以「生产形态 fixture（month 在、week 缺）」为准。

<!-- pinned 2026-08-27T02:39:17Z -->
