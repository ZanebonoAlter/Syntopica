## MODIFIED Requirements

### Requirement: 日报存储
系统 SHALL 提供 `SaveReport`（创建或更新日报+关联 sections）、`GetReport(boardID, date)`（查询单篇）、`ListReports(boardID, days)`（查询列表）三个存储接口。

`ListReports` SHALL 在 `days <= 0` 时使用 7 天默认值。对于任意正数 `days`，系统 SHALL 按请求天数查询，不得静默截断到 30 天。

同 board 同日重跑 SHALL 继续整份覆盖 sections/threads，但 SHALL 保留独立保存的人工关键标记和已处理推荐记录。来源替换 SHALL 标明失效并保留确认快照，SHALL NOT 通过相似度把旧确认转移到其他新条目。

#### Scenario: 保存日报和分组
- **WHEN** 流水线完成生成
- **THEN** 系统 SHALL 创建一条 BoardDailyReport 记录（status="done"）和多条 DailyReportSection 记录（每个聚类一条），并在事务中完成

#### Scenario: 查询最近 7 天日报
- **WHEN** 请求 `ListReports(boardID=5, days=7)`
- **THEN** 系统 SHALL 返回 board #5 最近 7 天的日报列表，按 period_date 倒序

#### Scenario: 查询超过 30 天日报
- **WHEN** 请求 `ListReports(boardID=5, days=42)`
- **THEN** 系统 SHALL 查询最近 42 天，不得将范围截断为 30 天

#### Scenario: 非正数天数使用默认值
- **WHEN** 请求 `ListReports(boardID=5, days=0)`
- **THEN** 系统 SHALL 查询最近 7 天

#### Scenario: 重生成保留选择而不移植确认
- **WHEN** 同日重生成替换旧 sections/threads，原条目有已确认或已处理建议
- **THEN** 选择记录仍在；非同一来源的新条目不自动继承确认；已处理的同一来源建议不重新推送

## ADDED Requirements

### Requirement: 日报推荐与历史兼容

日报读取 SHALL 以可选附加信息区分未处理建议与已确认标记，不重命名原 thread 字段或改变原文章展开行为。新日报推荐 SHALL 不阻塞日报保存及终态；历史缺字段 SHALL 降为无建议，但仍可从标记读取结果恢复已确认状态。读取历史 SHALL NOT 触发批量 AI 回刷。

#### Scenario: 旧日报可直接手动标记
- **WHEN** 打开没有推荐字段的旧日报且条目仍具有效泳道归属
- **THEN** 正常阅读并可手动标记，不调用 AI 回刷；已有持久标记仍可显示

#### Scenario: 推荐失败不改变日报终态
- **WHEN** 日报已保存而推荐失败
- **THEN** 报告仍为成功，不发第二条日报终态通知，不删除人工节点
