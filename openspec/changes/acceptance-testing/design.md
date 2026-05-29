## Context

项目有三套测试：Go 单元测试 (backend-go)、Vitest (front)、Python UAT/集成 (tests/)。Playwright 已在 `front/tests/e2e/` 使用（TypeScript），验证前端组件渲染。Python UAT (`tests/uat/`) 验证后端 API + WebSocket 契约。但缺少"用户故事闭环"级别的验收测试——打开浏览器，按真实用户操作路径走完 happy path。

`unify-tag-hierarchy` 变更已完成，涉及 Sector 管理、层级树、模板重建、PendingChange 审批等核心功能，全部无 E2E 验收覆盖。

约束：
- 单用户系统，无认证
- Go 后端 localhost:5000，Nuxt 前端 localhost:3000
- 本地 LLM 服务（调用慢但可用）
- /tags 页面组件无 data-testid，只能用文本 + CSS 选择器
- Windows 开发环境

## Goals / Non-Goals

**Goals:**

- 建立通用的变更验收测试流程，每个变更一套验收测试，归档后保留作回归
- API 级别测试验证后端 happy path 闭环（不依赖浏览器）
- UI 级别测试验证浏览器内用户操作闭环
- 真实环境：真实后端 + 真实前端 + 本地 LLM，不 mock
- API 测试数据真实入库，UI 测试可看到 API 测试创建的数据
- 首个验收套件覆盖 `unify-tag-hierarchy` 的 10 个用户故事

**Non-Goals:**

- 不做 CI/CD 集成（手动本地运行）
- 不做 mock/stub（真实环境验收）
- 不做异常路径测试（只覆盖 happy path）
- 不改造现有 front/tests/e2e/ 或 tests/uat/（各自定位不同）
- 不在此次变更中给 /tags 页面加 data-testid（后续按需加）

## Decisions

### D1: Python + uv + pytest + Playwright

**决策**: 验收测试使用 Python 生态（而非 TypeScript），通过 uv 管理依赖。

**理由**: 项目已有两套 Python 测试（tests/uat/、tests/workflow/），Python 是验收测试的自然选择。pytest 的 fixture 机制天然适合测试准备/清理。uv 比 pip 更快更可靠。

**替代方案**: TypeScript Playwright（与 front/tests/e2e/ 统一）。被否决因为验收测试不只测前端——它验证后端 API + 前端 UI 的完整链路，Python 更适合编写全栈验收。

### D2: 按变更名称组织，API 和 UI 分层

**决策**: 目录结构为 `tests/acceptance/changes/<change-name>/`，文件命名 `test_story_00_api_*`（API 层）和 `test_story_01~06_*`（UI 层）。`00` 前缀的 API 测试先跑，验证后端可用后再启动浏览器。

**理由**: API 测试纯 HTTP 调用，秒级完成，失败能立即定位后端问题。UI 测试依赖 API 正常工作，API 先通过减少 UI 测试的不稳定性。

### D3: 真实数据，测试挂了手动清理

**决策**: 测试通过真实 API 创建数据（Sector、配置变更等），不做测试后自动清理。如果测试中途挂掉，用户手动清理数据库。

**理由**: 自动清理增加复杂度且不可靠（测试崩溃时 cleanup 不执行）。单用户系统、手动运行、本地数据库——手动清理成本极低，不值得为它增加框架复杂度。

### D4: 选择器集中管理，后续按需加 data-testid

**决策**: 所有 CSS/文本选择器集中在 `helpers/selectors.py`，不在测试文件中硬编码。当前不加 data-testid。

**理由**: /tags 页面组件刚写完不会频繁重构，文本选择器够用。集中管理确保 CSS 变更时只改一个文件。后续加 data-testid 时也只需改 selectors.py。

### D5: 长操作（rebuild）用轮询 + 合理超时

**决策**: 重建等涉及 LLM 调用的操作，测试中用轮询等待完成，设 10 分钟超时。超时后 skip 而非 fail。

**理由**: 本地 LLM 速度不确定，硬编码超时不可靠。skip 让用户知道"需要更多时间"，可以后续重跑验证。fail 会误报——后端可能正常只是慢。

### D6: 每个 story 文件自包含

**决策**: 每个 `test_story_*.py` 文件有独立的 `beforeEach` 导航到 /tags，不依赖其他测试文件的执行结果。但 API 测试创建的数据（如 Sector）UI 测试可以"看到"。

**理由**: 自包含让每个文件可独立运行和调试。数据可见性来自真实入库——这是真实环境验收的核心价值。

## Risks / Trade-offs

- **[风险] CSS 选择器不稳定** → 缓解：选择器集中管理，后续加 data-testid。/tags 组件近期不会重构。
- **[风险] 本地 LLM 响应慢导致超时** → 缓解：长操作 skip 而非 fail，用户可调超时或重跑。
- **[风险] 测试数据残留影响后续运行** → 缓解：手动清理。单用户本地系统，成本极低。
- **[Trade-off] 不 mock = 测试不可重复** → 真实环境验收本就依赖数据状态。无数据时 skip 而非 fail。
- **[Trade-off] 不加 data-testid = 选择器脆弱** → 当前可接受。集中管理降低维护成本。
