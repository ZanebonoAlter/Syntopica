<!-- constraint-domains: semantic-board -->

## 1. 生效时机徽标上移（前端）

- [x] 1.1 `front/app/features/tags/components/console/BoardComposeOverlay.vue`：修正模式 `.bco__top` 标题行新增徽标组（`data-testid="lane-correction-effect-badges"`），三枚结论型徽标「立即生效」「次日日报生效」「无需保存」；样式复用既有 `.bco__timing` 族（accent 实线 / 虚线 / 中性），字号 11px，`flex-wrap` 兜底。验证：组件测试断言三枚文案在位。
  —— 落点：`.bco__top` 内新增 `.bco__effect-badges`（`margin-left:auto` 靠右、`flex-wrap`、11px、复用 `.bco__timing` 族并加 `.bco__timing--plain` 中性变体）；测试断言三枚文案在位
- [x] 1.2 同文件：移除修正模式底部页脚 `.bco__foot`（同义小徽标上移，避免两处重复）；新建泳道模式的 `.bco__actions`（取消本次 / 起名并创建）SHALL NOT 受影响。验证：组件测试断言 `lane-correction-foot` 不再渲染且新建模式页脚出口仍在。
  —— 落点：删除 `.bco__foot` 整块模板与对应 scoped 样式；`.bco__timing` 族保留并由顶部徽标组复用，新建模式 `.bco__actions` 未动
- [x] 1.3 同文件：新建泳道模式（`isCorrection === false`）SHALL NOT 渲染徽标组。验证：组件测试断言新建模式无 `lane-correction-effect-badges`。
  —— 落点：徽标组容器带 `v-if="isCorrection"`；测试新增「新建模式不渲染生效时机徽标组」用例

## 2. 测试

- [x] 2.1 更新 `front/app/features/tags/components/console/BoardComposeOverlay.correction.test.ts`：既有页脚断言迁到顶部徽标组断言（`lane-correction-effect-badges` 含三枚文案），补新建模式不渲染断言。验证：`cd front && pnpm test:unit app/features/tags/components/console/BoardComposeOverlay.correction.test.ts --maxWorkers=2` 全绿。
  —— 落点：页脚断言改为「顶部徽标组存在 + 三枚文案 + `lane-correction-foot` 不再渲染」；新增新建模式无徽标组用例；该文件 16 例全绿，同目录 `BoardComposeOverlay.test.ts` 56 例全绿
  —— review R-4/R-5/R-6 修复（断言收紧 + 正锚 + 对比度色阶）

## 3. 文档

<!-- doc-impact: flow -->
- [ ] 3.1 `docs/reference/flow/semantic-board.md`：归档后在 §变更溯源 补一行（日期 / 变更 / 摘要 / 归档位置）；若该文件描述了修正工作面的界面事实，同步补「生效时机徽标常驻顶部」。
- [ ] 3.2 本 change 的 spec delta 同步进主 spec `openspec/specs/lane-correction-surface/spec.md`（由 `openspec archive` 执行）。

## 4. 验证

- [x] 4.1 `cd front && pnpm lint` → 零失败（exit 0；7 warnings 全在其它既有文件，本 change 两个文件零告警）。
  —— 实测：`✖ 7 problems (0 errors, 7 warnings)`（warning 均在 `front/app/features/ai/...`、`front/app/utils/schedulerMeta.test.ts` 等既有文件）
- [x] 4.2 `cd front && pnpm exec nuxi typecheck` → 零失败（exit 0）。
  —— 实测：无 error 输出，仅 Node DEP0205 弃用告警
- [x] 4.3 `cd front && pnpm test:unit app/features/tags/components/console/BoardComposeOverlay.correction.test.ts --maxWorkers=2` → 全部通过。
  —— 实测：两文件同跑 `Test Files 2 passed (2) / Tests 72 passed (72)`（correction 16 + 同目录 `BoardComposeOverlay.test.ts` 56）
- [x] 4.4 `cd front && pnpm build` → 构建成功。
  —— 实测：以 `NUXT_PUBLIC_API_BASE=/api pnpm generate` 执行（含完整 build + 预渲染 8 条路由）；产物校验 `index.html` / `200.html` / `404.html` 齐全后才铺盘
- [x] 4.5 部署 → 静态产物铺到 `backend-go/frontend/` 且站点与后端健康检查通过。
  —— 实测：`GET /` 200（7457B）、`GET /tags/1974` 200、`GET /health` 200；产物内含新文案（`_nuxt/BBS0q6BH.js` 命中「无需保存」）。⚠️ 首次用 `scripts/dev/deploy-frontend.sh` 铺盘时与**另一并发会话的 `pnpm generate` 撞车**（`.output/public` 被对方清空重建，我拷到缺 `index.html` 的半成品 → 站点根路径一度 404 目录列表），已改为「构建 → 校验产物完整 → 临时目录 + 原子换盘」重铺并复验通过
- [x] 4.6 `bash scripts/harness/doc-impact.sh verify openspec/changes/clarify-lane-correction-effect-timing` → 零失败。
  —— 实测：通过（声明 flow / 1 文件）
- [x] 4.7 `bash scripts/harness/check-standards.sh --change clarify-lane-correction-effect-timing` → 零失败。
  —— 实测：317 通过 / 2 失败，2 条失败均为**其它 archive change 未补 flow 溯源**（`2026-11-19-stage-driven-constraint-injection`、`2026-11-19-streamline-business-test-verification`），与本 change 无关（域外红，已记账见下）
- [x] 4.8 `bash scripts/harness/review-trace.sh openspec/changes/clarify-lane-correction-effect-timing` → 退出码 0。
  —— 实测：通过（`review.md` 7 条：R-1 High 已修复 / R-2、R-3 Medium 已修复、已采纳 / R-4~R-7 Low 已修复、已采纳）
- [x] 4.9 UI 验收：`bash scripts/harness/ui-verification-trace.sh openspec/changes/clarify-lane-correction-effect-timing` → 退出码 0（4 条判定条目全 pass，截图在盘）。
  —— 实测：1440×900 与 1920×1080 两档实测徽标组完整在视口内（1920：x=1688 / w=212 / right ≤ innerWidth），证据见 `ui-verification/report.md` 与 `ui-verification/*.png`
- [x] 4.10 域外红记账：4.7 命中的 2 条失败均属其它 active/archive change 的 flow 溯源欠账，非本 change 引起；本 change 影响包（`BoardComposeOverlay*.test.ts`）全绿，无域外红需要登记台账。
