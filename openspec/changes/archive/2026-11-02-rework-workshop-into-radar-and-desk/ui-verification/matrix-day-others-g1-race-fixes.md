# G1 父复核 R1/R2 竞态修复报告（tasks 8C.2）

> 来源：`matrix-day-others-g1-review.md` R1（索引强刷新旧响应）/R2（clear 后晚到回填 + 跨板块相关文章晚到）。
> 线程：全新短线程（原 G1 作者已结束，父不 resume）；唯一 writer，未派子线程、未提交/归档/部署。
> 旧记录（6 文件 106 例、相邻 66 例）保留在 `matrix-day-others-g1.md`；**旧用例没有覆盖本次两个竞态**，本报告只以新增用例的红→绿为准。

## 0. 结论

- R1、R2 均先复现出**真实红**（10 例红，失败断言与预期缺陷一一对应），再用最小改动修绿；无伪造红、无放宽容忍。
- 目标套件 2 文件 56 例全绿（含新增 10 例）；本包 6 文件基线对应当前完整集合 116 例全绿；本组 12 个 `.ts` eslint exit 0；相邻 LaneChronicle/BoardConsoleShell 66 例回归绿。
- 未改 Vue / normalizer / 选日模型 / 其它 API / 后端 / 原型 / 部署文件；未新增第二 store/第二缓存/helper 文件。

## 1. 写域文件与 SHA256（before → after）

| 文件 | before | after | 行数 |
| --- | --- | --- | --- |
| `front/app/features/tags/composables/useLaneDayReading.ts` | `1e43b08ac2baebc4…` | `f47887ca08406206…` | 495 → 513 |
| `front/app/features/tags/composables/useLaneDayReading.test.ts` | `5793fc3d2ffa855d…` | `c656ec7a36016ada…` | 744 → 871 |
| `front/app/features/tags/composables/laneDayReadingCache.ts` | `44a07d8a63d8bddc…` | `a5b1b94c86ffe888…` | 83 → 98 |
| `front/app/features/tags/composables/laneDayReadingCache.test.ts` | `127a73ee8ccbbfff…` | `6ba464ee2e63d758…` | 94 → 190 |

前 4 文件当前均为本 change 尚未入库的新文件（untracked），故 before SHA 取自开工前实测、after 为收尾实测。写域外文件未触碰（工作区里其它脏文件属他线，未计入本包）。

## 2. R1 复现（红）→ 修复 → 绿

### 2.1 缺陷与复现断言

原实现 `requestIndex(days, force)` 在 force 时绕过在途请求另发一个列表请求，但 `runIndexRequest` 只校验 `boardEpoch/boardId`，写索引仅比 `days` 覆盖——同 days 的新旧响应没有请求序：

| 新增用例 | 红阶段失败断言（原实现实测） |
| --- | --- |
| `R1 → force 新请求先成功、旧普通请求晚到：报日/索引不回退` | `expected [ '2026-10-22' ] to deeply equal [ '2026-10-22', '2026-10-27' ]`（旧列表把生成后新增报日缩回） |
| `R1 → force 刷新成功后旧请求失败：不写旧 indexError…` | `expected '旧请求挂了' to be null`（旧失败覆盖成功态错误位） |
| `R1 → 旧/新消费者都终结：lane 用最新索引不永 loading…` | `expected 'no-report' to be 'ready'`（lane 消费者拿到被旧列表覆盖后的索引，误报无日报） |

第 3 例同时锁「不永 loading」：lane 读取与 report 读取两个消费者都在同一场景里必须终态结束；修复不能靠「直接丢弃旧结果让旧调用方停在 `loading=true`」。

### 2.2 修复（`useLaneDayReading.ts`）

- 新增 `indexSeq`（索引请求版本，每启动一次列表请求 `++`），`indexInflight` 记录 `seq`。
- `runIndexRequest(boardId, epoch, seq, days)` 在 epoch/board 校验之后增加请求序判定：
  - `seq !== indexSeq`（已被更新请求超越）时**不写** `index/indexError`；
  - 若最新请求仍在途（`latest.seq > seq` 且同 epoch），**await 最新请求的 promise 并把它的结果转交**给本消费者（旧消费者不永 loading、看到的是最新索引）；
  - 最新请求已结算时返回当前索引快照（`index.value.items` + `indexError`），旧响应绝不回写。
