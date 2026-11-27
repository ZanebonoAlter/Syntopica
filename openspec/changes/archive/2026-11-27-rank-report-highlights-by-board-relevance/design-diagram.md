# design-diagram — rank-report-highlights-by-board-relevance

> 本 change 的时序图。落点与取舍见 `design.md` Decisions；图只画稳定骨架（消息先后与回退分支），不画易变字段名。

这张图回答什么问题：**模型是在哪一步、从哪些候选里挑重点的？贴题门槛加在哪？正文板块为什么必须先落库？查不到贴题信号时走到哪？**

```mermaid
sequenceDiagram
    participant O as 日报编排 GenerateAndSaveReport
    participant DB as daily_report_sections
    participant F as 重点收尾 finalizeDailyReportHighlights
    participant R as 仓储 ListReportSectionSignals（新）
    participant P as 候选池 selectCandidatePool（新）
    participant L as LLM
    participant S as 后排 rankPickedHighlights（新）
    participant W as FinalizeReportHighlights

    O->>DB: SaveReport（板块连游泳道档与话题状态同事务落库）
    O->>F: 开始生成重点（report id）
    F->>F: 当天聚簇，按既有候选序排好（不截断）
    F->>R: 按 report id 读本期板块贴题信号
    alt 读取失败
        R--x F: 错误
        F->>F: 记 Warn，候选池退回前 8（现状）
    else 读取成功
        R-->>F: 各板块标签集合 + 泳道档 + 话题当期状态
        F->>P: 全部聚簇 + 板块信号
        Note over P: 每簇定贴题档 0–5（跨板块取最好）
        alt 贴题簇 ≥ 3
            P-->>F: 只含贴题簇（至多 8）
        else 贴题簇 1–2
            P-->>F: 贴题簇 + 按贴题档补位到 3
        else 贴题簇 0
            P-->>F: 既有候选序前 8（现状）
        end
    end
    F->>L: 候选池的事实（提示词不变）
    L-->>F: 选中的簇 + 文案（返回顺序不作数）
    F->>S: 选中的重点 + 板块信号
    S-->>F: 按贴题档稳定排序（补位的在后）
    F->>W: 写 highlights / title / summary（首条）
```

图不表达的约束：贴题档的取值顺序（近 → 手动关注 → 中-在跟 → 中-候选 → 远 → 无对应板块）、梯队内部只比文章数（section 数与泳道热度不参与）、模型选中的重点不因贴题档被移除、标签回退模式不走这条路——这些在 `specs/daily-report-system/spec.md`「今日重点聚簇候选」与 `design.md` D3 / D7 / D11。
