# 矩阵静态新版 S9 第三轮（N1 极小收口 + 关键烟测）独立 K3/UI 报告

- 验收人：独立 K3/UI 验证线程（非实现作者；commandcode/deepseek/deepseek-v4.1-flash:max，implementer 档，未派子线程）
- 时间：2026-10-28 07:39–08:0x（Asia/Shanghai）；浏览器会话 `matrix-s9r3-6d5795`（agent-browser 0.37.1）
- 入口：`http://localhost:5100`；**index SHA `a35cd4cb8d74ead2750c172f380a7f352a1794b700e25be7c96c8cf29d0808e4`（实测 200/7457B 一致）**，`buildId:"16379c1d-5b69-412e-9faa-8aa2915a2394"`，chunk `/_nuxt/COV_Llg2.js`（实测 200/349388B，SHA `b037c968…` 与服务端一致）；后端 PID **1061075** 未重启
- **结论：N1 通过（1440/375/390，左右两端夹边交叠带均按最近可见端；run2 的 DOM 固定 end 不再发生）；小烟测通过；未发现新的 High/Medium，无需回退。**
- 范围声明：本轮**只做 N1 收口 + 关键烟测**，不是 25 步/10 视口全扫；未改的 CSS/DOM 宽窄双主题结论引用 run2 证据（`run2/report.md` 与 run2 PNG），不宣本次全视口重跑。

## 1. 现场与准入（实测）

| 项 | 结果 |
| --- | --- |
| index / buildId / chunk | `a35cd4cb…` / `16379c1d-5b69-412e-9faa-8aa2915a2394` / `COV_Llg2.js` 全部实测一致 |
| 后端 | PID 1061075 存活（ELAPSED 07:45:57）未重启 |
| 构建 | `.pi/subagents/validation/matrix-static-20261028-072952-run3/build.log`，deploy_exit=0，81s |
| manifest delta（path→SHA） | **仅 3 改**：`rangeBrush.ts`、`rangeBrush.test.ts`、`LaneChronicle.test.ts`；`path-sha-delta-vs-run2.tsv` 首行起 3 行，无其它漂移 |
| 源码指纹 | before/after 同 SHA `6b5269a2…`（构建期零源码变动） |
| 备份 | 当前前版 b861 包 `matrix-before-20261028-073014.tar.gz`，SHA `3d27fe9da20fb094ba58ce3e64e7e4798fd7f6187febc8211d00dd310243613d`（仅父可回退） |
| 未做 | 未 test/build/typecheck/dev/SQL 写/生成/激活/收编/删除/保存；未改产品/后端/其它 change/旧证据；主 tasks/review/8B 未动 |

## 2. 安全闸

- `run3/tools/cdp-method-guard.mjs`（run2 逻辑副本）：CDP `Fetch.enable('*', Request)` 挂页面/iframe；**仅 GET/HEAD/OPTIONS 放行**，其余 `failRequest(BlockedByClient)` 落 `guard.jsonl`。
- 实证（真实页面）：`POST /api/__guard_selftest_page` 被拦（TypeError，log 记录）；**打开站内原文时的 `POST /api/reading-behavior/track-batch` 被拦（log 记录，未落库）**；`GET /api/semantic-boards/overview` 200。
- 未点生成/收编/激活/删除/保存；所有交互为真实点击/指针/键盘/日期输入（未用 eval 触发 Vue `$emit`，未用 eval.scrollTo 代替用户输入）。

## 3. N1 专测（真正触发区间）

**触发条件（读源码 + 实测确认）**：新 `buildHitRegions` 先对两端做 24px 区夹取，再判“实际区间是否相交”。端点贴池边时，`startLeft = plotW-gap-12`、`endLeft = plotW-24`，相交当且仅当 **gap < 36px**；因此 **raw gap ∈ [24,36)** 是“run2 旧逻辑两独立区、新逻辑必须合并”的窗口（gap∈[36,48) 夹取后不相交，属正常两端区）。本轮用 13/14/30 日窗取到 gap 25.1–28.6px，均在窗口内。

