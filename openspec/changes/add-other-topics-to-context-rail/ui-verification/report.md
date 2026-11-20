# UI 验证账（add-other-topics-to-context-rail）

> minor 档：L1 `/tags/:boardId` 右轨「当日日报」块**内部**新增「当日其他话题」子块（≥1440px 才显示）。
>
> **验证环境**：本机 dev 栈（`bash scripts/dev/start-dev.sh`，前端 :3000 + 后端 :5100，直连形态 `NUXT_PUBLIC_API_BASE=http://localhost:5100/api`），
> agent-browser 无头 Chromium，版块 `美国新闻`（board 2197），矩阵选中日 `2026-11-19`（与用户反馈截图同版块同日）。
> 实测数据：当日日报 3 条点评 + 2 条正式泳道（3 条 / 2 条）+ 当日其他话题 6 个分区合计 10 条线索。

### A-1 宽屏 2560×1440：右轨「当日其他话题」子块渲染并填掉原留白【pass】（依据：`ui-verification/2560x1440-rail-other-topics.png`）
依据：agent-browser 在 2560×1440 实测子块 display=block、徽标「10 条线索」、6 行合计 10、留白由 438px 降到 86px、页面 scrollHeight=1440=clientHeight（截图 ui-verification/2560x1440-rail-other-topics.png）

实测（agent-browser `eval` 读真实 DOM 几何）：

| 指标 | 实测值 |
| --- | --- |
| 子块 `display` | `block`（≥1440px 门生效） |
| 标题 | 「当日其他话题」 |
| 计数徽标 | 「10 条线索」 |
| 行数 | 6 个分区 |
| 各行 | 美联储10月纪要… 1 条线索 / 特朗普深圳行… 4 条线索 / 沙特对印度石油出口量… 1 条线索 / 美国家安全局IT系统故障… 1 条线索 / 外国投资者9月美债持仓… 2 条线索 / 哥伦比亚总统拒批… 1 条线索，状态词均为「当时待观察」 |
| 口径自洽 | 各行合计 1+4+1+1+2+1 = **10** = 徽标 10 ✓ |
| 量纲分家 | 泳道层为「3 条 / 2 条」，子块为「N 条线索」（不同单位后缀、不同子块）✓ |
| **留白填充** | 无子块时泳道层底部到「话题追踪」块 **438px** 空白；有子块后 **86px**；438 − 352（子块实测高）= 86，精确吻合 |
| 页面高度 | `documentElement.scrollHeight = 1440 = clientHeight` → **页面未被撑高**（右轨自身滚动） |

### A-2 宽屏 1440×900：子块渲染、页面高度不变【pass】（依据：`ui-verification/1440x900-rail-other-topics.png`）
依据：1440×900 实测子块 display=block 高 386px、徽标「10 条线索」、6 行、页面 scrollHeight=900=clientHeight（右轨自身滚动），截图 ui-verification/1440x900-rail-other-topics.png

| 指标 | 实测值 |
| --- | --- |
| 子块 `display` / 高 | `block` / 386px |
| 徽标 / 行数 | 「10 条线索」/ 6 |
| 页面高度 | `scrollHeight = 900 = clientHeight` → **未被撑高**；右轨 `scrollHeight 1306 > 高 804` → 右轨内部滚动（改动前右轨内容已超一屏，非本 change 引入） |

### A-3 1200–1439.98px 与 375px：子块不出现、布局零变化、无横向溢出【pass】（依据：`ui-verification/1200x900-no-subblock.png`）
依据：1439×900 / 1200×900 / 375×667 三档实测子块 display=none 且占高 0，均无横向溢出；截图 ui-verification/1200x900-no-subblock.png 显示「各泳道」后直接是「话题追踪」

| 视口 | 子块 `display` / 占高 | 页面 `scrollWidth > clientWidth` |
| --- | --- | --- |
| 1439×900 | `none` / 0px | 否 |
| 1200×900 | `none` / 0px | 否（截图中「各泳道」之后直接是「话题追踪」，与改动前一致） |
| 375×667 | `none` / 0px（右轨整体不渲染，走 `BoardNarrowSummary`） | 否 |

三档 `display: none` 在 flex 容器中不占高度 → 与「≥768px 视口布局与交互零变化」回归红线一致。

### A-4 点行定位：右轨点某行 → 主区既有阅读槽读出该分区【pass】
依据：2560×1440 下点右轨第 2 行后 `[data-testid="other-topic-reading"]` 渲染出对应分区标题与 5 条线索（见下段实测记录；同图 `ui-verification/2560x1440-rail-other-topics.png`）

实测：在 2560×1440 下点右轨第 2 行「特朗普深圳行：会晤普京、拜访马科斯、会见习近平」（4 条线索）后，
舞台阅读槽 `[data-testid="other-topic-reading"]` 渲染，标题为「当日其他话题 · 2026-11-19 特朗普深圳行：会晤普京、拜访马科斯、会见习近平」，
线索列表 5 条（`lane-chronicle-thread-19029…`），并带「打开 2026-11-19 日报 ↗ / 看此日归档 / 同归属记录日 · 5 天」。
全程未新开浮层/弹窗；右轨自身不改选择态（走既有 `selectOther` 链）。

### A-5 工作面 context 页签（review R-1 修复后）【pass】
依据：`BoardWideWorkface.test.ts`「context 页：当日其他话题点行与当日日报重试均外抛给壳层（与 locate-lane 对齐）」用例在盘并通过（组件测试 `pnpm test:unit app/features/tags/components/console/BoardWideWorkface.test.ts`）

`BoardWideWorkface` 的 `BoardContextRail variant="full"` 在 ≥1440px 视口下同样渲染该子块，
修复前 `@select-other` / `@retry-now` 未接线（点行与重试是死键）；已补接线并由
`front/app/features/tags/components/console/BoardWideWorkface.test.ts` 的
「context 页：当日其他话题点行与当日日报重试均外抛给壳层（与 locate-lane 对齐）」机械锚住。

### A-6 静态托管产物（deploy-frontend.sh 铺设后）复验【pass】（依据：`ui-verification/static-2560x1440.png`）
依据：`bash scripts/dev/deploy-frontend.sh` 铺设产物后，agent-browser 直开 `http://localhost:5100/tags/2197`（2560×1440）实测子块 display=block、高 352px、徽标「10 条线索」、6 行、泳道层「3 条 / 2 条」；「当日日报」块底部到「话题追踪」的间距由隐藏时的 572px 降到 220px（572 − 352 = 220，精确等于子块高）；页面 scrollHeight=1440=clientHeight 未被撑高（截图 ui-verification/static-2560x1440.png）

> 说明：A-1/A-2/A-3 的几何实测取自 dev server（HMR 同源代码）；A-6 为静态托管产物的独立复验。
