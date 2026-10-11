# 原型验证 · FB-3 日报挑选、总览回看

本记录保存FB-3原型当时结果，取代其时FB-2总览候选展示。FB-4只读实查确认其日报形状过于简化：一thread=一section、扁平事件列表不代表现有全屏reader/一章多线索。以下148检查仍真实但只属旧简化模型，不能作为最新日报接入/批准基线；本轮未改原型代码或宣称新场景已测。

## 实际执行

```bash
node --check openspec/changes/add-topic-key-event-markers/ui-prototype/app.js
node --check openspec/changes/add-topic-key-event-markers/ui-prototype/verify.cjs
node openspec/changes/add-topic-key-event-markers/ui-prototype/verify.cjs
openspec validate add-topic-key-event-markers --strict
```

最终完整浏览器执行退出0，**148条原型断言通过**；明细/实际起止时间见[checks.json](checks.json)，status=passed、browserClosed=true。主题source/hash与现有main.css一致。会话keyevents-board-prototype，每次包括失败尝试都finally关闭，无dev服务或产品请求。语法/strict另算，不计入148。

## 核心入口分工

- 初始总览AI版块两行、9个真实来源fixture+1个confirmed失效快照。101/104仍是suggested存储态，但投影只是原新闻；没有候选虚线/徽标/理由/待确认aria或mark/dismiss按钮，也不删除相应section。
- focus/就地详情同样不显示未定建议；当前新增选择必须去日报。UI模式守卫模拟伪造总览mark按钮也不产生写（这是前端上下文验证，不冒充服务端鉴权）。
- 普通详情「去来源日报挑选」准确定位本版块、fixture来源日期、对应thread且标题获焦；本期只显示该期有效来源，不把历史/其他板块建议混在同日报。
- 这期日报显示建议、理由、确认/不采纳；无建议可手标，不采纳后可自己再选。确认后回总览只有人工高亮，原字段、section ID集合、状态色、动态计数/tag_count均保持。
- 刷新恢复人选，不把缓存suggested重新带入总览；取消有来源节点仍保留背景且重新挑选需日报。失效快照可在总览取消维护，无假section、原文或日报锚，取消不伪装普通source。
- 在途回总览：尚未成功仅普通新闻，合法成功同步confirmed但无旧提示；切日报日期：旧期合法写仍存旧源，不显示旧条目/旧提示到新期；切版块同样隔离。
- 历史日期可挑本期跟进；空演示日无来源或建议，不凑假新闻。原型native date select只是既有日报日期选择的示意，不新建日期字段/日历系统。

## 原有检查仍保留

- 版块AI只llm/agents，住房只housing/supply；全部是文章池说明；focus/返回不换owner，不造全局topic轴，旧读取generation不锁新版块。
- editorial/dark十二项Token：base/elevated/sunken/accent/主次文字/边框/五态原动态色与main.css比对；原型无第三套配色。
- 突出只改强调，普通点/文本仍可读可选，节点集合/时间坐标不变；同日命中区/标题无交叠，范围扩展只增标尺、不造source。
- 点击/Enter到匹配详情、标题获焦；返回节点恢复焦点并完整在固定标签右侧，elementFromPoint中心实际命中。选择不确认/展开文章/研究，Space可切强调。
- 日报saving互斥、不提前确认，重复提交只一条请求；读取中原文可读/禁未知写；保存失败仍原建议、可重试清错。已确认取消维护不删原新闻或旧报告。
- 1440×900、1920×1080、375×667、390×844、767×900、768×900，每个两主题长内容：无全页横溢、日期局部滚动、节点桌面36/窄屏44px命中；普通/确认标题完整文本可由title/详情读到。
- 375的日报两主题额外确认日期选择器/动作至少44px、选择器沿用主题底、只有当前板块当前期。所有资源本地，没有HTTP/HTTPS或AI调用。

## 复验中修正

首次到第24条后，恢复长页面滚动的位置使tab中心被sticky header挡住，真实click失败（无force点击），本轮失败浏览器也已关闭。验证器现在先把目标滚到可见区域再真实点击，模拟用户先滚动再点；不移除header或伪造点击成功。此处不声称改了正式产品壳。

新增日报动作/日期选择器补窄屏44px命中下限，并实际比对两主题背景。最终完整重新执行通过，不把失败尝试计为通过。

## 本轮十二张截图

都是最后一次验证产生的full页面，尺寸指视口，不声称完整页面一屏容纳。

| 文件 | 场景 |
| --- | --- |
| [daily-review-editorial-1440.png](daily-review-editorial-1440.png) | AI当期日报，建议/理由/挑选动作，只该期 |
| [daily-review-editorial-375.png](daily-review-editorial-375.png) | 住房当期日报，手机浅色 |
| [daily-review-dark-375.png](daily-review-dark-375.png) | 同住房日报，手机深色 |
| [daily-error-375.png](daily-error-375.png) | 仅日报挑选保存失败与恢复入口 |
| [board-ai-editorial-1440.png](board-ai-editorial-1440.png) | 确认后AI回看，普通背景/confirmed，不含AI候选 |
| [board-ai-dark-1440.png](board-ai-dark-1440.png) | 同AI回看，深色 |
| [board-housing-editorial-1440.png](board-housing-editorial-1440.png) | 住房独立两泳道，未处理建议仅普通来源 |
| [board-housing-dark-1440.png](board-housing-dark-1440.png) | 同住房深色，无未定推荐 |
| [board-ai-editorial-1920.png](board-ai-editorial-1920.png) | 大屏长内容总览 |
| [board-ai-editorial-375.png](board-ai-editorial-375.png) | 手机长内容总览浅色 |
| [board-ai-dark-375.png](board-ai-dark-375.png) | 同总览手机深色 |
| [board-ai-editorial-390.png](board-ai-editorial-390.png) | 390窄屏长内容 |

其他旧截图不属于本轮审批基线，尤其混合入口/绿色主题/总览AI候选/旧总览保存失败截图。

## 边界

本轮没有正式产品代码/接口/表/主题源/数据变更、没有构建部署、没有真实推荐或付费模型效果评估。字段/写入/来源身份断言是fiction fixture与原型投影，不替代PG事务、服务端confirmed过滤或真实Vue缓存验证。后续test-cases.md S1-S6、49个Scenario、产品静态:5100 opencli/PG/真实材料效果仍待实施。

仍ui-impact=major、ui-approval=pending。FB-3修订不是用户批准，不apply；产品/旧数据不变，无用户操作。