| 视口/位置 | range | raw gap | 夹取后区间/交叠 | 命中区 | 交叠带探针（距 start / end） | 真实指针结果 |
| --- | --- | --- | --- | --- | --- | --- |
| 1440 右端贴池边 | 2026-10-14..10-27（14 日） | **28.6px** | start [1091.8,1115.8] / end [1108.4,1132.4] → 交叠 [1108.4,1115.8] | `brush-hit-overlap` 1091.8..1132.39（w 40.59） | x=1112（8.19 / 20.41）→ 近 start | 按下+左移 4px：**preview start 10-12、end 不动、URL 不变**；松手提交 `?start=2026-10-12&end=2026-10-27`；首动 1px 仅 -1 天 |
| 1440 左端贴池首 | 2025-10-28..11-09（13 日） | **26.4px** | start [0,24] / end [14.4,38.4] → 交叠 [14.4,24] | `brush-hit-overlap` 331.59..369.98 | x=353（21.41 / 4.99）→ 近 end | 按下+右移 4px：**end 11-11、start 不动、URL 不变**；松手提交 `?start=2025-10-28&end=2025-11-11` |
| 375 右端贴池边 | 2026-09-28..10-27（30 日） | **25.13px** | 两 24px 区交叠 | `brush-hit-overlap` 323.06..360.19（w 37.13） | x=340（4.93 / 20.2）→ 近 start | 按下+左移 4px：**start 09-23、end 不动、URL 不变**；提交 `?start=2026-09-23&end=2026-10-27` |
| 375 左端贴池首 | 2025-10-28..11-26（30 日） | **25.13px** | 交叠 | `brush-hit-overlap` 44.8..81.92 | x=65（20.2 / 4.93）→ 近 end | 按下+右移 4px：**end 12-01、start 不动、URL 不变**；提交 `?start=2025-10-28&end=2025-12-01` |
| 390 右端贴池边 | 2026-09-28..10-27（30 日） | **26.32px** | 两 24px 区交叠 | `brush-hit-overlap` 336.88..375.19（w 38.31） | x=356（7.12 / 19.2）→ 近 start | 按下+左移 4px：**start 09-23、end 不动、URL 不变**；提交 `?start=2026-09-23&end=2026-10-27` |

- **N1 判定：PASS。** 关键差异：run2 在 1440 14 日（同样 raw gap 28.6、同样探针）由 DOM 后者 `hit-end` 抢判，实际移动的是 end；本 build 该区间合并为单区，按最近可见端判为 start，end 不动。左右两端均验证；preview 阶段 URL 始终不变，松手恰提交一次。
- 首动不跳：1440 交叠带 1px 首动仅 -1 天；4px ≈ -2 天（0.45 日/px），无跳变；375/390 的 4px ≈ 5 天（1.15 日/px），与像素比例一致。
- 证据：`metrics/n1-1440-*`、`n1-375-*`、`n1-390-*`、`n1-trigger-run3.json`。

## 4. 关键小烟测（非全扫）

| 烟测 | 结果 |
| --- | --- |
| 14 日两端可见线中点 | start 中点 +10px → start 10-19（end 不动）；end 中点 -10px → end 10-22（start 不动） |
| 7 日（<24 合并） | `brush-hit-overlap` w 25.2（右夹边）；左线中点 +6px → start 10-24 |
| 单日 | 上/下半区分别 start 10-15 / end 10-25 |
| Esc | 拖动后 Esc → 恢复 10-14/10-27、URL 不变、随后松手不提交 |
| 30 列 | `matrix-scroll sw=cw=845`、非 scrollable、30 个日期头、页面 1440/1440 无横溢 |
| 缓存 365↔14 各一次 | 365：`?days=365`、默认 2025-10-28..2026-10-27、365 列、**0 新增请求**（与 B 池共用）；14：`?days=14`、默认 10-14..10-27、14 列 |
| 主页面选格→标题/摘要→原文 | 点 1204/10-27 格 → 编年；thread 15565 真实 summary；相关 1 篇（157310）→ 弹层 `role=dialog`/`aria-modal=true`/`aria-label=站内原文阅读`/初始焦点在 dialog，1296×758（90vw 存量），正文真实；Esc 关闭后焦点回 `lane-chronicle-article-157310`，stage scroll 468 不变 |
| 控制台异常 | 清空后做日期输入 + 选格 + 读取，`console` 消息数 **0** |
| 宽窄双主题 | 新包桌面 1440（浅色默认/30 列）、手机 375（深色 30 日）、390（浅色 30 日）新图并已看图；其余未变 CSS/DOM 的双主题结论引用 run2，不宣本轮全视口重跑 |

