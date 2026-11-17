<!-- ui-impact: major -->
<!-- ui-approval: pending -->
<!-- ui-prototype: ui-prototype/index.html -->

<!--
  marker 契约（ui-design-gate 机器读取，缺失/非法/重复会被阻断）：
  - ui-impact 必须与 proposal 声明一致
  - none  → approval=not-required、prototype=none，正文只保留「## N/A Reason」
  - minor → approval=not-required、prototype 可为 none，保留四节轻量契约
    （入口与变更 / 受影响状态 / 复用组件与布局模式 / 验收映射）
  - major → 完整八节 + 三节结构化声明（`## Interaction Decisions` /
    `## Data Provenance` / `## 明确不做`）+ 可丢弃原型；canvas 主模式用
    `states/` 三态图 + `interaction-table.md`，其余模式用可点 HTML；
    approval 由用户明确确认后才可从 pending 改为 approved；
    信息架构/主流程/布局模式修订后重置 pending
-->

## User Journey

<!-- 页面入口 + 用户主/次任务（major 必填） -->

## Information Architecture

<!-- 信息层级、导航结构、区块划分（major 必填） -->

## Interaction Contract

<!-- 主/次/危险操作、交互流程、反馈方式（major 必填） -->

## Interaction Decisions

<!-- major/minor 必填；none 可省略。一行一个决策：
     | ID | 决策 | 模式 | forbidden | 状态 |
     模式取值限定 7 词表：page / panel / drawer / dialog / inline-edit / canvas / command
     （完整判据见 docs/reference/standard/frontend/interaction.md）
     pinned = 用户已定，实现与原型不得替换/简化/合并/改写交互形态/放宽 forbidden；
     唯一合法变更路径 = 用户明确改口后由 agent 写回并注明「变更来源：用户 <日期>」 -->

## Data Provenance

<!-- major/minor 必填；none 可省略。一行一个用户可见数据项：
     | 数据项 | source | 说明 |
     source 取 api <path>（须能在 front/app/api/** 或后端路由 grep 到）/ 表.字段 /
     派生 <公式> / none（须标 needs-approval 并在对话中取得用户确认） -->

## 明确不做

<!-- major 必填。每条须能读成「不出现在本界面里的具体东西」；
     禁「暂不支持」这类无对象措辞 -->

## State Matrix

<!-- loading / empty / error / success 完整状态矩阵，含错误恢复（major 必填；minor 只列受影响状态） -->

## Layout Contract

<!-- layout mode：reader=760 / contained=1120 / workspace / split（各栏最小宽度与溢出策略）
     + 目标视口（桌面 1440×900、1920×1080；支持窄屏另列）
     + dialog 尺寸档 sm=420 / md=560 / lg=760 / xl=1040（92vw 上限）
     自由宽度必须写理由与目标视口 -->

## Component Reuse

<!-- 复用映射：AppPageShell / AppDialog(size) / AppButton / AppInput …；需要新组件时列明 -->

## Prototype

<!-- major：按主模式分流——
     ① 主模式为 canvas：states/ 三态图（空态 / 编辑中 / 已确认）+ interaction-table.md
        （操作/触发/前置/结果/反馈/失败），marker 指向 states/ 首张图，不用可点 HTML；
     ② 其余模式：ui-prototype/ 可点 HTML 原型说明（打开方式、fixture 数据、与真实接口的解耦说明）；
     minor/none：写「无需原型」 -->

## Acceptance

<!-- major：agent-browser 主链路断言 + 1440×900 / 1920×1080 两档视觉检查 + 与批准原型的差异说明，
     证据以 ui-verification/report.md 建账（### A-N 场景【pass|fail|未覆盖】+ 依据行，
     未覆盖写原因；格式契约见 ui-verification-trace-gate）；
     minor：验收映射指名道姓——组件测试写真实存在的测试文件路径，
     或 agent-browser/人工验证在 ui-verification/report.md 建账；none：N/A -->
