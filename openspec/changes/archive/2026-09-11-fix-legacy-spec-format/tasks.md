## 1. A/B 组：骨架与混合修复（4 文件）

- [x] 1.1 branding：加 `# branding Specification` title + `## Purpose`（一句品牌一致性）+ `## Requirements` 头包住既有 requirement；验证 `openspec validate branding --type spec` 通过（validate valid ✓）
- [x] 1.2 lint-zero-debt：`## ADDED Requirements` → title + Purpose（门禁零债务目标）+ `## Requirements`；验证同上（valid ✓）
- [x] 1.3 thread-lineage：保留 DEPRECATED Purpose，补 `## Requirements` 节 + 一条退役声明 requirement（含 Scenario：WHEN 查询 lineage API THEN 404/不存在）；验证同上（退役声明 requirement + 2 Scenario，valid ✓）
- [x] 1.4 theme-system：加 title + Purpose（从 Capability 段一句提炼，注明权威源在 theming.md），`## Requirements` 头插到首个 `### Requirement` 前；验证同上（6R/9S 保留，Purpose 注明权威源在 theming.md，valid ✓）

## 2. C 组：设计文档加壳（3 文件）

- [x] 2.1 settings-workspace：加 title/Purpose（独立设置工作区定位）+ `## Requirements` 节提炼 1~2 条核心行为 requirement（导航分区/领域设置入口，带 Scenario），原节（Navigation/Layout/...）保留为自由内容；验证 validate 通过（壳 2 Scenario 锚定 SettingsWorkspace 实存组件，valid ✓）
- [x] 2.2 unified-dialog：同模板——壳 requirement 锚定 AppDialog size 四档 + 92vw 上限（引用 layout.md 契约），原 API/Structure 节保留；验证通过（壳引用 layout.md dialog 四档契约，valid ✓）
- [x] 2.3 unified-form-controls：同模板——壳 requirement 锚定统一表单原子组件 + 主题 token 响应，原 AppButton/AppToggle/... 节保留；验证通过（壳锚定 App* 组件族实存，valid ✓）

## 3. D 组：补 Scenario（6 文件）

- [x] 3.1 board-concept-management：两条 DEPRECATED requirement 逐条判断（能力已删→合并为一条退役声明 requirement；仍活跃→补 Scenario），其余缺 Scenario 的补 WHEN/THEN；验证通过（子线程：4 退役声明 scenario + 4 SHALL NOT 措辞，valid ✓）
- [x] 3.2 board-management-api：为缺 Scenario 的 requirement（req.3 等）补 Scenario（从 handler 代码现状写）；SHALL/MUST 措辞顺带补正；验证通过（子线程：2 scenario 对照 board_crud_handler.go + 4 措辞，valid ✓）
- [x] 3.3 match-detail-ondemand：req.2 等补 Scenario；措辞补正；验证通过（子线程：1 scenario 对照 board_match_handler.go + 1 措辞，valid ✓）
- [x] 3.4 detective-wall-camera / detective-wall-interaction / detective-wall-scene：逐条补 Scenario（相机/交互/场景三份，req.2~17 中缺的）；验证通过（子线程：camera 3+interaction 4+scene 14 scenario，均对照代码核实，valid ✓）

## 4. 防增量门禁 + 全量验证

- [x] 4.1 check-standards.sh 加 I 段：跑 `openspec validate --specs`，任一失败记 FAIL（与 A-H 段同输出协议）；验证 `bash scripts/check-standards.sh` 含 I 段零失败（I 段挂通，146/0 含新段 ✓）
- [x] 4.2 全量收口：`openspec validate --specs` 101/101 全绿（0 失败）；`bash scripts/check-standards.sh` 全段通过；截图证据记入本任务（101 passed / 0 failed + check-standards 146/0 ✓）

## 5. 测试

- [x] 5.1 纯文档 change：grep 一致性校验——`grep -c "^## Requirements" openspec/specs/{13 个}/spec.md` 全部 ≥1；`openspec validate --specs | grep -c "✗"` = 0（§11.3 纯文档豁免路径）（13 文件 Requirements 节齐全 + validate 零 ✗ ✓）

## 6. 文档

<!-- doc-impact: none(纯 openspec spec 格式修复 + 检查脚本，无 reference 活文档变更；无 flow 影响，§12.2 豁免) -->

- [x] 6.1 无 docs/reference/ 变更；无 flow 影响（tasks 声明留痕）（无 reference 变更留痕 ✓）

## 7. 验证

- [x] 7.1 `openspec validate --specs 2>&1 | tail -1` → 期望 `101 passed, 0 failed`（实测 Totals: 101 passed, 0 failed ✓）
- [x] 7.2 `bash scripts/check-standards.sh 2>&1 | tail -3` → 期望 0 失败（含新 I 段）（实测 通过 146 / 失败 0 ✓）

| Scenario | 测试文件 |
|---|---|
| （skip_specs change：无 delta Scenario，本表为占位；I 段门禁行为由 4.1 验证） | 人工 |
