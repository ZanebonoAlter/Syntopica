
## 当前基线与工具复用边界（propose 核对）

工作区已有 redesign-enrichment-research-workbench 的未归档实现（schema3平面块、候选忽略与主从工作台），本change依赖其完成/同步，不覆盖这些脏文件。主spec仍是schema2，写delta须把前置change相关MODIFIED条文先合并成有效基线。signal_detect.go:SignalDetector 无registry，score>=6整数、最多两次识别；当前 prompt 为突变/反常/初现/背离。signal_research.go:buildSignalResearchRegistry 只授权7数据源，ResearchCandidate仅读发现InputSnapshot，40轮/40执行，150分钟，progress保存账本。web_search.go已有WebSearcher/BochaWebSearcher，summary:false，默认10条，wire.go动态DB>env>config取Bocha配置，已生产接线（文件顶部no-real-backend注释过时）；工具返回title/url/snippet无可靠发布日期。fetch_page.go已有PageFetcher和ReaderPageFetcher，读取readability正文截4000rune，但ReadabilityCrawler.ScrapePage直接readability.FromURL，不经safefetch，不能把当前实现当作安全任意URL抓取。safefetch已有逐跳DNS+固定IP拨号、禁私网/metadata、2MiB/10s/3跳限制，可供signal专用安全PageFetcher复用后用readability解析bytes，不宜全局改旧调查抓取行为。现有board_investigation也有搜索/页面工具，但review/动态授权/多泳道流程不应整套搬入signal。research-data-sources主spec显式排除web_search，必须同步MODIFIED工具授权Requirement，不只改报告域。本轮两次子线程派发分别失败：explore档未被工具发现、scout后台缺宿主；均未启动子任务，主线程完成规划，未绕CLI。已跑test-assets.sh board-signal-reports和research-data-sources（输出/tmp/question-research-test-assets-*.txt）。

**引用**：backend-go/internal/dataenrichment/service/signal_detect.go、backend-go/internal/dataenrichment/service/signal_research.go、backend-go/internal/dataenrichment/service/web_search.go、backend-go/internal/dataenrichment/service/fetch_page.go、backend-go/internal/reader/service/readability_crawler.go、backend-go/internal/platform/safefetch/safefetch.go、openspec/specs/research-data-sources/spec.md

<!-- pinned 2026-10-19T08:45:20Z -->
