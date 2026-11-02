# 归档统计带改造（v11 / FB-14）交付与验收

> 线程：主线程直改（未派子线程）。范围：归档视图顶部统计带 + 工具行形态。
> 顺序：FB-14 留痕 → ui-design v11 段 → 原型 revision → 实现 → 定向单测 / lint / typecheck → 部署 → 实页双主题验收。
> **未跑**全量 `pnpm test:unit`、`pnpm build`（部署脚本内含构建）、后端/Go/SQL/DB/AI。

## 1. 变更文件

| 文件 | after（行 / SHA256 前 16） | 动作 |
| --- | --- | --- |
| `front/app/features/tags/components/console/BoardArchiveView.vue` | 674 / `374d95f8f7271351` | 改：`ar-stats` 四格 → `ar-deck`；`statsItems` → `inReportPct` / `rangeLabel` / `filterCounts`；工具行分三段 + 右端动作组；chip 去计数（计数进 `title`） |
| `front/app/features/tags/components/console/boardArchiveView.css` | 548 / `7663ec9752b6b696` | 改：删 `.ar-stats/.ar-stat*/.ar-window*/.ar-sep/.ar-chip-count`，加 `.ar-deck*`（含 5px 比例条）、`.ar-group` / `.ar-toolbar-tail`；窄屏媒体查询同步 |
| `front/app/features/tags/components/console/BoardArchiveView.test.ts` | 406 / `b8f09793843bed3b` | 改：新增 deck 用例（比例 40%、交叉文案、跨日 `→`）、chip 去计数用例；空态补 `—` 断言 |
| `openspec/.../human-feedback.md` | 140 / `64fdd6b53d288536` | 追加 FB-14 |
| `openspec/.../ui-design-archive-rework.md` | 135 / `4ec6a394fd38ceae` | 追加 v11 修订；同步 User Journey / IA / Visual Design / 状态表四处 |
| `openspec/.../ui-prototype/revision-2026-11-01-archive-deck/` | 新建（`gen.py` + `standalone.html` + `reference/`） | 新原型修订：统计带 + 工具行同款形态 |
| `openspec/.../ui-verification/archive-deck-2026-11-01/` | 本目录 5 张截图 + 本文件 | 验收证据 |

未触碰：`useBoardArchive.ts`（口径与取数）、`specs/workshop-article-archive/spec.md`（信息量未变，不产生 delta）、后端、API、数据库、`docs/reference/**`（无「统计带」相关表述）。

## 2. 设计对照（FB-14 五项 → 落实）

| 问题 | 落实 |
| --- | --- |
| P1 数字两行内重复 | 计数只在 deck 出现一次；chip 文本固定为「全部 / 未纳入日报 / 弱证据」，计数进 `title` |
| P2 四格暗示四分法 | 一行结构 `N 篇 ｜ 已进 X（P%）｜ 未进 Y ｜ 弱证据 Z（可与入报交叉）`；弱证据明确标注交叉而非并列 |
| P3 口径权重最低 | 两句口径改为 deck 内直接可见小字（不靠 hover） |
| P4 大屏语言 | 删卡片/grid/serif 1.5rem；改 mono 数字 0.9rem + 5px 比例条，与列表发丝线同一体系 |
| P5 命名撞口径 | 保留 spec 术语「已进日报 / 未进」，但以比例条与「可与入报交叉」消歧；顶栏「打开日报」未被占用 |

## 3. 命令与结果

| 命令 | 结果 |
| --- | --- |
| `pnpm test:unit app/features/tags/components/console/BoardArchiveView.test.ts --maxWorkers=2` | 19 passed（含新增 deck 用例） |
| `pnpm lint` | 0 error（8 warning 均为既有文件） |
| `pnpm exec nuxi typecheck` | exit 0，0 个 TS error |
| `bash scripts/dev/deploy-frontend.sh` | 构建 + 铺盘 + 重启后端 + `/health` 200 |

## 4. 实页验收

| 视口 / 主题 | 证据 | 结论 |
| --- | --- | --- |
| 1920×1000 明 | `deck-1920-light.png` | deck 单行：`2026-10-30 · 137 篇 ｜ 已进日报 28 ▮▮ 20% ｜ 未进 109 ｜ 弱证据 52 可与入报交叉`；口径一行；工具行单行，`⚙筛选 / ⓘ匹配流程` 靠右 |
| 1920×1000 暗 | `deck-1920-dark.png` | 同上；比例条与数字在暗色可读 |
| 430×900 暗 | `deck-430-dark.png` | deck 折 2 行、口径折 2 行；工具行折 4 行无横向滚动；动作组左对齐 |
| 原型 1440 | `prototype-deck-1440.png` | `revision-2026-11-01-archive-deck` 渲染无报错；`2026-10-25 → 2026-10-31 · 163 篇 ｜ 已进日报 81 ▮▮ 50% ｜ 未进 82 ｜ 弱证据 55（可与入报交叉）` |

- 实页数据核对：137 = 28 + 109（deck 自洽）；弱证据 52 独立计数（不与入报互斥），与 `useBoardArchive.archiveStats` 口径一致。
- 未覆盖：物理 4K 截图（以 1920 CSS 等效视口验收）；e2e；Safari/Firefox。
