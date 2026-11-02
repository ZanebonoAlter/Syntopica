<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

# UI Design: add-ghost-mount-unmount

## 复用契约（minor 档）

全部复用体检面板现有「未收编候选」卡片的交互契约（`front/app/features/tags/components/BoardHealthPanel.vue`），无新布局模式、无新弹窗档：

1. **卸载建议清单**（BoardHealthPanel 新区块）：同未收编候选卡片形态——列表行（tag label + 真家板名 + gap 值）+ 复选框 + 「批量卸载」按钮；空态文案「无卸载建议」；卸载后行内消失。
2. **墓碑列表/撤销**：同卡片内次级区块或抽屉（现有面板的展开模式）——列表行（tag label + 板名 + 卸载时间）+ 逐行「恢复」按钮；恢复后行内消失。
3. **反馈**：操作走现有 api client 的 loading/error 处理；批量卸载按钮复用现有确认交互（如有）或无确认直接执行（对齐未收编候选「一键收编」的无确认模式）。

无 `ui-prototype/` 需求（不引入新布局/新组件形态，全部为既有卡片的同构实例）。
