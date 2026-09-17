# notification-center Specification

## Purpose
把异步任务的终态结果从"发完即丢的 WS 广播"升级为"落库持久通知 + 铃铛面板"：页面没开着时发生的任务结果（白名单=日报生成终态）在用户下次打开页面时仍然可见、可查历史、可标已读。

## Requirements

### Requirement: 通知白名单——只日报生成终态
系统 SHALL 仅在日报生成链路的终态节点产生通知：① 当次任务全部完成时产生一条"完成"通知（含生成日期、版面数、保存条目数）；② 当次任务存在失败版面时，在终态产生一条"失败汇总"通知（含失败版面数）。系统 SHALL NOT 为单版面失败产生多条通知，SHALL NOT 为抓取（Firecrawl）、自动刷新、单篇打标等高频过程类事件产生通知，SHALL NOT 产生"任务开始"类通知。

#### Scenario: 定时日报完成产生完成通知
- **WHEN** 凌晨定时日报任务全部版面生成完成（daily_report_done）
- **THEN** 通知表新增一条 type=success 的通知，标题含生成日期，摘要含版面数与保存条目数

#### Scenario: 部分版面失败产生一条失败汇总
- **WHEN** 一次日报任务运行中 6 个版面有 1 个失败
- **THEN** 任务终态产生一条 type=error 的失败汇总通知（含成功版面数与失败版面数），而非逐版面通知
- **AND** 不再另发完成通知（失败汇总与完成通知互斥，每次任务运行至多一条终态通知）

#### Scenario: 全部失败同样只有一条
- **WHEN** 一次日报任务所有版面均失败
- **THEN** 只产生一条失败汇总通知

#### Scenario: 非白名单事件不产生通知
- **WHEN** 标签任务完成/失败、Firecrawl 批次完成、自动刷新完成事件发生
- **THEN** 通知表无新增行，无 notification WS 事件

### Requirement: 通知持久化与 WS 同步推送
每条通知 SHALL 写入 notifications 表并即时经 WS 广播统一 `notification` 事件（载荷含完整通知对象）；页面离线期间产生的通知 SHALL 在下次 API 查询时可见（落库即触达）。

#### Scenario: 在线用户实时收到
- **WHEN** 日报任务完成且用户页面开着
- **THEN** WS 客户端收到 `notification` 事件，铃铛角标 +1

#### Scenario: 离线事件下次打开可见
- **WHEN** 日报在页面未打开时生成完成，用户随后打开页面
- **THEN** 未读数查询返回该通知，面板列表可见

### Requirement: 通知数据结构
通知 SHALL 包含：type（success/error）、title（如"日报已生成 · 2026-09-17"）、summary（如"共 6 个版面，保存 42 条条目"）、link_type 与 link_id（可选，用于跳转）、is_read、created_at。

#### Scenario: 失败通知携带可跳转目标
- **WHEN** 日报失败汇总通知产生
- **THEN** link_type 指向日报相关页面，前端点击可跳转

### Requirement: 通知查询与已读 API
系统 SHALL 提供单用户（无鉴权）通知 API：分页列表（支持 unread 过滤）、未读数查询、单条标已读、全部标已读、清空全部。清空全部 SHALL 经前端确认弹窗后才可调用。

#### Scenario: 未读数查询
- **WHEN** GET 未读数接口被调用且存在 2 条未读
- **THEN** 返回 2

#### Scenario: 全部标已读
- **WHEN** 全部标已读接口被调用
- **THEN** 所有 is_read=false 行置 true，未读数归零

### Requirement: 通知条数上限淘汰
notifications 表 SHALL 有 500 行上限：写入新通知时若超出上限，SHALL 淘汰最旧的已读行直至满足上限（淘汰在写入路径同步完成，不依赖定时任务）。

#### Scenario: 写入超限淘汰最旧
- **WHEN** 表已达 500 行且新通知写入
- **THEN** 最旧的行被删除，总行数保持 ≤500

#### Scenario: 未读行不被优先淘汰
- **WHEN** 需要淘汰且最旧行为未读
- **THEN** 从最旧的已读行开始淘汰；仅当全部为未读时才从最旧未读淘汰

### Requirement: 打标队列排空汇总为候选（本期不实现）
打标队列排空汇总通知（触发条件：队列从非空变为空，内容含成功/失败计数）记录为候选需求，本期 SHALL NOT 实现。

#### Scenario: 队列排空不产生通知
- **WHEN** 标签队列从有任务变为全部 completed
- **THEN** 通知表无新增行
