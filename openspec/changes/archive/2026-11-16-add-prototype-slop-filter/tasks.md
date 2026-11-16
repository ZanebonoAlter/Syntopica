# 任务 — add-prototype-slop-filter

> 纯文档 change：不改产品代码。测试节按 `openspec/config.yaml` 执行「纯文档 change 豁免代码测试，但必须附 grep 一致性校验」。

## 1. 规范文本

- [x] 1.1 **新建** `docs/reference/standard/frontend/prototype-content.md` 的 `### Requirement: 占位数据与伪造内容必须区分`：判定依据句（像字段说明 vs 像具体真人）+ 封闭白名单五字面 + 标注载体与剂量 + provenance 审批衔接 + 三条 Scenario（含「诚实占位被接受」）；验证：`grep -c 'johndoe@example.com' docs/reference/standard/frontend/prototype-content.md` → ≥2，`grep -c '诚实占位被接受' …` → ≥1
- [x] 1.2 同文件 `### Requirement: 空态、加载、错误态必须给原因与下一步`：适用范围声明（SPA 加载以 `loading-experience.md` 为准）+ 四态二列表 + 「三种空态不得共用」；验证：`grep -c 'loading-experience.md' docs/reference/standard/frontend/prototype-content.md` → ≥1
- [x] 1.3 同文件 `### Requirement: 视觉与动效的最低否决项`：V1–V6 含「怎么核」列 + **载体分流**（画布形态四行可访问声明落 `interaction-table.md`）+ 三段式裁决；验证：`grep -c '可访问与动效声明' docs/reference/standard/frontend/prototype-content.md` → ≥2
- [x] 1.4 **不写入** em dash 禁令（review R-4 判为收益零、误伤确定）；`—` 仅保留「诚实占位」语义；验证：`grep -c '破折号' docs/reference/standard/frontend/prototype-content.md` → 0
- [x] 1.5 `layout.md` / `theming.md` / `loading-experience.md` 三处补互指；`standard/README.md` 表与目录树、`constraints-index.md` 各登记新文件；验证：`grep -lc 'prototype-content' docs/reference/standard/frontend/{layout,theming,loading-experience}.md docs/reference/standard/README.md docs/reference/constraints-index.md` → 5 个文件均命中

## 2. spec 基线

- [x] 2.1 delta spec 与正文的 **Requirement 标题集合与判据数值** 一致（不要求逐字相同，正文可有更细的说明）；验证：`diff <(grep '^### Requirement' openspec/changes/add-prototype-slop-filter/specs/frontend-interaction-contract/spec.md | sed 's/.*: //') <(grep '^### Requirement' docs/reference/standard/frontend/prototype-content.md | sed 's/.*: //')` → 无差异
- [x] 2.2 归档时由 `openspec archive` 把 delta 并入主 spec；验证：归档后 `grep -c '视觉与动效的最低否决项' openspec/specs/frontend-interaction-contract/spec.md` → 1

## 3. 审查

- [x] 3.1 派 `change-review` 档子线程审查本 change 的制品与规范文本，产出 `review.md`；审查 brief 须点名：三条判定是否真能人工逐条核对、是否与 `theming.md` / `interaction-conventions.md` 产生双重权威、八条 Scenario 是否都有落点；验证：`bash scripts/harness/review-trace.sh openspec/changes/add-prototype-slop-filter` 通过
- [x] 3.2 处置 review 意见：High 条目状态 MUST 为 已修复 / 已采纳 / 已驳回 / 误报丢弃；验证：`review-trace.sh` 零 High 待处理

## 4. 测试

- [x] 4.1 文档一致性 grep 校验（纯文档 change 替代测试）：`grep -o '[a-z-]*\.md' docs/reference/standard/frontend/prototype-content.md | sort -u` → 所列文件均在 `docs/reference/standard/frontend/` 下存在，无死链
- [x] 4.2 行数预算（拆文件后）：`wc -l docs/reference/standard/frontend/{interaction,prototype-content}.md` → 146 与 117；`interaction.md` 守住 ≤180 自设上限
- [x] 4.3 与既有规范的无冲突性：`theming.md`（取值）、`loading-experience.md`（SPA 加载）、`interaction-conventions.md`（状态标记与数值呈现）三处均已加互指；三段式裁决（取值→theming / 下限→本条 / token 不达标→修 token 而非私自调色）已写入正文

## 5. 文档

<!-- doc-impact: standard -->

- [x] 5.1 更新 `docs/reference/standard/frontend/interaction.md`（见 1.1–1.4）
- [x] 5.2 更新 `docs/reference/standard/frontend/layout.md`（见 1.5）
- [x] 5.3 判定是否需进 `constraints-index.md`：无需新增行（该索引已有 `standard/frontend/interaction.md` 一行，本 change 只是给同一文件加节）
- [x] 5.4 运行 `bash scripts/harness/doc-impact.sh verify openspec/changes/add-prototype-slop-filter` 与 `bash scripts/harness/check-standards.sh`；验证：两者无阻断项

## 6. 验证

- [x] 6.1 制品完整性：`ls openspec/changes/add-prototype-slop-filter/{proposal.md,design.md,ui-design.md,tasks.md}` 与 delta spec 均存在 → 期望结果：5 个文件齐全
- [x] 6.2 openspec 校验：`openspec validate add-prototype-slop-filter` → 期望结果：`Change 'add-prototype-slop-filter' is valid`
- [x] 6.3 归档就绪：`bash scripts/harness/archive-readiness.sh add-prototype-slop-filter` → 期望结果：六项全绿
- [x] 6.4 反向检查（防误伤）：本 change 未对 `front/` `backend-go/` `.pi/extensions/` 执行任何编辑；工作区存在其他 change 的并发脏改，故不用 `git status` 判定，改为核对本 change 的实际改动清单只有 `docs/reference/standard/frontend/{interaction,prototype-content,layout,theming,loading-experience}.md`、`docs/reference/standard/README.md`、`docs/reference/constraints-index.md` 与 change 目录
- [ ] 6.5 回检（归档后观察项）：下一个 `ui-impact: major|minor` 的 change 归档时，其 `ui-prototype/` 若仍出现 `John Doe` / `johndoe@example.com` / 无对照期百分比 delta，则判定纯文本规范无效，须并入 B 阶段脚本化

### Scenario → 落地对账

> 纯文档 change：8 个 Scenario 映射到承载它的文档或人工对读方式（无自动化测试）。

| Scenario | 测试文件 |
| --- | --- |
| 模板假名被驳回 | docs/reference/standard/frontend/prototype-content.md |
| 无对照期的涨幅被驳回 | docs/reference/standard/frontend/prototype-content.md |
| 诚实占位被接受 | docs/reference/standard/frontend/prototype-content.md |
| 无名空态被驳回 | docs/reference/standard/frontend/prototype-content.md |
| 三种空态不得共用 | docs/reference/standard/frontend/prototype-content.md |
| 原型画成无焦点环 | docs/reference/standard/frontend/prototype-content.md |
| 画布形态缺可访问声明 | docs/reference/standard/frontend/prototype-content.md |
| 画布动效忽略减动偏好 | docs/reference/standard/frontend/prototype-content.md |
| 装饰性元素被驳回 | docs/reference/standard/frontend/prototype-content.md |
