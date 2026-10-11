## 1. harness-retro.sh 分类改造

- [x] 1.1 新增白名单常量 `INFORMATIVE_REASONS`（concurrent-mixed / concurrent-dirty-tree / foreign-breakage），注释锚定语义（归属/状态告知、不期待服从动作、与 harness-facts reasonCode 词汇表同步）与缺省纠正型原则
- [x] 1.2 soft 聚合 SQL 拆分类：`soft.items` 失效名单只含纠正型组（`n >= threshold AND reason NOT IN 白名单`）；新增 `informative`（分组计数数组，不过滤阈值）、`warn_total_corrective`、`warn_total_informative`；`warn_total` 保持全量总和
- [x] 1.3 ④ 段渲染：标题改「④ 软提醒失效（纠正型；同 policy+reasonCode 的 warn 计数 ≥ N）」；失效名单下方加「情报型 warn（归属/状态告知，不计失效）: X 条（组1 n1 / 组2 n2 …）」汇总行；两组皆空时维持「（无…）」占位语义
- [x] 1.4 metrics 键：`soft.stale_groups` 收窄为纠正型失效组数（键名不变）；新增 `soft.warn_total_corrective` / `soft.warn_total_informative` / `soft.informative_groups`；usage/`--warn-threshold` 帮助文案注明"仅作用于纠正型失效判定"
- [x] 1.5 基线兼容：确认基线比对逻辑对新键按缺键处理（不报错、不误报差值）；旧基线文件读入正常

## 2. smoke 同步

- [x] 2.1 A1 断言同步新 ④ 段标题（含"纠正型"）
- [x] 2.2 新增 fixture 场景（spec Scenario「混合两类时分开统计」落点）：fixture 库同时插入超阈值 `quality-gate/concurrent-mixed` warn ×6 与超阈值纠正型 warn（如 `test-scope-guard/full-go-test` ×6），断言：失效名单只含 full-go-test；情报型汇总行含 concurrent-mixed=6；`soft.warn_total`=12 可复算
- [x] 2.3 既有断言回归：阈值边界（等于即命中）、低于阈值不出现、`--warn-threshold 0` 拒绝、基线差值与损坏降级全绿
- [x] 2.4 `bash scripts/harness/harness-retro.smoke.sh` 全量绿

## 3. 文档

<!-- doc-impact: none(harness 工具链 change：仅更新 docs/reference/harness/pi-extensions.md 与 .agents/skills/harness-retro/SKILL.md，均不在 8 域路径内；无 flow 业务域变更) -->

- [x] 3.1 `.agents/skills/harness-retro/SKILL.md`：④ 段解读表拆两型（纠正型三选一处置不变；情报型读法=并发频率/成本趋势，不读成"催了没用"）；改进项判据指标名列表补 `soft.warn_total_informative` 等新键
- [x] 3.2 `docs/reference/harness/pi-extensions.md`：变更溯源表加一行（date/change/一句话）

## 4. 真库回检

- [x] 4.1 对真库 `--days 7` 验证：失效名单 3 组→0（本周数据全为情报型）；情报型汇总行 265 条（concurrent-mixed 239 / concurrent-dirty-tree 16 / foreign-breakage 10）与账本直查一致
- [x] 4.2 `--save-baseline` 重落基线（新语义键），供下一轮体检对比

## 5. 测试

- 本 change 为 harness 工具链改造（纯消费侧口径调整，不触碰扩展记账），交付验证 = smoke 全量（含新增 L 组 fixture：同库混入情报型×6 + 纠正型×6，断言分开统计）+ 真库回检（第 4 节）；harness-retro.sh 的既有断言（阈值边界/拒绝非法参数/基线降级）全量回归。

## 6. 验证

1. `bash scripts/harness/harness-retro.smoke.sh` → 全绿（66/66，含 A1 新标题断言、L1/L2/L3 情报型不进失效名单 + JSON soft 键复算 12=6+6）
2. `bash scripts/harness/harness-retro.sh --days 7`（真库）→ ④ 段失效名单 0 组、情报型汇总 265 条与账本直查一致
3. `bash scripts/harness/harness-retro.sh --save-baseline` → `.pi/harness/retro-baseline.json` 含新键（soft.warn_total_corrective / soft.warn_total_informative / soft.informative_groups），旧键 soft.stale_groups 语义收窄为纠正型失效组数
