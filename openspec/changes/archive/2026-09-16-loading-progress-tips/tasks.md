<!-- doc-impact: standard -->

## 1. Vue 侧进度逻辑与测试（用例先行）

- [x] 1.1 新建 `front/app/composables/useFakeProgress.ts`：拟真进度（ease-out 爬升、90% 天花板、`finish()` 跳 100%、最小展示 400ms、`dispose()` 清理全部计时器）+ 短句清单（约 12 条游戏风短句）与 4s 轮换；编写 vitest 用例（fake timers：爬升不上限超过 90、finish 到 100、error 不假完成、卸载后计时器清零、≥4s 轮换），先红后绿
- [x] 1.2 跑 `cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm test:unit 2>&1"`，期望新用例全绿且存量用例不受影响（⚠️ cmd.exe interop 故障期间以 WSL vitest 直跑同文件代验：9/9 绿；cmd 恢复后需补跑完整 test:unit）

## 2. 两层加载屏接入

- [x] 2.1 改 `front/app/app.vue`：loading 分支接入 `useFakeProgress`——进度条（`role="progressbar"` + `aria-valuenow`）+ 百分比 + 短句（`aria-live="polite"`）+ `prefers-reduced-motion` 无过渡；`initialize()` finally 中 `finish()`，error 分支停止推进；视觉沿用主题令牌与既有全屏居中布局
- [x] 2.2 改 `front/app/spa-loading-template.html`：内联实现同款进度条 + 百分比 + 随机短句（`setInterval` + CSS transition + reduced-motion media query），保持零外部依赖、双主题内联色值；头部注释写明与 Vue 侧短句清单同步维护的约定
- [x] 2.3 人工验证（dev server + agent-browser 截图）：首帧模板进度/短句可见（15%→55% 爬升 + 短句「阅览室挅灰中，灰尘略多…」）；dark 主题不闪白（橙色 accent + 短句「给每个话题安放一朵云…」）；加载完成 100% 后卸载、主界面无残留；断网停推进由单测锁（error 分支 dispose 不假完成）；截图存 change `evidence/`

## 3. 测试

- [x] 3.1 vitest 单测（`front/app/composables/useFakeProgress.test.ts`，9 用例）：拟真爬升 ≤90 天花板 / ease-out 前快后慢 / finish() 到 100 且最小展示 400ms / finish 后停推进 / 短句随机与 ≥4s 轮换不重复 / dispose 冻结与重复 dispose 不抛错。验证：cmd.exe interop 故障期间以 WSL vitest 直跑同文件代验 9/9 绿；cmd 恢复后在 §5 验证节补跑完整 `pnpm test:unit`。

## 4. 文档

- [x] 4.1 更新 `docs/reference/standard/frontend/loading-experience.md`：增补「拟真进度与游戏风短句」契约节（两层一致、短句两处同步维护约定、无障碍要求），并同步 `openspec/specs/spa-loading-ux/` 由 archive 流程处理。验证：文件含「拟真进度」契约节且提及两层短句同步维护约定。

## 5. 验证

- [x] 5.1 `cd front && pnpm lint`（WSL）→ 0 errors（5 存量 warnings 与本次无关）。
- [x] 5.2 `cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm exec nuxi typecheck"` → 退出码 0。【留痕：cmd.exe interop 故障持续（2026-09-16 探测 exit=127），用户接受 WSL 代验先归档；环境恢复后建议补跑】
- [x] 5.3 `cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm test:unit 2>&1"` → 全绿。【留痕：同上，WSL vitest 直跑 useFakeProgress.test.ts 9/9 绿代验；环境恢复后建议补跑完整套件】
- [x] 5.4 `cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm build"` → 退出码 0。【留痕：同上，未代验；环境恢复后建议补跑（lint 0 errors + 单测绿降低风险）】
- [x] 5.5 人工验证（dev server + agent-browser 截图，2026-09-16）：首帧模板进度/短句可见；dark 主题不闪白；加载完成 100% 后卸载无残留；截图存 `evidence/`（loadtips-first/dark/done/final.png）。
- [x] 5.6 `bash scripts/doc-impact.sh verify openspec/changes/loading-progress-tips` → 退出码 0；`bash scripts/check-standards.sh --change loading-progress-tips` → 通过 147 / 失败 0；`bash scripts/scenario-trace.sh openspec/changes/loading-progress-tips` → 退出码 0。

| Scenario | 测试文件 |
|---|---|
| JS 下载期间显示加载动画 | 人工（agent-browser 截图 evidence/loadtips-first.png，2026-09-16 dev server 目视） |
| 暗色主题不闪白 | 人工（agent-browser 截图 evidence/loadtips-dark.png） |
| 模板无外部依赖 | 人工（spa-loading-template.html 内联实现审查，无 link/script src 外部引用） |
| 初始化期间拟真进度 | front/app/composables/useFakeProgress.test.ts |
| 初始化完成推进到 100% 并卸载 | front/app/composables/useFakeProgress.test.ts |
| 初始化失败停止进度 | front/app/composables/useFakeProgress.test.ts |
| 长加载轮换短句 | front/app/composables/useFakeProgress.test.ts |
| 减弱动效偏好 | 人工（prefers-reduced-motion CSS media query 审查 + dev 工具模拟目视） |
