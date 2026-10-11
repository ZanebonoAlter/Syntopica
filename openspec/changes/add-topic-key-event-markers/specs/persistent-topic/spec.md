## MODIFIED Requirements

### Requirement: 话题生命线 API

`GET /api/daily-reports/topics/:id/lifeline` SHALL 返回该 PersistentTopic 下的全部 section（不限天数，按 `persistent_topic_id` 聚合）及内部关系。

响应 SHALL 包含 topic 元信息（label/status/first_seen_date/last_seen_date/hit_count）和 sections 列表（含 period_date/cluster_label/status/article_count/thread_count）。relations SHALL 只包含两端 section 都属于该 topic 的关系。

相比 `getSectionLifecycle`，本 API SHALL NOT 依赖 embedding 连通性，直接按 persistent_topic_id 聚合。

响应 SHALL 向后兼容地提供可选已确认关键节点列表，包含确认快照、来源日报日期、依据和来源可用状态。节点 SHALL 不占用真实 section ID、不改变 identity/similarity 关系；无真实 section 但有已确认快照时 SHALL 仍可读取节点。无节点时返回空集合，不自动启用/归档话题。

#### Scenario: 查询话题完整生命线
- **WHEN** topic #12 下有 4 天的 section（#40/#50/#60/#70）
- **THEN** 响应 SHALL 返回这 4 个 section 及其内部 identity/similarity 关系

#### Scenario: 话题跨命名漂移聚合
- **GIVEN** topic #12 下 section 标签依次为 "AI 编程竞争"/"开发者生态重构"/"AI 工具内卷"
- **WHEN** 查询 lifeline
- **THEN** 响应 SHALL 返回全部 3 个 section（即使 embedding 两两 distance 部分超过 penalty）

#### Scenario: 无 section 仍能查确认快照
- **WHEN** 原 section 被替换且该泳道保有确认快照
- **THEN** 生命周期响应返回快照，不伪造 section 或关系，不自动改变话题状态

## ADDED Requirements

### Requirement: 当前泳道关键节点高亮

既有泳道 SHALL 在当前选中版块范围内，以来源日报日期从左到右定位节点，普通动态作背景、人工确认节点醒目高亮；未确认AI建议只在日报展示，总览不得出现建议徽标、理由或挑选动作，建议所对应的真实section仍是普通新闻背景。总览可展示本版块多条泳道，进入既有focus后 SHALL 只展示选中泳道，不能将所有版块的话题混到同一时间轴。系统 SHALL 提供突出关键节点模式，弱化普通背景但不删除普通节点、移动时间位置或把内容改成关键事件列表。同日节点顺序 SHALL 稳定。日期范围复用现有选项；当前窗口无确认节点 SHALL 提示并允许关闭强调，背景仍可查看。高亮 SHALL 不更改真实动态计数、图关系或人工选择；切换泳道 SHALL 重置局部强调，迟到响应 SHALL 不串入新泳道。

#### Scenario: 高亮保留背景但不混入他泳道
- **WHEN** 已聚焦的当前泳道窗口有确认节点、未确认建议以及另一泳道节点
- **THEN** 高亮只来自本泳道人工确认，普通及建议所对应的来源新闻仍为可达背景，但不展示建议状态/理由/挑选入口，他泳道不混入，时间位置和真实动态计数不变

#### Scenario: 取消最后节点仍保留时间背景
- **WHEN** 在突出模式中成功取消最后一个节点
- **THEN** 提示当前窗口无确认节点，原新闻与日期仍可查看并能关闭强调；失效快照不伪装成真实动态

#### Scenario: 切换泳道不串状态
- **WHEN** 前一泳道请求未返回而用户切到新泳道
- **THEN** 新泳道强调重置，迟到响应不覆盖新泳道状态

### Requirement: 泳道节点可读性与就地详情

泳道 SHALL 保持可读日期与节点标题，不将长跨度压成无法辨识的小点。同日多个条目 SHALL 分别可选并避开标题/命中区遮挡；常规标题换行，超长标题以省略号呈现且完整文本在可访问名称、title或就地详情可达。点击/键盘选择 SHALL 展示对应条目原位详情而非直接确认；横向滚动 SHALL 限于轴容器，不造成页面级横向溢出，窄屏入口 SHALL 可触摸。

#### Scenario: 点击节点读详情但不标记
- **WHEN** 用户点击或键盘选择未确认的节点
- **THEN** 原位详情显示匹配的完整标题、依据和人工动作，状态不变、不自动研究、不新开弹窗

#### Scenario: 同日标题错开且长文本可读
- **WHEN** 同日有多个事件，含一条超长标题
- **THEN** 每条入口与标题分别可达且不遮挡；超长内容可从就地详情读取，不能只硬裁标签

#### Scenario: 窄屏长轴局部滚动
- **WHEN** 375px宽视口查看长时间窗口并选择节点
- **THEN** 轴局部横向滚动、页面不横向溢出，详情单栏，节点可触摸并可键盘选择

### Requirement: 版块内泳道边界与既有主题

优化 SHALL 复用叙事工坊的版块选择、话题总览的lanes/focus入口；总览的泳道、section和关键注记 SHALL 限于selectedBoardId。当前版块身份 SHALL 在上下文与详情可辨；切换版块 SHALL 清理局部选择/强调，前版块迟到结果 SHALL NOT 覆盖新版块。全部入口 SHALL 保持文章池语义，不能新增跨版块全局泳道。

editorial/dark SHALL 复用main.css既有语义Token，不另定义绿色品牌配色。关键注记 SHALL 使用独立外环/星标，不覆盖原statusFill的emerging/continuing/split/merge/ending颜色或回写状态。普通节点标题 SHALL 继续来自section.cluster_label；关键文字来自thread/确认快照并作为独立注记，不能回写原section标题。真实section ID、关系和动态计数 SHALL 保留；失效快照 SHALL 不伪造section。版块侧栏标签数 SHALL 继续来自tag_count，不能被关键数量替代。

#### Scenario: 总览只展示选中版块泳道
- **WHEN** 用户选择AI版块，系统另有住房版块
- **THEN** 总览只含AI版块的泳道/动态/注记，详情标明该版块与泳道；点泳道进入原focus，返回仍是AI版块

#### Scenario: 切换版块迟到结果不串入
- **WHEN** 用户在AI版块的请求未返回时切至住房版块
- **THEN** 新版块清理局部选择/强调，迟到结果不改变住房内容或提示，AI版块合法人工选择仍按原owner保存

#### Scenario: 双主题高亮不覆盖原状态语义
- **WHEN** 用户切换editorial/dark并确认或取消关键注记
- **THEN** 配色沿既有主题Token，原section状态/状态色/标题/身份与真实动态计数不变，侧栏tag_count不变，只有人工注记层更新

#### Scenario: 全部入口不汇总所有泳道
- **WHEN** 用户选择侧栏全部而未选定版块
- **THEN** 保持文章池语义，不展示全局泳道或让跨版块注记进入同一时间轴