- 只有当前 `seq === indexSeq` 的响应可写索引、清/写 `indexError`；`indexLoading` 仍由 `indexInflight === entry` 的身份判定归位（旧 finally 不会错清新请求的 loading）。
- 保留：单索引/板块、已有更大覆盖不被小列表缩回、失败保留旧索引（`RDC-6` 全绿）、force 必发新请求（用例断言 list 调用数 1→2）。

不使用「串行强刷」方案：串行会让 force 排在旧请求之后、旧响应当前消费者语义改变，且无法以真红用例证伪原缺陷；请求序 + 最新结果转交同时满足「新数据可见」与「旧消费者终态」。

## 3. R2 复现（红）→ 修复 → 绿

### 3.1 缺陷与复现断言

**缓存层**（`laneDayReadingCache.ts`）：`clear()` 只清两个 Map，`loader.then` 无失效代检查，且 `inflight.delete(key)` 不校验是否仍属自己。

| 新增用例 | 红阶段失败断言 |
| --- | --- |
| `clear 后旧成功晚到：不回填成功缓存，结果标 stale` | `expected true to be false`（原实现 `res.ok === true` 且回填了缓存） |
| `clear→同键新请求在途→旧成功晚到：不删新在途槽…第三个 load 合乘新请求` | `expected true to be false`（旧 payload 已回填 → `cache.has('k') === true`） |
| `clear 后新请求先成功、旧成功晚到：新内容/缓存/在途槽不变` | `expected undefined to be true`（无 `stale` 标记，旧内容覆盖新缓存） |
| `旧请求 reject 晚到：不删新在途槽、不污染缓存` | `expected undefined to be true`（无 `stale` 标记） |

**调用端**（`useLaneDayReading.ensureRelatedArticle`）：`await articleCache.load(...)` 之后没有 board/epoch 校验，旧板块的返回直接 `publishRelated`。

| 新增用例 | 红阶段失败断言 |
| --- | --- |
| `resetForBoard 后旧成功晚到：relatedEntries 不复活、文章缓存不回填` | `expected 'success' to be 'idle'`（旧板块 success 重新出现，缓存 count 1） |
| `切板块后旧失败晚到：不把「缓存已失效」发布成新板块错误` | `expected 'error' to be 'idle'`（缓存失效错误冒成新板块条目错误） |
| `同 ID 新板块先成功、旧板块晚到：不覆盖新板块条目与缓存` | `expected '旧板块文章' to be '新板块文章'`（旧响应覆盖新板块 success 与缓存 payload） |

### 3.2 修复

`laneDayReadingCache.ts`
- 新增失效代 `generation`：`clear()` 时 `generation += 1`；每次 `load` 捕获 `requestGeneration`。
- 结算时先按**槽位身份**清理：仅当 `inflight.get(key) === entry` 才 `delete`——clear 后同键新请求的槽不会被旧结算删掉（在途合乘不被破坏）。
- `requestGeneration !== generation` 时返回 `{ ok: false, fromCache: false, stale: true }`：不 `set`、不删新槽、调用方可据此拒绝发布。
- 接口兼容：`BoundedCacheLoadResult` 只新增**可选** `stale?: boolean`；FIFO/容量/force/在途合乘/失败不入缓存（不逐出成功项）全部保持。

`useLaneDayReading.ts`
- `ensureRelatedArticle` 在调用前捕获 `boardId` + `boardEpoch`，await 后校验：跨板块/卸载（epoch 或 boardId 变化）或 `res.stale` 时**直接返回**，不发布 success，也不发布 error（失效不是新板块的错误）。
- 仍然发布真实当前请求的失败（generation 未变时 `ok:false` 照常落 `error` 条目）。

## 4. 验证（Linux 本机串行，参数不带 `--`）

### 4.1 红 → 绿轨迹（各阶段单次实跑，数字不跨轮相加）

