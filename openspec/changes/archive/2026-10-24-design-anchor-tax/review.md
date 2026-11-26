# Review — design-anchor-tax

审查日期：2026-10-19（子线程 `change-review-glm`，模型 zai-coding-cn/glm-5.3 high，只读）
审查范围：change 全套制品 + 本次 diff（design 模板/指令、review 规范第 0 条、harness-retro F 段、smoke N 系列、SKILL ⑦F）。

### R-1 2026-10-19【Medium】绝对路径可解析到仓库外文件，与 spec「仓库内」口径有偏

- **状态**：已修复
- 证据：`scripts/harness/harness-retro.sh` `resolve_file` 的绝对路径分支只判 `os.path.isfile`，`/etc/hostname` 会被算作落点。
- 处置：绝对路径与相对路径统一用 `realpath` 判定必须位于仓库内（`within_repo`，含 `..` 逃逸与软链）；smoke 新增 n-vague D6（仓外临时文件）回归，断言未解析。

### R-2 2026-10-19【Medium】doc-impact verify 当前 FAIL，归档门禁①会拦

- **状态**：已修复
- 证据：`bash scripts/harness/doc-impact.sh verify openspec/changes/design-anchor-tax` → `声明 none 但启发式命中 flow`；根因是 edit.map 归属混入并发 change 的 `backend-go/internal/dataenrichment/service/signal_compose_test.go`。
- 处置：tasks.md 文档节补 `<!-- doc-impact-excuse: flow=... -->`；复跑 verify 通过（见核实记录）。

### R-3 2026-10-19【Medium】D2「低于 50% 升级」判据未机械限定新模板子集

- **状态**：已采纳
- 证据：retro F 段样本=活跃+窗口内归档全量（含旧格式正文兜底），真库实测 22%；直接对全样本套 50% 必然触发升级，与 D5「指标按新样本观察」相悖。
- 处置：design Risks 与 SKILL ⑦F 均补「升级判据只看新模板 change 子集（或同窗口改前/改后差值），不对全样本绝对值套 50%」。

### R-4 2026-10-19【Low】Scenario 标题解析要求引号包裹，模板/规范未写明

- **状态**：已采纳
- 证据：解析器只认 `Scenario「…」/“…”`；`Scenario 空泛落点不算`（无引号）→ 未解析。
- 处置：design 模板注释补示例 `Scenario「空泛落点不算」`；解析器不放宽无引号形态（避免把普通正文误当标题），观测口径与文档一致。

### R-5 2026-10-19【Low】唯一 basename 解析隐含「文件名须带扩展名」

- **状态**：已修复
- 证据：`resolve_file` 条件 `'/' not in path and '.' in path` 使唯一文件 `Dockerfile` 无法命中。
- 处置：去掉 `'.' in path` 条件；smoke 新增 n-mixed D7（落点 `Dockerfile`）断言解析。

### R-6 2026-10-19【Low】proposal 验证基线缺口径限定

- **状态**：已修复
- 证据：proposal Impact 只写 42%/293/38%，未说明探索期宽松口径与 retro 严格口径不可直减。
- 处置：proposal 验证基线行补口径说明并指向 design Context。

## 审毕声明

- 审查范围：上述 6 条已逐条处置；其余维度（口径一致性、模板有效性、只读性、解析器边界、测试充分性、范围漂移）审查未发现 High 问题。
- 复跑：`bash scripts/harness/harness-retro.smoke.sh` 106/106 全绿；`openspec validate design-anchor-tax --strict` 通过；`doc-impact.sh verify` 通过。
