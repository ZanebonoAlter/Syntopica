# Tasks

<!-- doc-impact: flow, api, database, configuration -->

## A. API 层（子线程A）

- [x] 1.1 A1 dataenrichment.md analysis-methods 退役改写
- [x] 1.2 A2 daily-reports.md 补 GET /today
- [x] 1.3 A3 analysis/pause + settings/bocha + settings/proxy 文档化（落点：schedulers.md / dataenrichment.md / system.md，_index.md 同步）

## B. Database 层（子线程B）

- [x] 2.1 B1 三张零文档表归域文档（section_assignment_overrides / topic_anchor_overrides → daily-report-watch.md §9.13/9.14；tag_category_meta → topic-tags.md §4.8）
- [x] 2.2 B2 _index.md 表清单回填 16 张 + 计数改真库口径（86 = 84 业务 + 1 废弃残留 + 1 框架；顺带 2 张幽灵表移废弃节 + 4 个漏列补齐）

## C. Flow / 配置层（子线程C）

- [x] 3.1 C1 semantic-board.md 热度入口 heat-v2 修正（L234/L472）
- [x] 3.2 C2 两 flow 文档 8 处失效前端引用回收（+同概念 3 处）
- [x] 3.3 C3 变更溯源补 3 行 + 修 3 死链（+顺手 7 处 archive 死链）
- [x] 3.4 C4 configuration.md 补 4 环境变量 + analysis-methods 残留句清理（A 组发现的追加项）

## 验收

- [x] 4.1 三层文档改动抽查：退役口径不再出现活端点表述、新表字段与 GORM 模型一致、死链全部指向 archive/ 或活跃目录真实路径
- [x] 4.2 文档引用路径存在性复检（审计脚本口径）：flow 代码入口 0 失效引用（cmd/xxx 为 backend-go/cmd/ 简写，均存在）；check-standards 312/314，余 2 条为已登记遗留

## 测试

纯文档 change：豁免代码测试（后端无 .go 改动、前端无 .vue/.ts 改动，change-scope 无命中包）。

## 文档

<!-- doc-impact: flow, api, database, configuration -->

- 全部 9 项修复落在 docs/reference/（api 5 文件、database 5 文件、flow 4 文件 + configuration.md），清单见上各组行。

## 验证

- `grep -En '^\| (GET|POST|PUT|PATCH|DELETE).*analysis-methods' docs/reference/api/dataenrichment.md | wc -l` → 期望 `0`（表格无活端点行；L520 退役注记提及旧端点全部 404 属预期）
- `grep -c 'daily-reports/today' docs/reference/api/daily-reports.md` → 期望 ≥ `2`（表格行 + 小节）
- `grep -rln 'section_assignment_overrides\|topic_anchor_overrides' docs/reference/database/tables/ | wc -l` → 期望 ≥ `1`（两表入域文档）
- `for t in $(docker exec syntopica-postgres psql -U postgres -d syntopica -tAc "SELECT table_name FROM information_schema.tables WHERE table_schema='public'"); do grep -q "\`$t\`" docs/reference/database/_index.md || echo "MISS:$t"; done` → 期望无输出（真库表全收）
- `grep -c 'heat-v2-lane-clues-1d' docs/reference/flow/semantic-board.md` → 期望 ≥ `1`（现行热度口径入口）
- `grep -rEn 'openspec/changes/(lane-manual-correction-loop|unify-lane-workbench)\]' docs/reference/flow/ | wc -l` → 期望 `0`（死链已修，均指 archive）
- `grep -c 'FINGENIUS_API_KEY\|COMPRESS_MIN_BYTES\|DEMO_READ_ONLY\|RSSHUB_REWRITE' docs/reference/configuration.md` → 期望 ≥ `4`
- `bash scripts/harness/check-standards.sh` → 期望除已登记遗留（add-prototype-slop-filter 溯源、streamline-business-test-verification doc-impact）外全 [OK]

## 遗留（非本 change 引起，登记不修）

- `2026-11-16-add-prototype-slop-filter` 溯源欠账（standard 域，无 flow 归属）——前 change 债
- `daily_report_sections.watch_id` 表文档漏列——B 组发现，范围外
- `streamline-business-test-verification` doc-impact 失败——他人 change 债
