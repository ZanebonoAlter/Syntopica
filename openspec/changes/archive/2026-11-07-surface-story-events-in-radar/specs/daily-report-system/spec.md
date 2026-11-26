## MODIFIED Requirements

### Requirement: 重点依据与正文定位
前端 SHALL 在故事重点旁显示服务端快照「N 个来源」（0 是合法值），可读提示附 48h 热度、截止时刻及不完整标记。字段缺失 SHALL 省略数字而非伪造 0。带有效 story_id 且有正文目标的重点 SHALL 可键盘激活，定位本 report 当前正文 DOM 顺序中该线第一条 thread；必要时展开目标所在正文容器，滚动、聚焦并高亮 2s。锚点 SHALL 为 `report-thread-{thread.id}`，归并只读解析存活 id。

旧重点无 story_id SHALL 仅展示文案、不猜标签归属。无目标 SHALL 禁用定位并提示「本期正文暂无对应事件」；点击时目标消失 SHALL 原地轻提示，SHALL NOT 打开新详情页、近似跳 section 或自动重建。重复激活 SHALL 替换单个高亮与计时器，切 report/卸载清理；reduced-motion SHALL 不平滑滚动，划选 SHALL NOT 触发定位。

跨页寻址 SHALL 通过日报阅读台路由的 `story` 查询参数（`/tags/:boardId/:date?story=<story_id>`）：参数 SHALL 在该期日报详情就绪后，按存活身份解析并定位本 report 当前正文 DOM 顺序中该事件线第一条 thread，展开/滚动/聚焦/高亮 2s，与刊头重点定位同一路径。参数所指事件线在本期正文无落点时 SHALL NOT 滚动、SHALL NOT 跳到近似位置，SHALL 提示「本期正文暂无对应事件」。翻页或切换 report 后 SHALL 清理该参数，SHALL NOT 在其它报告重复定位；无效或非法的参数值 SHALL 忽略且 SHALL NOT 报错阻断阅读。

#### Scenario: 点击第一条可见正文线程
- **WHEN** 同一 story 在本期两个正文位置出现，用户点击或键盘激活重点
- **THEN** SHALL 展开需要展开的正文容器并滚动/聚焦 DOM 中第一条对应 thread，高亮 2s，SHALL NOT 按最小 id 或跳别的 report

#### Scenario: 失去目标与旧重点
- **WHEN** 重点无 story_id 或有 id 但正文已无对应 thread
- **THEN** 无 id 的重点 SHALL 保持静态文案；无目标的故事重点 SHALL 提示本期正文暂无对应事件，SHALL NOT 产生空跳转

#### Scenario: 重复定位与减弱动画
- **WHEN** 连续定位两条重点，随后切换 report，且用户设置 reduced-motion
- **THEN** SHALL 始终最多一个高亮，切换时清理，无平滑动画，划选文案 SHALL NOT 触发定位

#### Scenario: 展示筛选不改变热度
- **WHEN** 某线程被既有 7 天/0.6 去重规则降为 flash
- **THEN** 其 story SHALL 在无其他 body thread 时不进入可定位重点候选，但身份、signals、H/P SHALL 不变；昨日重点徽标 SHALL NOT 把该线程恢复 body

#### Scenario: 深链参数定位与失去目标
- **WHEN** 用户从雷达打开 `/tags/1974/2026-11-06?story=42`，该期正文含事件线 42 的 thread
- **THEN** 阅读台 SHALL 定位该事件线在正文 DOM 顺序中的第一条 thread 并高亮 2s
- **AND** 当该期正文已无事件线 42 的 thread 时 SHALL 提示「本期正文暂无对应事件」且不滚动，SHALL NOT 跳到近似位置
