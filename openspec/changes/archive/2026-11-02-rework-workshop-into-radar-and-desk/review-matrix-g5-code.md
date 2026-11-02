# 矩阵 G5 首批独立代码审查

来源：只读plan子线程 `01a17e6e-81c5-733f-b236-6e467e99d29b`，`commandcode/deepseek/deepseek-v4.1-flash:max`，不是实现作者上下文。未写代码、未测试/构建/浏览器/真实API/SQL。本报告不是实页验收证据。

父抽核：Shell根字号与生成对话框关闭、useConsoleShellData元信息生命周期、useConsoleReturnContext连续query写入与滚动宿主，均有实际代码对应。其余沿独立审查证据进入定向复现；作者若不同意须举源码与用例，不盲改。

## 裁决

**先修 R-27～R-33，再进入 build/只读S9。** 两High、三Medium属于主链路，不只修前两项；两Low为旧指引/错误动作落点，可同批修。308方法/组件用例绿不能替代完整交互故事绿。8B.3/8B.4不勾。

| 主review | 优先级 | 文件/证据 | 用户影响与验收 |
| --- | --- | --- | --- |
| R-27 | High | BoardConsoleShell.vue:367–377 把html.fontSize写成clamp(15px,.86vw,30px) | 与批准按角色字号冲突；2560推算根22px，17rem/21rem轨宽约374/462px（这是CSS算术，非实页测量）。移除全局根字号写入/恢复；局部角色按token/批准样式，不统一倍乘整页。单测挂载/卸载不改html字号，实页后量轨宽。纠正§13伪称ui-design有此公式的文字。 |
| R-28 | High | useConsoleShellData.ts:49–79/96–103 元信息仅mount/换board读取；Shell.vue:461–465 generate cancel只load A；G1 anchorDate非空reportAnchor优先 | 已有历史再生成后metadata/源轴/最新入口停旧日、可能排除新日且声称之后无日报。暴露并在生成关闭/显式刷新串联真实metadata与A读取，保0active候选日报；失败可重试，保同板块最后成功元信息/标陈旧，不拿有限365空当历史不存在。不要仅max(activeDate,metaDate)掩盖候选新报。用mock模拟新报告，不真生成。 |
| R-29 | Medium | useObservationWindow ensurePool失败锁；retryPool/poolError/poolStaleNotice未传到Stage | B失败只剩?且A重试永远不重发B。接真实错误/陈旧提示与独立retryPool，确认仅B重发、A/名单/选中上下文不被改变，成功恢复历史。不要靠无条件清错假恢复/自动重试风暴；365源与B共享缓存按现有语义处理。 |
| R-30 | Medium | useConsoleReturnContext.ts:118–122先applyRange再selectLane；各自展开未更新route.query | 真vue-router异步replace会取消前一导航，裸研究返回后URL可能丢start/end，显示与链接不同步。合并一次完整有效query或确实等待导航（await返回void的setter无效），显式URL/非法输入规则保留。测试fake router异步提交、返回后刷新仍保days/start/end/lane。 |
| R-31 | Medium | useConsoleReturnContext.ts:66–67 stage.scrollTop??window.scrollY；Body在<1440舞台overflow visible | 非滚动stage值0不触发??，手机/1280记录0；恢复又focus不preventScroll可拉走。按实际滚动宿主记录/恢复，仅一个宿主，并preventScroll；单测stage非滚动且windowY=120应存120、桌面真实stage应存stageY。实页1280和390往返核对。 |
| R-32 | Low | BoardSettingsForm.vue:241、BoardArchiveView.vue:337/349旧“更多/治理右轨”指引 | 指向已撤入口。仅精准文案改实际总览全局入口与本板块管理，别重绘归档/其他页；补文案断言。 |
| R-33 | Low | BoardContextRail.vue:180–184“话题候选/人工激活”触发openCompose，新建泳道而非TopicManageDialog | 文案承诺错落点。父裁决保持候选人工激活语义，接既有本板块话题管理，不改名把它解释成编排；新建泳道既有入口仍保留。只证明对话框/board scope与回调，不真激活。 |

## 静态已成立的部分（不替代实页）

A/B名单分离、稳定ID join、G4 R24 gate、右轨实体区分、watch板块作用域、全局治理回迁，审查者未找到阻塞；不是只新增文件。其它页面旧债/SignalReportView.test.ts七条旧类型错与本次区分，不洗根53/1/2。

## 后续实页必查

1. 1440/1920/2560 root computed字号/左右轨宽/角色字体；手机375/390双主题。
2. 365曲线双端手柄：SVG宽10/viewBox1000在375可能仅约3.75px；确认真实命中区（可能另有hitbox，不先当确认bug），机械测量、真实pointer拖选/取消，不只截图。
3. 研究返回URL保start/end/lane与刷新稳定；1280/390实际滚动宿主和focus preventScroll。
4. 元信息新日更新与B失败后独立恢复可在组件/拦截GET故障场景验证；不得为验收真生成/回填/收编/激活/删除或改数据。

仍未build/实页/部署，不能声称S9或视觉通过。

## 修复批回填（2026-10-28）

R27–R33 已按本报告与 `review.md` 定向修复并补受影响用例；逐条源码/用例记录见 `review.md` 对应条目，状态均为「已修复（待父复核）」。本批不代表整 change 完成，8B.3/8B.4 仍未勾；未 build/实页/部署。

## S9 首轮真实验收返修（2026-10-28 R34–R39）

R34–R39 已按 `ui-verification/matrix-stage-2026-10-28/parent-triage.md` 定向修复并补受影响负向用例；逐条源码/用例记录见 `review.md` 对应条目，状态均为「已修复（待父复核/实页）」。未改 spec/test-cases/原型，未 build/实页/部署；真实 1440/1280/390 滚动、≤31 几何、手柄 elementFromPoint、computed uppercase 留父构建后 K3；8B.3/8B.4 未勾。
