## ADDED Requirements

### Requirement: 客户端虚拟系统状态条

通知面板 SHALL 支持非落库的置顶系统状态条（客户端虚拟条目）：由前端运行时状态驱动显隐，SHALL NOT 写入通知表、SHALL NOT 产生通知 WS 事件、SHALL NOT 计入未读数角标、SHALL NOT 参与条数上限淘汰与已读生命周期。首个接入的系统状态为 AI 模型健康未就绪（复用 analysis-pause-control 的可见性条件与文案契约）。置顶条 SHALL 展示于面板列表容器之上，不进入分页加载序列。

#### Scenario: 虚拟条目不产生落库通知

- **WHEN** AI 健康态在未就绪/就绪间多次翻转
- **THEN** 通知表 SHALL 无新增行，无 notification WS 事件，未读数不变

#### Scenario: 虚拟条目不受清空/已读操作影响

- **GIVEN** 面板正展示 AI 未就绪置顶条
- **WHEN** 用户执行「清空全部」或「全部已读」
- **THEN** 落库通知按既有契约处理，置顶条显隐 SHALL 仍仅由健康状态与暂停意图驱动，SHALL NOT 因清空/已读操作被移除或标记
