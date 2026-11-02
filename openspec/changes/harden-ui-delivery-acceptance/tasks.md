## 1. 结果合同与同源解析

- [ ] 1.1 按 test-cases.md D01–D24 补临时fixture与分支断言，使用真实可读PNG验证尺寸，运行 `bash scripts/harness/ui-verification-trace.smoke.sh`；不得只按文件名制造阳性
- [ ] 1.2 给现有 trace 增加 `--delivery`，默认证据职责保留；落实v1/发布日期/none-minor-旧schema边界，D01/D02/D13–D17通过
- [ ] 1.3 在同一parser记录稳定A-ID、历史/最新/非法条目，解析唯一必验表，落实任意最新fail、核心缺失/未覆盖与复验新证据判定；D03–D12/D24通过
- [ ] 1.4 将最新条目自己的相对PNG引用与其合同视口匹配，校验真实尺寸和目录边界，禁止借其他场景/旧图/全局清单；D18–D23通过

## 2. 集成与规划入口

- [ ] 2.1 spec-gate既有UI项消费单次 `--delivery`，保持未就绪与环境fail-open/bypass区分，运行 `.pi/extensions/tests/spec-gate.smoke.cjs` 相关断言
- [ ] 2.2 archive-readiness既有UI项同步单次 `--delivery`，运行 `bash scripts/harness/archive-readiness.smoke.sh`，观察与硬门禁同源且不重复解析
- [ ] 2.3 在 syntopica-ui 的UI模板/指令补动作作用域、复用宿主及major v1表说明；none/minor不要求major表，人工核对模板与本change规格一致

## 3. 工坊试点

- [ ] 3.1 在不改变 pending 审批的前提下，为工坊补v1和固定必验表；保留八节、真实故事、现有fail/未覆盖，不把机器格式调整当新原型已批准
- [ ] 3.2 只读试点负路径：`bash scripts/harness/ui-verification-trace.sh openspec/changes/rework-workshop-into-radar-and-desk --delivery`，期望exit1，明确列出A-43失败、核心未覆盖/缺失；默认模式证据成功不能抵消
- [ ] 3.3 待工坊原型获批且产品真实修复后，按A-ID追加实页复验并只读重跑delivery/自查，期望结果收口；只有fixture阳性或静态原型不得勾此项，不提前归档

## 4. Review

- [ ] 4.1 派子线程聚焦 review（本change制品+diff+定向测试），核对parser复用、默认兼容、非法/复验/路径边界和伪放行；按High/Medium/Low处置并在review.md建账，再复跑影响断言

## 5. 测试

- [ ] 5.1 `bash scripts/harness/ui-verification-trace.smoke.sh`，期望默认回归与D01–D24全部通过，不执行真实archive/DB操作
- [ ] 5.2 `bash scripts/harness/archive-readiness.smoke.sh`，期望delivery透传与原有聚合行为通过
- [ ] 5.3 `node .pi/extensions/tests/spec-gate.smoke.cjs`，期望UI结果未就绪阻断、旧schema/none/minor/环境分支保持，D25/D26通过
- [ ] 5.4 按 test-cases.md 核对同fixture重复执行幂等、中文定位信息与已知机械边界；未声明浏览器截图内容可由脚本识别

## 6. 文档

<!-- doc-impact: standard -->

- [ ] 6.1 同步 `docs/reference/开发执行规范.md` §0.6/§5.3/§11：作用域/宿主/旧信息语义、修复复验、证据与结果职责及同源命令；无flow影响（纯开发工具链，不改业务流程）
- [ ] 6.2 同步 `docs/reference/standard/frontend/layout.md` 与 `standard/shared/test-design.md`（含JIT摘要）：打开态、逐场景视口、旧能力去向与复验留痕，不新增平行规范文件
- [ ] 6.3 同步 `docs/reference/standard/shared/doc-authoring.md` 注册点/门禁索引与 `harness/pi-extensions.md`：v1 marker、固定表、归档判定入口、旧合同降级和既有事件口径
- [ ] 6.4 同步模板/规格/文档引用，运行 `bash scripts/harness/doc-impact.sh verify openspec/changes/harden-ui-delivery-acceptance` 与 `bash scripts/harness/check-standards.sh --change harden-ui-delivery-acceptance`，期望无本change文档欠账

## 7. 验证

