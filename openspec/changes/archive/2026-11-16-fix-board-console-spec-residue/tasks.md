# Tasks · fix-board-console-spec-residue

> 纯 spec 文本修订 change：不产出代码，不跑测试，只做文档一致性校验。

## 1. spec 修订

- [x] 1.1 把 `specs/workshop-board-console/spec.md` 的 MODIFIED 块应用后，主 spec「矩阵视图（泳道 × 日期）」不再出现肯定式的事件线口径。验证：`awk '/^### Requirement: 矩阵视图/,/^### Requirement: 当日线索工作面/' openspec/specs/workshop-board-console/spec.md | grep -c '事件线'` → `0`。

- [x] 1.2 确认主 spec 内「矩阵视图」与「泳道热度口径（线索数单一口径）」两条 requirement 口径一致（同为非综合稿线索数合计 + 行内相对强度）。验证：`awk '/^### Requirement: 矩阵视图/,/^### Requirement: 当日线索工作面/' openspec/specs/workshop-board-console/spec.md | grep -c '非综合稿线索数合计'` → `≥ 1`。

## 2. 测试

- [x] 2.1 本 change 无代码改动，豁免代码测试。验证：`git status --short | grep -E '^ ?[MAD].*(backend-go/|front/)' | grep -c 'fix-board-console'` → `0`（本 change 目录下无前端/后端文件）。

## 3. 文档

<!-- doc-impact: none（纯 openspec spec 修订，不改 docs/reference/ 任何文档） -->

- [x] 3.1 确认无需更新 `docs/reference/`。验证：`bash scripts/harness/doc-impact.sh verify openspec/changes/fix-board-console-spec-residue` → 通过（声明 none，无 flow/api 域文件被要求更新）。

## 4. 验证

- [x] 4.1 `openspec validate fix-board-console-spec-residue --strict` → `Change 'fix-board-console-spec-residue' is valid`。
- [x] 4.2 `bash scripts/harness/check-tasks-tail.sh openspec/changes/fix-board-console-spec-residue` → `✓ tasks.md 尾三节与 doc-impact 声明标记齐全`。
- [x] 4.3 `bash scripts/harness/doc-impact.sh verify openspec/changes/fix-board-console-spec-residue` → 通过。
- [ ] 4.4 归档本 change：主 spec `workshop-board-console` 更新 1 条 requirement，change 移入 `openspec/changes/archive/`。验证：归档后 `grep -c '非综合稿线索数合计' openspec/specs/workshop-board-console/spec.md` → 增加 1。

### Scenario 映射表

| Scenario | 测试文件 |
| --- | --- |
| 渲染矩阵 | 人工：本 change 只改 spec 文本，不改行为；现行实现的矩阵渲染断言见 `front/app/features/tags/components/console/ObservationMatrix.test.ts`（既有，未改） |
| 图例可见 | 人工：同上，图例文案断言见 `front/app/features/tags/components/console/ObservationMatrix.test.ts`（既有，未改） |
| 断档可读 | 人工：同上，断档格断言见 `front/app/features/tags/components/console/ObservationMatrix.test.ts`（既有，未改） |
| 零线索可开 | 人工：同上，零线索格断言见 `front/app/features/tags/components/console/ObservationMatrix.test.ts`（既有，未改） |
