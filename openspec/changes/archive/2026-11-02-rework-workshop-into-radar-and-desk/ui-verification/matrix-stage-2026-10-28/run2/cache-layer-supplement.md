# 第24步缓存容量与 N1/N3 便宜层补证

- 日期：2026-10-28
- 性质：**纯逻辑/组件层证据，不是浏览器/真实数据库 14/20 大样本证据**。本文件不覆盖 run2 原报告，也不把第24步 PARTIAL 改绿。
- 命令（实际路径已核对）：
  ```bash
  cd front
  pnpm test:unit \
    app/features/tags/composables/useLaneDayReading.test.ts \
    app/features/tags/composables/laneDayReadingCache.test.ts \
    app/features/tags/components/console/rangeBrush.test.ts \
    app/features/tags/components/console/LaneChronicle.test.ts \
    --maxWorkers=2
  ```
- 结果：**4 files / 70 tests passed，exit 0**（2026-10-28 07:24 实跑）。

## 容量/逐出/失败/晚到/卸载（useLaneDayReading.test.ts，20 tests）

| 用例 | 覆盖 |
| --- | --- |
| `RD-1/2/3 — 列表 1 份与详情/文章缓存` → `同板块两个不同日：列表仅 1 次、每报告详情仅 1 次` | 列表索引仅 1 份；详情命中不重发 |
| 同上 → `同文章跨 thread 展开复用：getArticle 仅 1 次` | 文章跨 thread 复用 |
| `CA-3/4/5/6/7/8 — 有界缓存` → `CA-3：报告 0→14 份无提前逐出、命中不请求` | 报告容量上限 14；未到上限不逐出 |
| 同上 → `CA-4：第 15 份 FIFO 逐出最旧；被逐出项重读允许再请求` | 15 份 FIFO；逐出后重读 |
| 同上 → `CA-5：文章 20→21 FIFO 逐出；命中复用` | 文章容量上限 20；FIFO |
| 同上 → `CA-7：切板块释放（索引/详情/文章/相关条目/当前显示）；卸载释放` | 板/卸载释放 |
| 同上 → `CA-8：当前阅读对象独立持有——FIFO 逐出不消失` | 当前读对象稳定 |
| `RD-4 — epoch：切板块/切日迟到丢弃` | 晚到旧响应丢弃、不污染新上下文 |
| `RD-5 — 失败分层与隔离` | 列表/详情/文章失败不入成功缓存、可重试 |
| `join 分层 — degraded / no-report / 跨上下文 / stale` | 分区缺失降级、无报告、跨上下文先清显示、stale 保留 |
| `R24 — 离开可读上下文失效显示 / sectionKey 上下文隔离` | 离开上下文 suspendDisplay 不动成功缓存 |

## 缓存 helper（laneDayReadingCache.test.ts，7 tests）

覆盖 `createBoundedRequestCache`：FIFO 超限逐出、同键重写刷新插入位、同键在途合乘、失败/undefined 不入成功缓存且不逐出成功项、force 绕过缓存、clear 释放成功项与在途表。

## 本次 N1/N3 影响目标

- `rangeBrush.test.ts`（15 tests）：新增 `夹边导致 24px 区实际重叠：合并区按最近端（左夹偏向 end、右夹偏向 start）`；旧短 7/14、单日、池边界、非重叠区断言同时通过。
- `LaneChronicle.test.ts`（28 tests）：新增 `loaded 真实 thread.summary="" 展开显示「暂无摘要记录」；未取到报告不冒空态`；旧 CH 组与 R34 settled 信号用例同时通过。
- 以上 4 文件合计 70 tests 全绿；未跑 9 files/181、旧 308 或全量。

## 明确边界

- 这是 **mock fetchers/组件层** 对 14/20 容量、FIFO、失败不入缓存、晚到、卸载、当前读对象稳定的证据。
- **没有**在浏览器/真实库构造 15 份报告或 21 篇文章；第24步“真实浏览器缓存容量子项”仍为 PARTIAL，保持 run2 报告结论。
- 未改 UI 报告、旧证据、后端、原型或 tasks/8B。
