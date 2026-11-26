# UI 验收报告：相关事件（S1 看相关 / S2 调阈值）

> 变更：`enable-story-cross-day-continuation`（capability `related-events`，ui-impact: minor）
> 执行：2026-11-11 01:00–01:35 CST，agent-browser（headless Chromium）+ 静态发布路径 `http://localhost:5100`
> 证据截图：本目录 `*.png`；判定标准见 `docs/reference/standard/shared/test-design.md`「UI 验收机械断言」与 `standard/frontend/layout.md`

## 环境与前置

| 项 | 值 |
| --- | --- |
| 前端 | `bash scripts/dev/deploy-frontend.sh`（`pnpm generate` → 铺 `backend-go/frontend/` → 重启后端）。静态产物落地 01:23，本次验收跑在**最终产物**上（浏览器验收前的最后一次构建） |
| 后端 | `:5100`（`/health` 200）。**部署脚本第 3 步的 60s 健康检查连续两次判失败，实际后端在 ~110–190s 后才就绪**——本机启动期做迁移/向量维度自检并撞上 provider 超时；静态文件与 API 均已正常。属环境启动慢，非本 change 行为 |
| 数据库 | 真库（`docker compose -f docker-compose.pg.yml`），`related_event_embeddings` 由 AutoMigrate 建表 |
| 索引 | 显式执行 `go run ./cmd/related-events --refresh --board 1980`：`tags=839 indexed=839 reused=0 failed=0`，耗时 2m16s，space=`provider=qwen3-embedding|model=qwen3-embedding|gen=g1` |
| 起始阈值 | `ai_settings.related_event_similarity_threshold` = 0.82（默认），验收结束时已回填 0.82（GET 复核） |
| 数据 | 板 1980 归档日 2026-11-10；用例卡片 = event 标签 267115「Anthropic 发布 Claude Fable 5.2 与 Mythos 5.2 模型」（45 字符长标题），其相关列表 10 条（最长标签 47 字符） |

## 主链路：调阈值 → 看相关事件 → 打开来源（1440×900）

| # | 动作 | 断言（机械） | 结果 |
| --- | --- | --- | --- |
| 1 | `/tags` → 全局治理 → 匹配规则 | 弹窗 `.app-dialog` 带 `is-sized`、宽度 **760**（`size="lg"`，92vw 上限）；`[data-testid="mc-related-threshold"] input` 值 **0.82**；区块文案含「不合并事件」 | PASS |
| 2 | 改 0.95 → 保存 | 弹窗关闭、无 `mc-save-error`；`GET /api/semantic-boards/matching-config` 返回 **0.95**，`semantic_board_match_sim_threshold` 保持 0.8 不变 | PASS |
| 3 | 重开匹配规则 | 输入框显示 **0.95**（重开显示保存值，不是默认值） | PASS |
| 4 | 回 `/tags/1980` → 文章归档 → 点 event 标签 chip | 标签卡 `archive-match-popover` 打开，事件类别，无横向溢出（`scrollWidth == 1440`） | PASS |
| 5 | 展开「相关事件」 | 发一次 `GET .../tags/267115/related-events`（网络面板确认 200）；`候选 543 · 已索引 543 · 阈值 0.95`；**空态**「当前阈值与近 30 天范围内暂无相关事件。」（真阈值产生的空，不是伪造） | PASS |
| 6 | 恢复阈值 0.82 → 保存 → 重开卡片展开 | `候选 543 · 已索引 543 · 阈值 0.82`，10 条，按接口顺序；首条「Anthropic 推出 AI 智能体 Claude Duet 0.90」 | PASS |
| 7 | 点条目名展开描述 | 显示完整描述（「2026 年 10 月 22 日，Anthropic 推出基于 Opus 5.5 模型构建的 AI 智能体 Claude Duet…」），无横向溢出 | PASS |
| 8 | 点「查看来源」 | `ArticlePreviewModal` 打开，标题内容为对应来源文章「Anthropic 推出 AI 智能体 Claude Duet」（每日AI资讯｜AI工具集）；标签卡仍在底层 | PASS |
| 9 | 按 Esc | 预览关闭、**标签卡保留且列表仍是 10 条**（修掉了「同一个 Esc 连带关卡片」；回归用例见 `ArchiveMatchPopover.relatedEvents.test.ts`） | PASS |
| 10 | 布局锚点 | 卡片中心 `elementFromPoint` 命中卡片自身；「查看来源」按钮点击点命中按钮自身（未被遮挡）；卡片完全在视口内（`right=1149 < 1440`） | PASS |

