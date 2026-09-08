<!-- ui-impact: major -->
<!-- ui-approval: pending -->
<!-- ui-prototype: ui-prototype/index.html -->

<!--
  marker 契约（ui-design-gate 机器读取，缺失/非法/重复会被阻断）：
  - ui-impact 必须与 proposal 声明一致
  - none  → approval=not-required、prototype=none，正文只保留「## N/A Reason」
  - minor → approval=not-required、prototype 可为 none，保留四节轻量契约
    （入口与变更 / 受影响状态 / 复用组件与布局模式 / 验收映射）
  - major → 完整八节 + ui-prototype/ 静态原型，approval 由用户明确确认后
    才可从 pending 改为 approved；信息架构/主流程/布局模式修订后重置 pending
-->

## User Journey

<!-- 页面入口 + 用户主/次任务（major 必填） -->

## Information Architecture

<!-- 信息层级、导航结构、区块划分（major 必填） -->

## Interaction Contract

<!-- 主/次/危险操作、交互流程、反馈方式（major 必填） -->

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

<!-- major：ui-prototype/ 静态原型说明（打开方式、fixture 数据、与真实接口的解耦说明）；
     minor/none：写「无需原型」 -->

## Acceptance

<!-- major：opencli 主链路断言 + 1440×900 / 1920×1080 两档视觉检查 + 与批准原型的差异说明；
     minor：组件测试 / opencli / 人工验证映射；none：N/A -->
