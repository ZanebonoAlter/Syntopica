## Purpose

前端 vitest 运行环境的确定性约束：单测进程的模式开关不继承宿主进程，全局 mock 状态在测试用例之间隔离——保证任何入口（pi 会话 / 巡检脚本 / 手动终端）跑出的单测结果一致可信。

## ADDED Requirements

### Requirement: 单测进程 NODE_ENV 确定性

前端单测运行环境 SHALL 在 vitest 配置层钉死 `NODE_ENV=test`，覆盖宿主进程传入的任何 `NODE_ENV` 值（含 pi 会话自带的 `production`）。单测进程 SHALL 始终以非生产模式的 Vue 运行时加载，保证依赖开发模式事件追踪的组件测试断言（如 `emitted()`）在任何启动入口下行为一致。该钉死 SHALL 仅作用于 `pnpm test:unit` 进程内，不得影响 `pnpm build`（构建仍为 production）与 `pnpm dev`（仍为 development）。

#### Scenario: 宿主进程携带 production 时单测仍可断言组件事件

- **WHEN** 在 `NODE_ENV=production` 的 shell（如 pi 会话的 bash 工具）中执行 `pnpm test:unit <组件测试文件>`
- **THEN** 组件事件断言（`wrapper.emitted(...)`）正常工作，结果与干净环境执行完全一致

#### Scenario: 构建与 dev server 不受影响

- **WHEN** 分别执行 `pnpm build` 与 `pnpm dev`
- **THEN** 构建产物仍按 production 模式生成、dev server 仍按 development 模式运行，行为与本 change 前一致

### Requirement: useState mock 状态用例间隔离

前端测试 setup 中 Nuxt `useState` 的 mock（按 key 单例 registry，支撑跨实例共享 composable 的可测性）SHALL 在每个测试用例开始前将 registry 中各状态重置为初始值（按各自 init 语义），使每个用例拿到全新的初始状态且 registry 条目的引用保持稳定（模块顶层解构的 composable 引用跨用例仍指向同一状态源）；同一用例内经同一 key 的跨实例读写共享语义 SHALL 保持不变。

#### Scenario: 前一用例的状态不泄漏给后续用例

- **WHEN** 同一测试文件内，用例 A 经 `useState(key)` 写入状态后，用例 B mount 使用同一 key 的组件并断言初始值
- **THEN** 用例 B 读到的是该 key 初始化的全新值，而非用例 A 的残留值

#### Scenario: 同一用例内跨实例共享不回退

- **WHEN** 同一用例内，测试代码与被测组件经同一 `useState(key)` 读写
- **THEN** 双方互相可见（per-key 单例语义保持），useConfirm 类跨实例共享 composable 的既有测试保持全绿