截图：`1440-config-related-threshold.png`（配置字段）、`1440-related-expanded.png`（列表+长标题）、`1440-source-preview.png`（来源预览）。

## 状态覆盖（README 状态矩阵逐项）

| 状态 | 触发方式 | 断言 | 结果 |
| --- | --- | --- | --- |
| 未展开 | 打开 event 卡 | 无请求（网络面板零 `related-events`）、只有「相关事件 ▸」按钮 | PASS |
| 查询中 | 展开瞬间 | 局部「正在查询相关事件…」，原辅助标签/判定过程仍可读 | PASS |
| ready 空集 | 阈值 0.95 | 空态文案 + 覆盖数字（543/543） | PASS |
| not_ready | `network route` 返回 `status=not_ready, reason=source_not_indexed`（**响应为桩，本机索引已全量就绪，无自然未就绪样本；如实标注**） | 「相关事件索引尚未就绪（本条事件还没有向量），刷新索引后可再查。」且**不出现空态文案**；截图 `1440-related-not-ready.png` | PASS（桩） |
| 查询失败 | `network route --abort` | 局部「相关事件查询失败，请重试」+「重试」按钮；卡片其余内容照常；截图 `1440-related-error.png` | PASS（桩） |
| 重试恢复 | 解除 abort → 点重试 | 10 条 + 覆盖数字，无错误残留 | PASS |
| 非 event 卡 / 入板过程模式 | person 标签卡、入板 chip | 无「相关事件」入口、零请求（组件测试断言；浏览器只在 event 卡上验证了入口存在） | PASS（组件层） |
| 迟到响应 | 组件测试（切换标签后旧 promise resolve） | 旧响应不覆盖新标签 | PASS（组件层） |

## 三视口机械断言

| 视口 | 无横向溢出 | 浮层/弹窗可点 | 点击后到达状态 | 截图 |
| --- | --- | --- | --- | --- |
| 1440×900 | `scrollWidth == 1440`（列表、长标题、预览三态） | 卡片中心与「查看来源」命中本体 | 展开→列表；查看来源→预览；Esc→回卡片（10 条仍在） | `1440-*.png` |
| 1920×1080 | `scrollWidth == 1920` | 卡片与预览均 `elementFromPoint` 命中本体 | 同上（预览 1728px 宽，视口内、无溢出） | `1920-related-expanded.png` |
| 390×844 | `scrollWidth == 390`；卡片 `is-narrow`（371×658 底部形态，底部贴合 834/844） | 「查看来源」命中按钮自身 | 同上（预览 351px 宽，视口内、无溢出、Esc 回卡片） | `390-related-expanded.png`、`390-source-preview.png` |

## 长内容与可读性

- 事件标签 45 字符、「相关事件」条目最长 47 字符：条目名 `white-space: nowrap` + `text-overflow: ellipsis`，`title` 携带完整文本（实测 title 与全文一致）；展开后描述 `overflow-wrap: break-word` 可读。
- 来源文章标题在按钮旁以 ellipsis 展示、`title` 全文可达；无来源条目按钮 `disabled`，文案「暂无可读来源」，不伪造文章 ID。

## 对账清单（A 条目，供 `ui-verification-trace.sh` 对账）

