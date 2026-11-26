# G1 父复核：R1/R2待回归验证修复

G1作者自测6文件106例、相邻2文件66例及本组TS eslint绿，记录仍在matrix-day-others-g1.md。父检查当前源码后发现以下静态可达竞态，尚未亲自执行新回归用例；不能拿原绿记录称已覆盖这些边界。tasks8C.2暂重新置未完成，等待全新短线程先复现再修。

## R1：索引强刷的新旧响应缺少同板块请求序保护（Medium）

useLaneDayReading.ts/requestIndex允许force绕过当前in-flight启动新请求；runIndexRequest只校验boardEpoch/boardId，写index时只按覆盖days比较。旧/新请求相同days时，旧请求在新强刷成功后回来仍满足`index.days <= days`，可把新报日覆盖掉；旧失败也可在新成功后写indexError。普通扩大覆盖的并发读也需核错误/加载状态归属。

修复必须保“强刷后新报日可见/失败保旧索引/独立lane与报告读取/在途合乘”；不能只加丢弃分支让仍合法的旧消费者永远停loading。可选择最小请求版本/承接最新或串行强刷方案，单份索引状态与缓存不能复制。

## R2：clear后旧请求回填缓存及跨板块相关文章晚到发布（Medium）

既有laneDayReadingCache.ts/load在loader resolve后无失效generation保护，会直接delete inflight/set；clear只清两Map，旧promise仍能再填缓存。clear→同key新请求→旧请求先/后完成，还会删除新in-flight槽，破坏去重。此为继承路径的缺口，不能算作者本次新引入，但CA-7及新壳共享读取前必须补。

useLaneDayReading.ensureRelatedArticle在await articleCache.load后无board/epoch检查，旧板块的返回可发布到新板块relatedEntries；即使缓存层返回失效失败，调用层也不能把失效当新板块错误发布。需cache与调用端都保护。

只允许本包store、laneDayReadingCache及其测试最小修复；不改Vue、选日/normalizer/其他业务契约，不修改原型、后端或foreign。修复报告追加真实红→绿结果；旧106/66历史不覆写，阶段验收在本批过后更新。

## 父独立复验与R3前置小门禁

R1/R2修复后父复算4个文件SHA与回包一致，并实际运行：`cd front && pnpm test:unit app/features/tags/composables/useLaneDayReading.test.ts app/features/tags/composables/laneDayReadingCache.test.ts --maxWorkers=2 -t 'R1 —|R2 —|clear 后旧请求晚到'` → 2 files / 10 passed / 46 skipped，exit0。此仅独立重测新增10例，不宣父重跑完整116/相邻66。

静态复核另见R3：已有成功索引days=369，生成后anchor前移使reportIndexDays变365；force确发新365请求，但runIndexRequest的`index.days <= days`不成立，忽略新报日却返回旧369数据。旧R1请求序修复正确，不能替代该覆盖缩小时的刷新。最小修复方向：请求/force有效days至少覆盖当前成功/当前在途的较大覆盖，使新鲜结果能更新且不缩回范围；保持单份缓存，不合并猜测旧数据。

该角尚待真实回归复现，授权下一全新G2线程作为明确前置小门禁先补测并最小修，仅useLaneDayReading及test；R3与原6文件目标绿后才能重勾8C.2并继续其G2专有网格文件。若不能最小复现/修或需改接口，停报，不跳门禁。G2不得借此重写其他G1模型/normalizer/cache。保留前两次106/116与66的真实历史。

## 并发门禁补充

之后另一工作线出现auxrepair编译缺字段，用户通知lint/vet/build三项转绿；又有auxiliary_description_repair.go的gofmt警报，父只读`gofmt -d`已无差异，随后用户通知lint转绿。父未修改Go，未重跑全量门禁，不据这些通知宣本增量所有测试已过。CodeGraph曾超时，已知路径精确读取，不unlock/restart他人pi。

## 关闭状态（2026-10-29，全新短线程）

R1/R2 均先复现真红（10 例失败断言与本节描述一一对应）后最小修复并转绿；修复细节与终态证据见 `matrix-day-others-g1-race-fixes.md`，tasks 8C.2 已勾。原发现保留不改写；旧 106/66 记录仍不代表这两个竞态已覆盖，当前有效数字为：目标 2 文件 56 例、本包 6 文件 116 例、相邻 66 例、本组 12 个 `.ts` eslint exit 0。未改 Vue/后端/部署，未提交归档。

## R3 关闭状态（2026-10-29，G2 线程前置小门禁）

R3（生成后 anchor 前移使索引需求覆盖 369→365，force 的新鲜列表被「已有更大覆盖不缩回」比较丢弃）已由 G2 线程作为**明确前置小门禁**先复现真红再最小修：

- 真红：`pnpm test:unit ...useLaneDayReading.test.ts --maxWorkers=2 -t 'R3 —'` → **2 failed | 45 skipped**（`expected [ '2026-10-22' ] to deeply equal [ '2026-10-22', '2026-10-28' ]`——新报日不可见；第二例 force 实发 365 而非在途 369）。
- 修复：仅 `requestIndex` 一个 hunk——新请求有效覆盖 `= max(请求 days, 当前成功索引 days, 同板块在途 days)`，仍整体替换单份索引（不合并/不猜旧数据）；接口、缓存契约、请求序转交、双 epoch、R1/R2 全部不动。
- 真绿：同命令 **2 passed**；G1 原 6 文件集合 **118 passed**（116 + 2）；本组 12 个 `.ts` eslint exit 0；相邻 `LaneChronicle + BoardConsoleShell` **66 passed**。
- 细节与 SHA 见 `matrix-day-others-g2.md` §A；tasks 8C.2 已重勾。**R3 关闭**，本节保留原发现不改写；旧 106/116/66 记录仍不作为 R3 已覆盖的证据。
