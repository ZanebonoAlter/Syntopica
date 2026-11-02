# G3 父准入复核：读取方案采纳，接缝/体量待收口

## 已核实（不是浏览器验收）

父只读GET `/api/semantic-boards/246030/section-timeline?days=0`：14307B、16 sections、163ms；1974同端点：693593B、702 sections、1364ms。响应真实具备id/report_id/period_date/cluster_label/thread_count/persistent_topic_id；nested persistent_topic为当前brief。API适配使用历史ID、不映当前status/title的方向正确；不等完整S11-C交互已验。

采纳G3固定days=0：既有后端正数上限90，不能用计算365+停更差额的正数假装全年覆盖。使用已有全部历史口径、板块单份懒缓存、拖轴本地裁切，不扩后端/参数。当前两个真实板块数据量如上；不是所有未来板块体积保证。

## G3-M1：阅读重试绕过索引刷新（待修）

OtherTopicReading.retryReading直接ensureReportDay(date)，refreshReading仅ensureReportDay(date,{force:true})。当同日日报重建导致reportId改变/旧详情缓存时，G1接口明确须先ensureReportIndex({force:true})再读当日；否则刷新仍指旧报告/分区，可能反复缺失或读旧缓存。G3组件应发受控重试/刷新意图给G4统一编排，不自建读取器；G4集成测试必须核顺序。正文和相关文章既有注入store复用不改。

## G3-M2：settled仅看view.status且setup立即触发（待修）

当前settledKey watcher immediate，未复用既有post-mount/DOM就绪协议；观察hook有旧dataset时即使loading=true也给ready view。强刷观察数据会改变节点带高度，不能在依赖尚在刷新时就给返回恢复终态；初始ready也不可在DOM挂载前发。需按既有useReadingSettledSignal协议最小复用/适配，等待正确date/section/topic上下文+报告终态+观察依赖实际非loading+DOM更新完成后发；卸载/切上下文的旧排队信号不得发，用户展开相关文章不进入首读等待。不主动滚动。

## G3-S1：超体量，不是可忽略的纯风格（待修）

standard/frontend/code-style.md §8：单文件>500行/~15KB应按内聚行为拆；子组件props接收数据，不深依赖父composable内部状态。OtherTopicReading530、BoardNarrowSummary795行需拆为观察带/日期条/受控组等内聚子组件及必要小行为；不能压行、删注释或纯机械分片凑数。

另G1 useLaneDayReading513/约23KB、G2 ObservationMatrixGrid646等既有本轮触达文件与大测试也需在独立结构收口包核规范，不能给G3顺带改其他包。这是未完成的标准化工作，不以当前单测绿或样式较多豁免；G4前需父统筹，不默认为已处理。

## 边界

本次G3子自测4文件38例+相邻Shell38与TS eslint绿为真实历史，保留不覆盖；未声称覆盖本复核新角。8C.4暂不最终验收，下一全新短线程只修G3-M1/M2并按标准拆本包组件、补回归。新API产品固定0，不应对正数请求仍标coverage all；统一ApiResponse包装按标准核实，禁止为此改共享旧API。G1/G2/后端/foreign/原型不动。

## 关闭依据（2026-10-29 全新短线程实跑，原发现不删不改）

以上「已核实 / G3-M1 / G3-M2 / G3-S1 / 边界」保持原文作为历史发现；关闭证据见新页 [`matrix-day-others-g3-closure.md`](matrix-day-others-g3-closure.md)，摘要：

- **M1 已修**：`OtherTopicReading` 新增受控 emit `retryReport({date,sectionId})`，错误态重试与缺分区刷新都只发意图，不再直读 store；G4 必须按 `ensureReportIndex({force:true})` → `ensureReportDay(date,{force:true})` 顺序编排（本包未验真实网络顺序）。红：`8 failed | 8 passed`；绿：本包目标 4 files/49 tests。
- **M2 已修**：新增 G3 专有 `useOtherReadingSettled`，等挂载 + nextTick + 报告终态 + 观察依赖真实非刷新终态（`refreshing` 门）后才发；换上下文/卸载旧排队作废；同 context 刷新终态可再发。共享 `useReadingSettledSignal` 未改。观察 hook 增 `view.refreshing` 与 `invalidate()`（懒失效、不 GET）。
- **S1 已拆**：`OtherTopicReading` 530→366 行/14163B（观察带 + 状态块拆出）；`BoardNarrowSummary` 795→385 行/12352B（报日条/泳道列表/组 summary/其他组拆出）；新增 7 个 source 全部 <500 行且 <15KB，DOM/testid/props/emits/交互与窄屏原 12 例不变。
- **API 统一**：`ObservationResponse<T> = ApiResponse<T>`；产品 load 恒定 days=0 且签名不再接受窗口 days（不会少覆盖却标 `coverage:'all'`）。
- 仍待父承接：G4 真实接线与顺序集成验证、真实浏览器/几何/双主题、大测试文件与 G1/G2 文件体量的独立结构包（本包未触碰）。
