/**
 * 行尾「⋯」操作菜单的条目类型（change `lane-manual-correction-loop` 6.3 / 6.5）。
 *
 * 独立成 .ts（不放在 SFC 里）：消费方（矩阵行头、雷达行尾）只取类型，不引组件。
 */
export interface RowActionMenuItem {
  /** 稳定键（消费方按 key 分派动作）。 */
  key: string
  /** 动作标签。 */
  label: string
  /** 危险动作（删除/归档类）用危险色。 */
  danger?: boolean
  /** 禁用时不可点。 */
  disabled?: boolean
  /** 禁用原因 / 补充状态（短句）。 */
  hint?: string
}
