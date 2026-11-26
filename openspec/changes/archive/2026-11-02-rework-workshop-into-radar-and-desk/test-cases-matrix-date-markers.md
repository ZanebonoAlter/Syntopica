# test-cases: 矩阵内视觉与关联日期黄点（8E）

## 故事 S1：从其他话题在现有日期轴读同归属记录

锚 Requirement：当日其他话题折叠组与矩阵关联日提示；继承 8C 数据/排除/返回契约，替换未批准旧 8D 呈现，不采用新面板。

| Scenario / 节拍 | 断言 | 最便宜测试层 |
| --- | --- | --- |
| 矩阵其他组复用泳道视觉且只绘当日格 | 仍在网格、同一共享行/格样式、非当日无格 | ObservationMatrix.groups / DayOtherTopicsGroup / 浏览器亮暗宽窄复核 |
| 同 ID 关联日黄点与真实标题 | 过滤异 ID/无 ID/窗外、同日一个点、全部真实原题、首 section | useOtherTopicObservation + ObservationMatrix.dayHeaders |
| 黄点点击原子保留阅读目标且普通日期不变 | yellow→稳定目标事件，普通→原报日事件，跨日不被 watcher 清空 | ObservationMatrix.dayHeaders + otherTopicSelection.select + BoardConsoleShellSelection |
| 换选清点与观察诚实降级 | 当前选择 gate、无 ID/加载/失败/陈旧，状态/重试仍在 | OtherTopicReading.observation + BoardConsoleShell.integration（换选/无 ID/清场/窄屏） |
| 缺索引观察日黄点点击不被回落撤销（review R-41） | 日期与目标原子保留；无报日时诚实 no-report 态；普通日期/缩窗/切板块/切泳道/清场回落语义不变 | BoardConsoleShell.integration（桌面/窄屏/连续第二次）+ laneConsoleView + useConsoleSelection + otherTopicSelection.select |
| 阅读区去重并复用泳道正文视觉 | 无日期药丸，真实标题/全量 threads/空摘要/相关文章不变 | OtherTopicReading.observation / OtherTopicReading / OtherTopicReading.signals |
| 零正式泳道仅保留有用阅读方向 | 无选泳道卡、有选日/话题提示 | BoardConsoleShell.integration（0 泳道） |
| 历史成员与返回上下文不回归 | 原排除/快照/返回不动 | ObservationMatrix.groups + BoardConsoleShellNavigation |

### 白盒分支与边界
- 选择 none / missing-id / 同 ID / 换 ID；观察 idle/loading/error/ready/stale；旧 view topicId 不匹配→无点。
- 同日 0/1/多分区；重复日期只一个节点、峰值统计不变，原序首 ID 目标不变；标题逐字数组而非合并正文。
- axis 外/异 ID/无 ID 记录不进入；可见窗切换仅本地投影、板块缓存不增加请求。
- 标记日点击只 emit observed 事件；普通报日只 emit header；普通无报日仍不可点；focus 可读 tooltip/aria。
- 阅读加载/缺分区/失败不借旧正文；取消选择马上清点；0 正式且无其他选择不挂演进空卡。
- 无 ID 不发观察请求；零业务写、不接 relations、不跨手动新 ID 补连。
- **已验证记录日（R41）**：只有经黄点选中的那一个日子受保护（未接线/无目标/保护别的日→普通回落）；
  保护不越过轴边界（缩窗排出照旧）；清场路径（普通选行/清场/切泳道/切板块/换日）逐项失效；
  目标实际无日报 → 保留日期与目标并显 no-report 诚实态（不借旧正文、不退回别日）。
  **复现口径**：选中日校正是值变化驱动，回落值恰等于回落前选中日时看不出搬走 → 复现须**从非默认
  回落日出发**（先切到另一个真实报日，再点索引缺该日的观察日）。

### 人工效果核对（主线程待执行，不冒充通过）
1440×900 / 1920×1080 + 窄屏、亮暗主题：行高/标题/实底格/选中一致；黄点 hover/focus 可读，正文无重复日期条且无伪滚动态势。入口 `/tags/246030` 冷启动与 `/tags/1974` 成熟板，历史 2026-10-07–24 数据按实际 GET/现有日期控件选取。

### review R-41 修复批的用例映射（2026-10-30）

| 断言 | 落点测试 |
| --- | --- |
| 桌面黄点点击：日期与目标原子保留（索引缺该日也不回落到别的报日）、无报日时诚实 no-report 态 | front/app/features/tags/components/console/BoardConsoleShell.integration.test.ts |
| 窄屏日期条黄点点击：同上（同源链路） | front/app/features/tags/components/console/BoardConsoleShell.integration.test.ts |
| 连续第二次黄点点击：日期与目标原子换到第二个观察日（不被排队 watcher 竞态清空） | front/app/features/tags/components/console/BoardConsoleShell.integration.test.ts |
| 校正器口径：已验证记录日保留 / 未接线·无目标·保护别的日→普通回落 / 轴外仍回落 / 索引未就绪口径不变 | front/app/features/tags/components/console/laneConsoleView.test.ts |
| 保护接线：已验证记录日不被搬走；保护撤销后回落 | front/app/features/tags/components/console/useConsoleSelection.test.ts |
| 保护生命周期：黄点选择写入；普通选行/切泳道/换日/切板块立即失效 | front/app/features/tags/components/console/otherTopicSelection.select.test.ts |
