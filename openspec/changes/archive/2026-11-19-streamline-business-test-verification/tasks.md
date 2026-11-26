## 1. 机制
- [x] 1.1 关闭白盒提醒和逐 Scenario 对账，保留其他归档行为；更新定向 smoke。
- [x] 1.2 同步永久业务测试准入、临时白盒、真实验证与历史兼容文档。
- [x] 1.3 聚焦 review 制品和 diff，结论在 review.md 留痕。

## 2. 测试
- [x] 运行受影响的归档门禁与 readiness smoke。

## 3. 文档
<!-- doc-impact: standard -->
<!-- doc-impact-excuse: flow=共享工作树的业务流程文档被误归属，本变更不改业务行为; api=并行接口文档被误归属，本变更不改接口; database=并行表结构文档被误归属，本变更不改数据结构 -->

> **无 flow 影响**：本 change 只收敛测试验证纪律（`docs/reference/standard/**` + harness 脚本），
> 不改任何业务域流程语义，故不补 flow 变更溯源行（§12.2 豁免）。
- [x] 同步 `docs/reference/standard/` 的 shared/test-design.md、shared/doc-authoring.md、backend/testing.md、frontend/testing.md、frontend/layout.md。
- [x] 同步 `AGENTS.md`、`docs/reference/开发执行规范.md`、`docs/reference/constraints-index.md`、`docs/reference/harness/pi-extensions.md` 和 harness-facts。

## 4. 验证
- `bash .pi/extensions/tests/run-harness-smoke.sh` → PASS（临时镜像 772/772）；复杂档无白盒文档且没有 Scenario 表时不因此拦截。
- `LC_ALL=C bash scripts/harness/archive-readiness.smoke.sh` → PASS（16/16）。
- `openspec validate streamline-business-test-verification --strict` → PASS。
- `bash scripts/harness/archive-readiness.sh streamline-business-test-verification` → 六项通过；文档节仅对确认属于并行工作的三项误归属留痕，未绕过归档门禁。
