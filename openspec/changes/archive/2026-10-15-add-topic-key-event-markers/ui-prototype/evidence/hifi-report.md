# FB-6 高保真 / 同轴标记 · 原型必要检查

当前预览基线是hifi-*.png与all-in-one index.html。旧FB-4的214/16次截图是历史，不算本轮重新执行。仅fiction原型；检查当时major/pending。2026-10-12用户随后明确认可FB-6，现major/approved，审批记录见ui-design.md。

## 命令与期望

```bash
node --check openspec/changes/add-topic-key-event-markers/ui-prototype/app.js
node --check openspec/changes/add-topic-key-event-markers/ui-prototype/verify.cjs
node openspec/changes/add-topic-key-event-markers/ui-prototype/verify.cjs
```

期望exit=0，hifi-checks.json status=passed/browserClosed=true。实际26条必要检查通过、9张截图；明细和时间见[hifi-checks.json](hifi-checks.json)。新版verify.cjs是精简检查，不再跑旧214。没有dev服务。

- 单文件隔离打开：只有复制的index.html，内嵌脚本语法可执行，无外部CSS/JS或HTTP资源请求。
- 章内三线索、依据2/1/0、同级button，摘要t:101锚/跑题保留；展开A仅两篇，不确认。标A不连带B/父字段。
- reader遵用户骨架顺序：章节小结→多线索→下方生命线。1440×900/1920×1080两主题+375×812深色，正文无横溢，标记按钮elementFromPoint命中；375新增动作44px，无文章header disabled不误禁有效mark。
- 总览原点与★n中心纵坐标差<1px；5101仍一个真实section，★2保存101/102独立身份。点开两条confirmed，不含建议/新确认；取消A仅退到★1、B/源字段/原点保留，返回徽标恢复焦点。
- 突出前后原section、时间坐标、状态色不变。1440/1920/375总览无全页横溢、星标真实命中。来源失效105独立同轴快照，无假section。

## 图像与阅读方式

reader截图定位本章（不是封面），总览截图包括同轴与逐条详情。尺寸是viewport，不声称一屏容纳全部内容；CSS主题切换后等待200ms避免按钮过渡中间帧作为颜色证据。截图只供人工看图，机械检查不以目测作为通过依据。

- [阅读器 深1440](hifi-reader-dark-1440.png) / [浅1440](hifi-reader-editorial-1440.png)
- [阅读器 深1920](hifi-reader-dark-1920.png) / [浅1920](hifi-reader-editorial-1920.png)
- [阅读器 375](hifi-reader-dark-375.png)
- [总览 深1440 / 两条展开](hifi-overview-dark-1440.png) / [浅1440](hifi-overview-editorial-1440.png)
- [总览 深1920](hifi-overview-dark-1920.png) / [375](hifi-overview-dark-375.png)

视觉安排延续用户明确人工接管；K3后台host缺失未修，未再派发或伪造审查。记录在旧report.md，仅本轮原型供用户确认，不是正式产品门禁。

## 部署影响与边界

正式应用/主题源/接口/真库/旧数据/部署未修改，无用户操作。章节小结摘录fiction既有摘要、SVG绘制fiction原日期/状态/标记，不是新增字段、真实趋势或AI效果。产品组件/静态:5100 opencli/PG/真实材料量化与49场景仍待实施。用户随后明确批准当前新版，approval=approved；这是UI审批，不代表产品已完成或部署，正式实现尚未启动。
