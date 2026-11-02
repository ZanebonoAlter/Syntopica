<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

# UI 复用契约

## 入口与变更

入口仍为板块「研究」tab。保留现有主从索引、候选详情、报告版本选择、忽略/恢复、话题背景和历史入口；没有新页面、弹窗、导航或多步向导。
- 原候选详情“研究问题/为什么值得研究”下增加两段文字：“还不清楚什么”“准备先查什么”，来源为已保存 research_brief。列表只保留简短主问题和原因，不增信息密度。
- 原“研究”按钮附近以现有说明文字提示：新协议可能使用已配置的外部搜索；未配置不阻止本地解释，不声称已经查过。旧协议提示“沿用原快照；使用新流程请重新发现”。
- 报告原四段与核查附录保留。thesis开始处显示来自保存answer字段的“已解释 / 部分回答 / 暂无法判断”及理由；它是答题范围，不是job状态、可信度分或评审。
- 原依据引用交互扩展文字来源：点击source引用进入现有核查区，显示冻结标题/URL/使用片段/发布时间或未知/取回时间/事后核查/截取范围。外链仅http(s)，用安全新窗口；网页正文作为文字，不渲染任意HTML。
- 新字段缺省时不伪造徽标或未知项；旧schema2/schema3仍按原样读。数字引用、表格和图表仍走原路径。

## 受影响状态

| 状态 | 展现与反馈 |
| --- | --- |
| loading | 保留真实research/compose进度与禁重复提交；新检索/读取计入工具执行，不把本地快照准备标成模型研究 |
| empty | 无候选仍是成功空态；无存文显示“缺少可读取的原文”，无数字显示既有无可用数据期，不冒充外搜已成功 |
| error | 来源请求失败在原缺口/核查区显示，仍可得到有限结论；整个job取消/超时仍failed，可手动重试；获取详情失败不是空列表 |
| success | 可解释、部分回答、未解决均能形成成功报告；原因与支持来源可查，不显示“已经证实”认证 |
| legacy | 无brief/answer/sources字段时保持旧阅读；旧重新研究不升级协议，重新发现为既有入口 |
| long / repeated | 长中文、连续长URL折行；引用点击后的核查目标在视口内；连续点研究仍只发一个有效任务并提示现有互斥 |

误输入反馈沿用现有周期校验，不新增表单；无效引用不生成可点假来源。来源地址不是可执行动作指令。

## 复用组件与布局模式

复用前置change的 AppPageShell split 主从布局、AppButton、SignalCandidateList、SignalReportView、reportPresentation/signalReport、useSignalWorkbench，不新增Store或公共组件体系。没有新dialog，不引入自由宽度。editorial/dark沿用语义Token，不加新状态颜色语义。仅在现有详情文本与附录项增加字段，因此ui-impact=minor；若实现要求新增流程或分区，应先修订为major并申请原型批准。

## 验收映射

- `front/app/features/tags/components/SignalCandidateList.test.ts`：新旧字段、说明、长内容、运行禁用/忽略允许性回归。
- `front/app/features/tags/components/SignalReportView.test.ts`：三种答题范围、缺字段兼容、来源定位、失败/空态。
- `front/app/features/tags/components/reportPresentation.test.ts`：source引用与原data/calc不互串、非法URL不链接、HTML转义。
- `front/app/features/tags/composables/useSignalWorkbench.test.ts`：保持默认200、显式重研究202、409与404停止轮询行为。
- 实现后 agent-browser 在静态 :5100 走“发现后选择候选→显式研究→读结果→点来源→返回→读旧报告”一条故事；真实模型步骤须先获授权，UI边界变体用隔离fixture，禁止测试写真库。1440×900与390×844，editorial/dark检查长URL、运行中状态、来源展开后的scrollWidth与目标可见性。截图作为佐证，断言为准。
- 在 `ui-verification/report.md` 以 A-N + pass/fail/未覆盖 + 依据建账；本轮仅规划，未创建假验收通过记录。无需可丢弃原型。
