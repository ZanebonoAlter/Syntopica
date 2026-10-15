# 线索级关键标记 · 高保真单文件原型（FB-6）

打开/下载[index.html](index.html)即可离线交互，只需这一个文件。CSS、editorial/dark主题Token、fiction数据和脚本已内嵌；旁边CSS/JS仅保留编辑来源，更新来源后需同步内嵌段。正式Nuxt不能移植这份整页。

## 这版看什么

- 阅读器沿用户截图骨架：左索引/历史，中间章节头→小结→独立线索→底部生命线，右页边注。封面、目录、摘要锚、文章展开和1100px页边注抽屉保留，字号/留白/分隔/操作精修。
- 总览星标贴原节点基线，不再有单独低位“已标记”轴。同章A/B已确认后显示★2，点击逐条回看/独立取消；取消A后退为★1，B、原section标题/状态/计数/坐标不变。这是紧凑展示，不是整章重要状态或事件聚合。
- 日报才展示建议/理由/新确认、不采纳、手标；总览只普通来源与confirmed维护。无文章线索仍可合法标记，无有效topic禁标不建题；失效快照同轴但不伪造section/原文。

## 试法

默认打开AI当期日报。A/B/108共用5101章，依据2/1/0篇。标A不连带B；也可以分别标A/B。目录进入总览或关闭reader选话题总览，点节点旁★n看已标记线索，点原点看来源动态。有效来源可准确回原版块/日期/thread的日报。

顶栏主题切换、演示选项（读取/保存失败/长内容/重置）仍可用。全部还是文章池，不是全局轴。时间窗、泳道聚焦/返回保留；日期只来源日报日，不声称事件发生日。

## 当前预览

- [阅读器 · 深色1440](evidence/hifi-reader-dark-1440.png) / [浅色1440](evidence/hifi-reader-editorial-1440.png)
- [阅读器 · 深色1920](evidence/hifi-reader-dark-1920.png) / [手机375](evidence/hifi-reader-dark-375.png)
- [总览同轴 · 深色1440（展开两条）](evidence/hifi-overview-dark-1440.png) / [浅色1440](evidence/hifi-overview-editorial-1440.png)
- [本轮精简检查](evidence/hifi-report.md)

截图是已选A/B等演示状态，不是AI默认自动确认；演示选项可重置。旧FB-1–4截图/214 checks.json仍为历史，不是本轮高保真批准基线。

## 必要验证与隔离

```bash
node --check openspec/changes/add-topic-key-event-markers/ui-prototype/app.js
node --check openspec/changes/add-topic-key-event-markers/ui-prototype/verify.cjs
node openspec/changes/add-topic-key-event-markers/ui-prototype/verify.cjs
```

verify.cjs现为本轮精简冒烟：仅index复制到隔离临时目录；26条必要检查、9截图、finally关闭浏览器，结果hifi-checks.json。1440/1920两主题及375短路径覆盖同轴/独立取消/真实命中/窄屏44px/来源字段不变；不重跑旧214、不声称产品组件/PG/opencli/LLM验收。主题切换截图等待200ms，让既有按钮过渡稳定。

主题唯一源main.css；本章小结只是fiction摘要摘录，SVG生命线只是fiction日期/原状态/确认投影，不新建摘要字段或趋势算法。不接API/数据库/AI/远程字体；localStorage v3只存原型，数据/18与12标签数均fiction。沿用户已选择的人工看图，不重复尝试失败K3后台入口，也不宣称K3批准。

## 部署影响 / 审批

2026-10-12用户明确回复“好的，我认可了”，当前FB-6新版major/approval=approved，记录与原型hash见ui-design.md。此前选方向/继续打磨不是批准；本次明确认可仅记录UI审批，正式实现尚未启动。正式页面、旧数据、接口、部署均未改，无配置/清理/重生成操作。产品S1–S6/49场景仍独立待实现，原型检查不代替。
