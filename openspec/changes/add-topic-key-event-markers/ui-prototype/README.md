# 日报挑选 → 版块泳道回看原型

FB-3：AI可能每期推荐，未定结果不带进长期视图。**日报负责挑选，话题总览只回看新闻与人工confirmed高亮。** 总览仍按FB-2限于当前版块，配色继续取现有双主题。

## FB-4 核实后的适用范围

已实看运行中日报：现有全屏报纸阅读器里一个section有多条DailyReportThread，每条自带标题/摘要与文章展开。本目录旧模型一thread=一section、日报扁平列表，不能作为当前日报接入/审批基线。本轮只读核实和更新制品，没有改这份JS/HTML；148检查仍只属简化交互，不证明新一章多线索场景。下一版应在真实线索行加同级控件、明确A/B独立，而不是再做扁平事件页；仍major/pending。

## 打开

浏览器打开index.html；复制/下载时保留index.html、style.css、theme-tokens.css、app.js同目录。不需要构建、产品服务或远程字体。

```text
file:///home/zanebono/software/Syntopica/openspec/changes/add-topic-key-event-markers/ui-prototype/index.html
```

## 推荐试法

1. 默认AI与大模型版块：只显示LLM/Agent两泳道。发布/API开放等未确认条目只是普通新闻，不显示AI建议徽标、理由、虚线候选或挑选按钮。
2. 点普通节点看原位详情，再点「去来源日报挑选」：定位同一版块、正确来源日期和对应条目，才显示AI建议/理由、确认或不采纳。无建议也可在日报「标为关键」。
3. 确认后点话题总览：已确认外环/星标出现，中心原状态色、来源身份/日期位置不变；不带其他期的未确认推荐。突出开关仍保留原新闻背景。
4. 日报日期选择复用原有概念，原型以原生select示意七个演示日（不是新日历管理）。切到历史日期可挑该期；空日不造条目；普通来源的导航会准确定位日期/thread。
5. 不采纳或取消后来源新闻仍在；想重新选去日报。总览保留已确认取消作为纠错维护，失效快照也能取消，没有假日报锚/原文/section。
6. 切左侧住房与信贷，只有房贷/供应两泳道；全部仍是文章池说明，不是全局轴。点泳道名聚焦，返回仍同版块。
7. 试双主题/手机/长文/读取/保存失败。保存期间切日期、总览或版块：合法旧写仍存原owner，不把旧期/旧板块提示塞进新视图，同owner确认更新回看。

新闻、来源日期、数值均为fiction fixture。左侧18/12是演示tag_count，不是关键数量；源clusterLabel与thread.title同值仅为演示，不代表产品可混同字段。

## 本轮预览

- [日报挑选 · 桌面](evidence/daily-review-editorial-1440.png)
- [日报挑选 · 手机浅色](evidence/daily-review-editorial-375.png) / [深色](evidence/daily-review-dark-375.png)
- [总览回看 · 浅色](evidence/board-ai-editorial-1440.png) / [深色](evidence/board-ai-dark-1440.png)
- [住房版块](evidence/board-housing-editorial-1440.png)
- [完整验证记录](evidence/report.md)

以前总览带AI候选的截图/绿色混合版块截图均已被替代，不是本轮实施或审批基线。

## 现有主题与状态色

唯一来源front/app/assets/css/main.css。generate-theme.cjs复制:root/editorial/dark变量块，输出theme-tokens.css及带source/hash的theme-source.json。style.css只引用语义Token，不自行配品牌色；正式Vue应直接用原main.css/useTheme，不移植这份HTML/JS/复制主题文件。

| 主题 | 既有背景 | 既有强调 |
| --- | --- | --- |
| editorial | #faf7f2 / #f5f0e6 | #d94a4a |
| dark | #080c12 / rgba(17,27,38,.98) | rgba(240,138,75,.85) |

中心点使用原statusFill五态Token，关键性只叠外环/星标。fixture的threadStatus模拟section.status，不是新增Thread状态。

## 可重复验证

```bash
node openspec/changes/add-topic-key-event-markers/ui-prototype/generate-theme.cjs
node --check openspec/changes/add-topic-key-event-markers/ui-prototype/app.js
node --check openspec/changes/add-topic-key-event-markers/ui-prototype/verify.cjs
node openspec/changes/add-topic-key-event-markers/ui-prototype/verify.cjs
```

最后实测退出0，**148条原型机械断言**，12张当前截图；checks.json status=passed、browserClosed=true。包括推荐只能日报、总览负断言与普通来源正断言、正确日期导航、跨入口同步、在途切期/板块、确认取消/失败、原字段/计数/颜色不变，以及六视口两主题长文、真实按钮命中和新增日报窄屏入口44px。每轮finally关闭浏览器，不起dev服务。

## 隔离与审批

localStorage专属v3，键board:lane:id；投影改变而存储形状未变，重置只影响原型。无产品API/数据库/AI调用，存储不可用仅当页保留。其他tab静态占位，文章池/时间线/侦探墙/回刷/合并/新建泳道/版块治理不在演示范围，不意味着可删存量功能。

仍major、approval=pending；反馈修订不等于批准。正式页面、主题源、旧数据和部署未改，无用户清理/重生成/配置。原型不代替产品静态:5100 opencli、PG testcontainer或真实AI效果评估；请明确确认新版，再另行启动实现。
