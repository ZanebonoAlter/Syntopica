# G5 修复后的独立代码复核

复核者：原独立只读plan会话`01a17e6e-81c5-733f-b236-6e467e99d29b`，CommandCode DeepSeek V4.1 Flash max，非实现作者。仅读源码/用例结构，未复跑测试、构建、浏览器或真实业务操作。

## 裁决

R-27～R-33全部代码级通过，未发现修复新增High/Medium。父准入构建与只读S9；这不是整change/S9/视觉通过，8B.3/8B.4仍未勾。

| 条目 | 代码证据与复核结论 |
| --- | --- |
| R-27 | Shell不再写/还原html根字号；17px挂载卸载负向断言在位；§13伪字号依据已纠正。实际轨宽/角色字号待S9。 |
| R-28 | useConsoleShellData同板块刷新失败保旧标陈旧、换板块清理；useConsoleRefresh串行meta→A防重；生成dialog真实done上升沿completed/cancel/头部刷新接同链；mock已覆盖新日轴/0active/失败重试。真实DB提交与WS时序未实测，不计入S9绿。 |
| R-29 | pool错误/陈旧/重试透传Stage，手机分支外可达；成功才清提示、失败保旧，无自动风暴；独立365重试保A/档/泳道/范围断言在位，共享365缓存语义不变。 |
| R-30 | observation/lane先无写恢复，再合成单次完整query并await；URL watcher恢复committedRange，R22原例仍在。fake router异步提交只一次且四键齐全，显式/非法URL优先保持。 |
| R-31 | 真滚动宿主二选一存/还原，focus preventScroll；windowY120与stage80用例在位；1280/390实际返回仍待S9。 |
| R-32 | 设置/归档精确文案指向总览全局治理与当前板块管理；正反词用例在位，无页面重绘。 |
| R-33 | 候选openTopics经Body/Workface到本板块TopicManageDialog；编排未误开，原新建泳道入口保留；仅mock证明作用域，不真激活。 |

## 留账/实页重点

- 生成完成事件取自全局WS done，常挂对话框可能在04:00调度完成时多刷一次；completed后关闭可能先后两轮刷新。非状态错误，未新增轮询或业务写；暂不扩优化范围。
- 真实生成的DB提交与WS先后无法靠当前mock证明，本轮禁真生成；保留未验，不强迫用户为补UI重生成历史。需要时另行授权安全验证。
- 新顶部刷新/池失败重试属于minor评审收口，已记录ui-design/批准记录，不修改冻结原型hash。1440头部换行风险待量。
- S9必须量：1440/1920/2560根字号与左右轨、375/390双主题及池重试可达、365手柄真实hit区/拖选/取消、1280/390返回URL刷新与滚动/焦点。
- 作者命令证据：修复8文件119+生成事件1，后续3文件54重叠回归；TS lint0；typecheck仍因他域7条SignalReportView.test.ts旧错exit1。独立复核者未复跑，不合并成虚假唯一用例总数。
- 根53pass/1fail/2未覆盖与旧关系/归档/他域债保留；尚无本矩阵build/静态S9/发布结论。
