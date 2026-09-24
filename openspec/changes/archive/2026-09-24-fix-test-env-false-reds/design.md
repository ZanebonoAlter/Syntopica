## Context

三处修复的实现载体各一处文件：`front/vitest.config.ts`（NODE_ENV）、`front/vitest.setup.ts`（useState registry）、`scripts/harness/test-patrol.sh`（后端分片超时）。前两处是前端测试底座（vue 3.5.26 / @vue/test-utils 2.4.6 / vitest 3.2.4 / happy-dom 20.8.4，版本经排查未漂移）；第三处是巡检脚本既有分片静态枚举。事实链与复现步骤见 `docs/research/test-env-pitfalls/explore-findings.md`。

## Goals / Non-Goals

**Goals:**
- 任何入口跑 `pnpm test:unit` 结果一致（与宿主 `NODE_ENV` 解耦）
- useState mock 恢复用例间隔离，同时不回退 bd4f8847 的 per-key 单例语义
- be-skeleton 类锁竞争场景下巡检分片有界失败（≤120s）

**Non-Goals:**
- 不改 pi 会话自身的 `NODE_ENV`（那是 pi 运行时行为，且 bash 工具继承父环境是通用机制）
- 不给超时 panic 输出做逐测试归账解析（panic 无 FAIL 行，走既有「环境错误」路径已可接受；归账语义不变）
- 不动 `pnpm build` / `pnpm dev` 的模式，不动 quality-gate 的命令面（其前端命令仅 lint/typecheck/build，production 对 build 是正确的）

## Decisions

1. **NODE_ENV 钉死位置选 vitest 配置层（config 顶层强制赋值），不改巡检脚本、不靠文档提醒。**
   vitest 配置层钉死、作用域天然限定在单测进程内；改脚本只覆盖巡检一个入口（手动从 pi 会话跑测试仍中招），文档提醒无强制力。**实现机制修正（2026-09-24 实测）**：最初按 `test.env: { NODE_ENV: 'test' }` 实现，但 vitest 对 test.env 的应用是 `process.env[name] ??= value`（只填缺失变量，不覆盖已存在变量，源码 cli-api chunk 可证），宿主已带 `NODE_ENV=production` 时无效（AppDialog 仍 3 failed）；改为 vitest.config.ts 顶层 `process.env.NODE_ENV = 'test'`（主进程加载 config 时执行，早于 vite define 解析与 worker 启动），production 注入下 13 passed。备选「test-patrol.sh 前端 runner 显式 `NODE_ENV=test`」仍被否：入口覆盖不全。
2. **registry 用例间隔离选 `beforeEach` 重置值、保留引用，不加细粒度 reset API、不做 key 白名单。**
   原方案「`beforeEach` 全量 clear registry」存在盲区（2026-09-24 实测暴露）：模块顶层解构的 composable 引用（如 AppConfirmDialog.test.ts 的 `const { confirm } = useConfirm()`）跨用例持有 registry 里的 ref，clear 后组件内新建 ref 与之分支，跨实例共享断裂（7 条用例 6 条假红）。修正为遍历 registry 把每个 ref 的值回到 init 语义初始态（`entry.r.value = entry.init?.() ?? undefined`）：前一用例的残留值不再泄漏（原 2 条假失败消除），引用恒定（跨用例共享引用与同用例跨实例共享均保持）。`beforeEach` 对首个用例同样成立（防御性重置）；细粒度 API 仍是无消费者的复杂度。清理粒度是用例间状态值，不是实例间引用。
3. **后端分片超时上界取 120s（go test `-timeout 120s` 显式参数）。**
   be-skeleton 正常耗时 12-15s，120s 约 8-10 倍余量，可吸收冷编译缓存 + 高负载抖动；60s 在树莓派冷缓存场景有误杀风险。分片命令由脚本静态枚举集中定义，一处改动覆盖全部 6 个后端分片。

## Risks / Trade-offs

- `test.env` 钉死后，若未来有测试**有意**验证 production 模式行为，需在单测内自行 `vi.stubEnv` 覆盖（vitest 提供），spec Scenario 已锚定默认 test 语义。
- `beforeEach` 清空 registry 使「用例间传递状态」型测试写法失效——当前无此类用例，属预期收紧。
- 120s 超时若遇更慢环境（首次全量编译）可能误报环境错误；接受，因复跑成本低且有台账留痕可辨。