**看图结论（新 PNG，亲自 read）**：
- `png/s9r3-1440-14d-plot.png`：头部单行+刷新、轨 272/255、矩阵 14 列、无溢出，与 run2 同构；
- `png/s9r3-1440-30col.png`：30 列单屏、两行 `09/28…10/27` 可读、格值不裁、池补充脚注在场；
- `png/s9r3-1440-article-modal.png`：真实 reader 正文（来源/日期/标题/标签/配图），非导语层；
- `png/s9r3-375-dark-30d.png` / `png/s9r3-390-editorial-30d.png`：手机头部完整、刷选 30 日窗、无横溢，390 窄屏摘要 40 线索/14 分区与 API 一致。

## 5. 未验 / 边界（本轮不扩）

- 第 24 步“真实浏览器缓存容量 14/20 大样本”**仍未验**（本轮未造 15 份报告/21 篇文章）；父的便宜层补证 `run2/cache-layer-supplement.md` 记录 4 files/70 tests 全绿（含 CA-3/4/5/7/8、RD-1/2/3/4/5、rangeBrush 夹边测试、LaneChronicle loaded-empty），**是组件/纯函数层证据，不改 PARTIAL**。
- 真实生成 DB 提交与 WS 先后、物理真机触摸仍单列未验（本轮无 CDP 触摸需求，未重做）。
- run2 的其它未验子项（thread_count 缺省、空 summary 真库样本、报告/section 缺失、无日报板块、A 首错/stale、snapshot=null、0 线索格）保持原状，不因本轮改变。
- 旧根 53/1/2、旧投诉、他域 7 条 SignalReport 类型债未洗。

## 6. 分层综合交付建议（给父）

| 层 | 证据 | 结论 |
| --- | --- | --- |
| N1 行为（本轮） | 上述三视口左右夹边交叠带真实 pointer 测试 + URL/preview | **PASS**，可关闭 N1 |
| 手柄其余语义（本轮烟测） | 14/7/单日/Esc/池边界 | PASS（run2 大样本仍有效） |
| 缓存 14/20 容量 | 组件/纯函数层 4 files/70 tests（父补证，实跑） | 逻辑层通过；**浏览器大样本未验**，第 24 步保持 PARTIAL |
| 布局/宽窄双主题 | 本轮 1440/375/390 目标状态新图 + run2 全视口证据 | 无回归 |
| 真实生成 DB/WS、真机触摸 | 无 | 单列未验 |

**是否需回退：否。** 未发现新的 High/Medium；N1 通过；本阶段静态验收可交付。主 tasks/review/8B 由父闭环，本线程未改。

## 7. 证据清单（run3，未覆盖 run1/run2）

- `report.md`（本文件）、`metrics/n1-trigger-run3.json`、`metrics/n1-1440-*.json`（6）、`n1-375-*.json`（4）、`n1-390-*.json`（3）、`smoke-1440-14d-geom.json`、`smoke-1440-30col.json`、`smoke-article-dialog.json`、`guard-summary.json`
- `png/s9r3-1440-14d-plot.png`、`s9r3-1440-30col.png`、`s9r3-1440-article-modal.png`、`s9r3-375-editorial-30d.png`、`s9r3-375-dark-30d.png`、`s9r3-390-editorial-30d.png`
- `guard.jsonl`（blocked：reading-behavior×1、自检×1）、`tools/cdp-method-guard.mjs`、`tools/cdp-input.mjs`
- 首轮/第二轮报告与 PNG 原样保留（run2 `report.md`、`cache-layer-supplement.md` 未改）。
