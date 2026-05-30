Status: ready-for-agent

## What to build

修复添加板块对话框关闭后再次点击"添加板块"按钮导致页面异常导航的问题。

根因需先通过浏览器复现确认，可能原因：Teleport overlay 关闭后 pointer-events 残留、或按钮 focus 后 Enter 键触发导航。

修复方向：根据复现结果选择方案（Dialog overlay 清理 / `@click.stop` 防冒泡 / nextTick 确保组件卸载）。

## Acceptance criteria

- [ ] 用浏览器在 http://localhost:3000/tags 复现 Bug，确认根因
- [ ] 根据根因选择修复方案并实现
- [ ] 添加板块 → 取消 → 再次添加板块 → 不再导航
- [ ] 添加板块 → 确认 → 成功创建板块 → 再次添加 → 正常
- [ ] `pnpm lint` + `pnpm exec nuxi typecheck` 通过

## Blocked by

None - can start immediately

## Reference

- Plan: `docs/plans/2026-05-17-fix-tags-page-interaction.md` Task 3
- `AddSectorDialog.vue:20-22`, `TagsPage.vue:301-306`, `SectorList.vue:92-100`
