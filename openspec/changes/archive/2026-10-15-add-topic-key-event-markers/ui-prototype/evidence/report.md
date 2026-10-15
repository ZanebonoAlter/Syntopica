# FB-4 原型证据 · 真实reader / 一章多线索

替代FB-3扁平日报的148条简化模型基线。**仅fiction静态原型，不是Nuxt/HTTP/PG/真实AI验收；major/pending。** 机械验证通过，K3图审启动阻断未执行，两者分别记账。

## 实际执行与机械结果

```bash
node --check openspec/changes/add-topic-key-event-markers/ui-prototype/fixtures.js
node --check openspec/changes/add-topic-key-event-markers/ui-prototype/app.js
node --check openspec/changes/add-topic-key-event-markers/ui-prototype/verify.cjs
node openspec/changes/add-topic-key-event-markers/ui-prototype/verify.cjs
```

浏览器实际退出0，**214断言**，status=passed/browserClosed=true。开始2026-10-11T16:02:02.020Z，结束2026-10-11T16:02:41.903Z；逐条名称见[checks.json](checks.json)。会话keyevents-reader-prototype，finally关闭。16次截图有重复保存同名最终态，共14个唯一文件，不声称16个不同场景。未起dev服务。

## 本轮可证事实

- 期刊卡→全屏reader，封面/导语/highlights/目录/话题章/页边注；共享section5101下101/102/108三个不同thread，标题与父章不同、依据2/1/0。
- 每线索动作与button.drm-thread__header同级，非嵌套button；摘要t:thread.id锚保留。展开A只出A的2篇；点文章只本地预览，不确认；选A不改B/父原标题、状态、所有原计数/数据。A/B可以独立确认/取消，多篇依据只一个marker。
- 无文章108的原展开按钮disabled，不误禁合法mark且保留跑题警告；无归属109禁标且说明原因，不自动建题。
- 总览去重真实section（AI8个有效泳道来源章）；5101只有一个原点，A/B各有thread/marker注记，原中心statusFill/label/身份/坐标不改。突出也不移动源点，真实section和快照分开，不增加假章或边。
- suggested仍在缓存但总览/聚焦/详情没有徽标、理由、新mark/dismiss或待确认aria；普通新闻保留。详情准确导航原版块/日期/thread。总览只能取消已有confirmed；失效快照无假原文/日报锚，取消不变source。
- 日报保存中锁本人但不预先确认、不锁兄弟；重复点击单请求。读取保留正文而禁未知写。错误回原态可重试；跨日期/视图/版块的合法旧写仍存旧owner、不塞旧消息，旧读取不锁新owner。空期/全部/无关键窗口不造内容。
- 六视口1440×900/1920×1080/375×667/390×844/767×900/768×900，editorial/dark逐一比较main.css十二Token；reader长词/正文可达、窄屏新增动作44px，无页面横溢；图中点击矩形不互盖，marker和动作elementFromPoint实际命中、键盘/返回焦点验证，页边注抽屉中心命中本体。
- 全部资源本地，无HTTP/AI调用。左侧tag_count、源数据、标记状态是fiction，不证明真实服务器/并发事务正确。

## 当前截图

| 文件 | 场景 |
| --- | --- |
| [reader-cover-editorial-1440.png](reader-cover-editorial-1440.png) | 报纸封面/导语 |
| [reader-threads-editorial-1440.png](reader-threads-editorial-1440.png) | 本章多thread、独立动作/摘要（长内容压力态） |
| [reader-error-375.png](reader-error-375.png) | 保存失败，原建议和重试保留 |
| [reader-threads-dark-1440.png](reader-threads-dark-1440.png) | 本章多thread、独立动作/摘要（长内容压力态） |
| [reader-threads-editorial-1920.png](reader-threads-editorial-1920.png) | 本章多thread、独立动作/摘要（长内容压力态） |
| [reader-threads-dark-1920.png](reader-threads-dark-1920.png) | 本章多thread、独立动作/摘要（长内容压力态） |
| [reader-threads-editorial-375.png](reader-threads-editorial-375.png) | 本章多thread、独立动作/摘要（长内容压力态） |
| [reader-threads-dark-375.png](reader-threads-dark-375.png) | 本章多thread、独立动作/摘要（长内容压力态） |
| [board-thread-markers-editorial-1440.png](board-thread-markers-editorial-1440.png) | 真实section点与独立thread确认注记 |
| [board-thread-markers-dark-1440.png](board-thread-markers-dark-1440.png) | 真实section点与独立thread确认注记 |
| [board-thread-markers-editorial-1920.png](board-thread-markers-editorial-1920.png) | 真实section点与独立thread确认注记 |
| [board-thread-markers-editorial-375.png](board-thread-markers-editorial-375.png) | 真实section点与独立thread确认注记 |
| [board-thread-markers-dark-375.png](board-thread-markers-dark-375.png) | 真实section点与独立thread确认注记 |
| [reader-cover-editorial-1920.png](reader-cover-editorial-1920.png) | 报纸封面/导语 |

full截图对固定reader展示的是其当前内部滚动位置，不代表整份报纸一屏显示；尺寸是viewport。长内容压力态/双已标记是验证过程状态，不是AI默认确认。旧FB-1/2/3截图及reader-first-smoke-1440不是最终验收基线。

## K3视觉抽检基础设施阻断

用户增量约束授权K3截图复核。按原协议派只读reviewer、fresh、model=`commandcode/moonshotai/Kimi-K3`、async=true；**没有启动成功、没有读图/审查产物，不记PASS**。

- run：`d012c144-f7e3-4210-9205-5f6bf2f9f9cd`；start=failed；status查询=Async run not found。
- 原始错误：`Background children require a supported standalone Pi host or the installed npm package (@earendil-works/pi-coding-agent); neither is available.`
- repo/cwd：`/home/zanebono/software/Syntopica`；主仓库，无独立worktree；branch=`develop`；ref=`0a33ccb893a6be19a765191c1c9b2bca07d4aa4e`（失败时）。
- 部分工作先留：`/tmp/keyevents-fb4-partial-1791734840896.patch` + `.tar.gz`；详情`/tmp/keyevents-fb4-lane-blocker.json`。归档包含当时change文件/未跟踪截图，不是只依赖空diff。
- 已停止该lane；没有偷换Agent、CLI或前台，没有给自己批准豁免。用户在结构化询问中明确选择“本轮由我看图”，人工接管本轮截图复核；本轮不再等待K3，但后台host问题未修，失败记录保留。该选择不是UI批准或apply授权。output绑定的visual-review-fb4.md没有产物，不提供虚构outputReference。

## 部署/边界

正式代码、main.css、API/表/业务数据、部署均未变，无用户操作、旧数据不需降级。214条只证明原型，49产品场景/S1–S6仍计划；实际组件允许性/共享缓存、静态:5100 opencli完整故事、PG FK/revision/事务、真实材料量化效果仍待实施。用户“继续改”未批准新版major UI，approval仍pending，不apply/归档。