| 阶段 | 命令（`cd front` 后） | 结果 |
| --- | --- | --- |
| 红（修复前，新增 10 例） | `pnpm test:unit app/features/tags/composables/useLaneDayReading.test.ts app/features/tags/composables/laneDayReadingCache.test.ts --maxWorkers=2` | **2 files failed / 10 failed \| 46 passed (56)**（断言见 §2.1/§3.1） |
| 红（R2 调用端单跑留痕） | 同上加 `-t "R2 — 相关文章跨板块晚到不发布"` | **1 file failed / 3 failed \| 42 skipped (45)** |
| 绿（修复后，目标 2 文件） | 同红命令 | **2 files / 56 tests passed** |
| 绿（稳定性复跑 3 次） | 同红命令 ×3 | 每次 **56 passed** |
| 绿（本包 6 文件基线） | `pnpm test:unit app/features/tags/composables/useLaneDayReading.test.ts app/features/tags/composables/laneDayReadingCache.test.ts app/api/normalizers/laneDayReading.test.ts app/features/tags/components/console/dayOtherTopics.test.ts app/features/tags/components/console/laneConsoleView.test.ts app/features/tags/components/console/useConsoleSelection.test.ts --maxWorkers=2` | **6 files / 116 tests passed**（旧 106 + 新增 10） |
| 绿（相邻回归，因改动触及既有消费者 `LaneChronicle` 注入的 store，单次目标跑） | `pnpm test:unit app/features/tags/components/console/LaneChronicle.test.ts app/features/tags/components/console/BoardConsoleShell.test.ts --maxWorkers=2` | **2 files / 66 tests passed**（与旧记录同数，未破） |
| eslint（本组 12 个 `.ts`） | `pnpm exec eslint <useLaneDayReading(.test).ts laneDayReadingCache(.test).ts normalizer(.test) dayOtherTopics(.test) laneConsoleView(.test) useConsoleSelection(.test)>` | exit 0（0 error 0 warning，无输出） |

细分计数（最终集合）：normalizer 9、useLaneDayReading 45（39+6）、laneDayReadingCache 11（7+4）、dayOtherTopics 20、laneConsoleView 23、useConsoleSelection 8 = **116**。

### 4.2 未跑（按 brief 红线）

未跑全量 `pnpm test:unit`、`nuxi typecheck`、`pnpm build`、全树 lint、浏览器/agent-browser、Go/DB/SQL/AI、dev 服务起停、部署、提交/归档。**不声称**这些通过。

## 5. 接口变化与终态证据

- 对外接口**无签名变更**：`useLaneDayReading` 返回字段、`ensureDayReading/ensureReportDay/ensureReportIndex` 签名、`BoundedRequestCache` 方法签名均不变；`BoundedCacheLoadResult` 仅新增可选 `stale?`。故 G4（8C.5）接线代码无需改动。
- 索引终态证据：force 后旧列表晚到 → `reportDates` 保持含新增报日、`indexDays` 仍 369、`indexError === null`、`indexLoading === false`；旧消费者 `loading` 终态 false 且 `current.status === 'ready'`。
- 缓存终态证据：clear 后旧成功/失败晚到 → `size() === 0` 或保持新值、`peek` 为新 payload、第三次 load 不再发请求（loader 调用数恒 2）。
- 相关文章终态证据：切板块晚到后 `readRelated(id) === { status: 'idle' }`、`articleCacheCount() === 0`；新板块 B 成功先到 + 旧 A 晚到 → 条目/`getArticlePayload` 均为 B，`articleCacheCount() === 1`。
- 单索引/单缓存状态所有者未变（仍只有 `index` + `reportCache` + `articleCache`），未新增 helper 文件。

## 6. 未覆盖 / 风险（交给 G4 与父复核）

1. **G4 接线风险（最重要）**：`ensureReportIndex({ force: true })` 现在保证「force 必发新请求」且旧响应不覆盖；壳层生成完成链路必须**先 await force 刷新再读当日**（`ensureReportDay` 的 `force` 仍只绕过详情缓存）。若 G4 并行触发「生成完成强制刷新」与「用户切日读取」，旧消费者的 lane 读取会等到最新列表返回才落定（可能晚一个网络往返），这是有意的正确性换延迟，不是 loading 卡死。
2. 索引请求序只保护**列表**响应；`reportCache`/`articleCache` 的晚到保护走「失效代 + 调用端 epoch」两道，未再加每请求 seq（同键在途合乘保证同键结果一致，不同键互不覆盖）。
3. `clear()` 之外的缓存失效路径不存在（当前只有 `resetForBoard`/卸载调用 clear）；若未来新增「单键失效」，必须同样递增失效代或按槽位身份处理。
4. 真实浏览器/真库未验证（8C.7 负责）：本次只做 Vitest 目标层 + eslint；视觉/真实接口时序仍需 8C.7 覆盖。
5. 旧 106/66 记录保留，但它们**不能**作为这两个竞态已覆盖的证据；本报告的 56/116/66 才是当前有效数字。