- [ ] 7.1 `openspec validate harden-ui-delivery-acceptance --strict`，期望delta规范有效；`bash scripts/harness/check-tasks-tail.sh openspec/changes/harden-ui-delivery-acceptance` 期望Review与尾三节符合
- [ ] 7.2 `bash scripts/harness/scenario-trace.sh openspec/changes/harden-ui-delivery-acceptance`，期望下表每个Scenario有落点
- [ ] 7.3 `bash scripts/harness/ui-verification-trace.sh openspec/changes/harden-ui-delivery-acceptance --delivery`，期望none按N/A通过，不受工坊pending污染
- [ ] 7.4 `bash scripts/harness/review-trace.sh openspec/changes/harden-ui-delivery-acceptance`，期望有效review留痕，High均已处置
- [ ] 7.5 `bash scripts/harness/archive-readiness.sh harden-ui-delivery-acceptance`，期望本change全部自查通过；不执行真实archive，不将其他change混合脏树视作本次完成
- [ ] 7.6 以工坊真实正/负两路径完成试点后更新验证记录和部署影响说明；期望不要求用户清库/重生成，未完成的产品故事与反馈仍保持未解决

| Scenario | 测试文件 |
| --- | --- |
| 默认模式仍只检查证据 | scripts/harness/ui-verification-trace.smoke.sh |
| 试点显式启用合同 | scripts/harness/ui-verification-trace.smoke.sh |
| 新 major 缺合同阻断 | scripts/harness/ui-verification-trace.smoke.sh |
| 存量未迁移明确降级 | scripts/harness/ui-verification-trace.smoke.sh |
| 非法合同不作存量放行 | scripts/harness/ui-verification-trace.smoke.sh |
| 非 major 与旧 schema 兼容 | scripts/harness/ui-verification-trace.smoke.sh |
| 必验故事从未验过 | scripts/harness/ui-verification-trace.smoke.sh |
| 合同空壳或行不合法 | scripts/harness/ui-verification-trace.smoke.sh |
| 任意最新失败阻断 | scripts/harness/ui-verification-trace.smoke.sh |
| 核心未覆盖阻断而非核心明确提醒 | scripts/harness/ui-verification-trace.smoke.sh |
| 追加有效复验后解除 | scripts/harness/ui-verification-trace.smoke.sh |
| 无复验或旧失败截图不能解除 | scripts/harness/ui-verification-trace.smoke.sh |
| 非法条目不得被忽略洗绿 | scripts/harness/ui-verification-trace.smoke.sh |
| 逐场景视口齐全通过 | scripts/harness/ui-verification-trace.smoke.sh |
| 别的页面宽屏图不能借用 | scripts/harness/ui-verification-trace.smoke.sh |
| 真实尺寸而非文件名判定 | scripts/harness/ui-verification-trace.smoke.sh |
| 截图路径不得越界 | scripts/harness/ui-verification-trace.smoke.sh |
| 自查与硬门禁同一结果 | .pi/extensions/tests/spec-gate.smoke.cjs |
| 标准条目解析 | scripts/harness/ui-verification-trace.smoke.sh |
| 判定词表外不合法 | scripts/harness/ui-verification-trace.smoke.sh |
| 格式对齐实战形态 | scripts/harness/ui-verification-trace.smoke.sh |
| 复验沿用场景锚点保留失败 | scripts/harness/ui-verification-trace.smoke.sh |
| 门禁与脚本同源判定 | .pi/extensions/tests/spec-gate.smoke.cjs |
| 扩展内旧字样检查退役 | .pi/extensions/tests/spec-gate.smoke.cjs |
| 环境异常与明确未就绪分开 | .pi/extensions/tests/spec-gate.smoke.cjs |
| 全局动作与重复入口有职责 | 人工：复核模板/reference与试点已批准合同中的作用域及入口理由 |
| 内联组件复用到新工作面 | 人工：复核模板宿主契约并在工坊计算故事实页复验 |
| 视图替换保留信息语义 | 人工：复核旧能力去向及工坊两类关系图真实故事 |
| major UI 双层验收通过 | 人工：实际产品复验与交付结果对账，不以静态原型代替 |
| 只有单元测试不得完成 major UI | scripts/harness/ui-verification-trace.smoke.sh |
| 字样齐备但无实物不得通过 | scripts/harness/ui-verification-trace.smoke.sh |
| minor 指名映射通过 | scripts/harness/ui-verification-trace.smoke.sh |
| minor 裸关键词不再放行 | scripts/harness/ui-verification-trace.smoke.sh |
| 有截图但故事失败仍不得完成 | scripts/harness/ui-verification-trace.smoke.sh |