### A-1 匹配规则新增「相关事件相似度」字段并保存生效【pass】
依据：1440×900 打开 `[data-testid="radar-more-matching"]`，`.app-dialog` 为 `is-sized`/760px，`[data-testid="mc-related-threshold"] input` = 0.82；改 0.95 保存后 `GET /api/semantic-boards/matching-config` 返回 0.95，重开弹窗仍显示 0.95；截图 `1440-config-related-threshold.png`。

### A-2 保存失败/非法输入保留输入并提示（不谎称已保存）【pass】
依据：组件测试 `front/app/features/tags/components/MatchingConfigDialog.relatedEvents.test.ts`（空值/1.5/-0.1 不发 save 且就地报错；`saveError` 保留输入并显示「保存失败，请重试」）；后端多字段 PUT 整体失败不部分落库见 `related_events_test.go`。

### A-3 event 标签卡按需展开相关事件，按接口顺序显示名称与相似度【pass】
依据：1440×900 点 `archive-tag-171120-267115` → 卡片 → 展开，发出 1 次 `GET .../related-events`（200），10 条按接口序，首条「Anthropic 推出 AI 智能体 Claude Duet 0.90」，覆盖行「候选 543 · 已索引 543 · 阈值 0.82」；截图 `1440-related-expanded.png`。

### A-4 查看来源走既有阅读预览，关闭后回到原标签卡【pass】
依据：点首条「查看来源」→ `ArticlePreviewModal` 打开对应文章标题；Esc 关闭预览后标签卡仍在且仍列 10 条（同一 Esc 不再连带关卡片）；截图 `1440-source-preview.png`。

### A-5 空态/未就绪/错误/重试四态可分辨【pass】
依据：阈值 0.95 下真实空态文案 + 覆盖数字；`not_ready` 与 abort 失败两态用 `network route` 桩触发（如实标注），分别显示「索引尚未就绪（本条事件还没有向量）」与「相关事件查询失败，请重试」+ 重试按钮；解除桩后重试恢复 10 条。截图 `1440-related-not-ready.png`、`1440-related-error.png`。

### A-6 三视口无横溢、浮层可点、长标题可读【pass】
依据：1440×900 / 1920×1080 / 390×844 三档 `document.documentElement.scrollWidth == innerWidth`；卡片与预览中心 `elementFromPoint` 命中本体；「查看来源」点击点命中按钮自身；45 字符标签名与 47 字符条目名 `text-overflow: ellipsis` + title 全文。截图 `1920-related-expanded.png`、`390-related-expanded.png`、`390-source-preview.png`。

### A-7 默认阈值上调为 0.86 后线上复验【pass】
依据：2026-11-11 09:4x 在最终产物（`bash scripts/dev/deploy-frontend.sh` exit=0，`✅ 部署完成`）上复验 1440×900：匹配规则输入框显示 **0.86**、字段说明含「默认 0.86」、弹窗仍 760px；event 卡展开相关事件为 **9 条**、覆盖行「候选 544 · 已索引 544 · 阈值 0.86」、状态 ready、无横向溢出；`GET .../related-events` 返回 `threshold=0.86`。截图 `1440-config-threshold-086.png`、`1440-related-threshold-086.png`。注：A-1/A-3 的原始断言（0.82/10 条）是 09:00 前那次验收的历史记录，按「历史执行不改写」保留。

## 未覆盖 / 已知限制

- not_ready 与查询失败两态用网络桩触发（本机索引已全量就绪，无法自然产生），渲染与恢复路径真实执行，数据是构造的。
- 未做视觉审美评审（按纪律视觉验收另派）；本报告只覆盖机械断言。
- 部署脚本第 3 步 60s 健康检查在本机过短（后端启动 ~110–190s），两次 `deploy-frontend.sh` 均以 exit=1 结束但静态产物已铺、后端随后健康；未修改部署脚本（不属本 change 辖区）。
