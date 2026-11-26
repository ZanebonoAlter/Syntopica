# 8E 矩阵关联日黄点：父线程实页验收

## 范围与环境

- 2026-10-30，静态托管 `http://localhost:5100`，只读交互，无生成/回填/业务写入。
- 首次 `bash scripts/dev/deploy-frontend.sh` 成功，generate 完成，后端重启后 `/health` 200。
- 自动化会话 `matrix-markers`，验收结束已主动关闭；未启动 dev server。
- 本记录不是整 change 归档验收，不覆盖旧场景映射欠账或其他功能。

## 已验证的正常链路

| 故事 | 操作与实际结果 |
| --- | --- |
| 无正式泳道也可读 | 卫生健康 `/tags/246030`，14 日窗 10/16—10/29，选10/16再点其他话题行；正文可读，不出现空泳道阅读卡。 |
| 关联日真实标题 | 选中后10/16、10/22、10/23、10/24共4日亮点；日期按钮 aria-label/title 包含对应日报原题，不按标题相似补关联。真实历史同ID内容跨度较大，不将其称为同一事件。 |
| 连续跨日 | 桌面依次点击10/22、10/23黄点，选中日期和对应正文同步变化，黄点仍保留，未回落到最新日。 |
| Tooltip | 1920×1080深色，悬浮10/22，浮层显示日期和真实标题；body Teleport 未被横滚容器裁切。浮层 x=896，y=474，w≈215px，在视口内。 |
| 窄屏承接 | 375×812深色，同一选择保留到窄屏日期条，点10/24黄点可直接读对应分区，4个标记仍在。 |
| 普通日期 | 窄屏点未带黄点的10/29，标记归零，旧10/24阅读目标清除。 |
| 页面溢出 | 1920桌面及375窄屏 `document.documentElement.scrollWidth > innerWidth` 均为 false。 |
| 双主题 | 1440×900浅色与1920×1080深色均展示实底当日格与日期标记；标记使用 warning token（本机计算色 rgb(196,136,60)），选中仍沿用 accent 边框。 |
| 成熟板对照 | 中东 `/tags/1974`，既有正式泳道矩阵保持，日期可选，其他话题仍为折叠组，不迁至独立面板。 |

## 截图

- `matrix-markers-light-1440.png`：1440×900浅色，选中10/16后的矩阵和黄点。
- `matrix-markers-dark-1920.png`：1920×1080深色，连续跨日后选中10/23。
- `matrix-markers-dark-375.png`：375×812深色，点10/24后的窄屏阅读。
- `matrix-markers-mature-1440.png`：中东成熟板矩阵对照，截取时为切日附近状态，不单独作为详情读取完成的证据。

## 独立审查与最终修复复验

独立只读review发现：成功报日索引缺少某个 observation 日期时，日期校正器可能回落并丢失目标。已按 `review-matrix-date-markers.md` R-41 修复：只有当前黄点指定、仍在可见轴内的真实记录日可豁免普通日期回落；轴外仍回落，切板块/泳道/普通日期释放保护。父线程核阅 `otherTopicSelection.ts`、`useConsoleReadingWiring.ts`、`useConsoleSelection.ts`、`laneConsoleView.ts`，确认同步接线和清场均已落地。

修前桌面/窄屏复现测试为红，修后以下命令通过（子线程实跑结果）：

- `cd front && pnpm test:unit app/features/tags/components/console/BoardConsoleShell.integration.test.ts app/features/tags/components/console/laneConsoleView.test.ts app/features/tags/components/console/useConsoleSelection.test.ts app/features/tags/components/console/otherTopicSelection.select.test.ts --maxWorkers=2` → 4 文件 / 61 用例通过，新增10例。
- `cd front && pnpm test:unit app/features/tags/components/console --maxWorkers=2` → 49 文件 / 582 用例通过。
- `cd front && pnpm lint && pnpm exec nuxi typecheck` → 均 exit 0；lint 的8条既有他文件 warning 未当作本轮错误。

父线程再次执行 `bash scripts/dev/deploy-frontend.sh` → exit 0，后端健康200；日志 `/tmp/syntopica-matrix-markers-deploy-final.log`。在新构建上重新打开真实 `/tags/246030`，1440×900，点10/16其他话题后依次点击10/22、10/23日期表头：最终 `selected=[2026-10-23]`、标记数4、正文动作含 `打开 2026-10-23 日报`、无页面横向溢出。截图 `final-1440.png`，会话 `matrix-markers-final` 已关闭。

缺索引边界由组件集成测试构造，不改真库制造故障；正常真实资料链路由 agent-browser 验收。索引缺日报时阅读槽仍保守展示 no-report，不新增按 timeline reportId 绕过现有报告读取口径的能力。

全change `scenario-trace.sh` 仍有117条场景中109项历史映射欠账；本轮7条均有精确映射，未新增缺口。此轮不归档整个change，不将旧欠账报告为已通过。

## 部署影响与操作

其他话题保留矩阵，行/格及选中样式与正式泳道复用；同ID关联日期在选中后显示标记，重复药丸导航撤下。无数据迁移、配置或重生成；无稳定ID的旧内容继续可读但不显示关联点。
