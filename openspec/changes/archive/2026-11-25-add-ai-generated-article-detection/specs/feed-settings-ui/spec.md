## ADDED Requirements

### Requirement: Feed 卡片展示 AI 生成检测 toggle

订阅源卡片与订阅源详情 SHALL 展示「AI 生成检测」开关，SHALL 与既有处理管线开关（AI 总结、刷新后自动总结、全文抓取、AI 打标签）同层呈现。开关默认关闭。

开关 SHALL 就地切换并即时保存，SHALL NOT 弹出确认弹窗。该开关 SHALL 独立于「AI 打标签」开关：开启 AI 生成检测 SHALL NOT 隐含开启打标，开启打标 SHALL NOT 隐含开启检测。

#### Scenario: 默认关闭

- **WHEN** 用户查看一个从未配置过 AI 生成检测的订阅源
- **THEN** 该订阅源的「AI 生成检测」开关 SHALL 呈关闭态

#### Scenario: 切换即时保存

- **WHEN** 用户切换某订阅源的「AI 生成检测」开关
- **THEN** 该状态 SHALL 即时保存到服务端，SHALL NOT 需要额外点击保存按钮

#### Scenario: 与打标开关互不联动

- **WHEN** 用户开启某订阅源的「AI 生成检测」而其「AI 打标签」处于关闭态
- **THEN** 「AI 打标签」开关 SHALL 保持关闭态，两个开关 SHALL 互不联动

#### Scenario: 切换失败有反馈

- **WHEN** 保存开关状态失败
- **THEN** 界面 SHALL 给出失败反馈，SHALL NOT 呈现为已保存态
