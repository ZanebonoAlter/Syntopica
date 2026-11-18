/**
 * 版块可编辑字段的**共用定义**（change `lane-manual-correction-loop` 6.2）。
 *
 * 契约出处：ui-design「Component Reuse」——版块编辑弹窗复用 `BoardSettingsForm.vue` 的字段定义，
 * **抽取共用**而非复制粘贴（避免再出现第四套并行实现）。
 *
 * 消费方：
 *  - `BoardSettingsForm.vue`（L1 版块设置页签）：名称 / 描述的文案与长度上限取自这里，
 *    其余字段（匹配窗口 / 上下文档位 / 增强开关）是设置页签独有的，不进本表；
 *  - `BoardQuickEditDialog.vue`（L0 雷达行轻编辑，`md` 档）：名称 / 描述 / 排序 / 受保护四字段。
 *
 * 字段元数据（文案 / 长度上限 / 占位符 / 提示）是**唯一事实源**；两处组件各按自己的排版渲染。
 */

/** 版块名称长度上限（后端 label 口径；与设置页签一致）。 */
export const BOARD_LABEL_MAX = 60

/** 版块描述长度上限（后端 description 口径；与设置页签一致）。 */
export const BOARD_DESCRIPTION_MAX = 500

export type BoardEditFieldKey = 'label' | 'description' | 'display_order' | 'protected'

export interface BoardEditFieldDef {
  key: BoardEditFieldKey
  /** 字段名（界面文案）。 */
  label: string
  /** 输入形态（决定消费方渲染哪种控件）。 */
  type: 'text' | 'textarea' | 'number' | 'toggle'
  maxlength?: number
  placeholder?: string
  /** 字段下的一行短提示（状态结论/必要说明，不写操作口径解说）。 */
  hint?: string
}

export const BOARD_LABEL_FIELD: BoardEditFieldDef = {
  key: 'label',
  label: '版块名称',
  type: 'text',
  maxlength: BOARD_LABEL_MAX,
  placeholder: '版块名称',
}

export const BOARD_DESCRIPTION_FIELD: BoardEditFieldDef = {
  key: 'description',
  label: '描述',
  type: 'textarea',
  maxlength: BOARD_DESCRIPTION_MAX,
  placeholder: '这个版块收什么、边界在哪',
}

export const BOARD_DISPLAY_ORDER_FIELD: BoardEditFieldDef = {
  key: 'display_order',
  label: '排序',
  type: 'number',
  placeholder: '0',
}

export const BOARD_PROTECTED_FIELD: BoardEditFieldDef = {
  key: 'protected',
  label: '受保护',
  type: 'toggle',
  hint: '受保护的版块不可归档',
}

/** 轻编辑弹窗的四个字段（顺序即界面顺序）。 */
export const BOARD_EDIT_FIELDS: BoardEditFieldDef[] = [
  BOARD_LABEL_FIELD,
  BOARD_DESCRIPTION_FIELD,
  BOARD_DISPLAY_ORDER_FIELD,
  BOARD_PROTECTED_FIELD,
]

/** 轻编辑的草稿态（未保存的本地副本）。 */
export interface BoardEditDraft {
  label: string
  description: string
  /** 空串 = 未填（提交时按 0 处理）；数字 = 排序值。 */
  displayOrder: number | ''
  protected: boolean
}

/** 版块记录的最小形状（两个消费方的输入都是它或其子集）。 */
export interface BoardEditSource {
  label: string
  description: string
  display_order: number
  protected: boolean
}

/** 由版块记录生成草稿。 */
export function createBoardEditDraft(board: BoardEditSource | null): BoardEditDraft {
  return {
    label: board?.label ?? '',
    description: board?.description ?? '',
    displayOrder: board?.display_order ?? 0,
    protected: board?.protected ?? false,
  }
}

/** 草稿校验：返回错误文案，null = 可保存。 */
export function validateBoardEditDraft(draft: BoardEditDraft): string | null {
  if (draft.label.trim().length === 0) return '版块名称不能为空'
  if (draft.displayOrder !== '' && !Number.isInteger(Number(draft.displayOrder))) return '排序需为整数'
  return null
}

/** 草稿 → `updateBoard` 载荷（只带轻编辑的四个字段，不动设置页签的其它字段）。 */
export function toBoardEditPayload(draft: BoardEditDraft): {
  label: string
  description: string
  display_order: number
  protected: boolean
} {
  return {
    label: draft.label.trim(),
    description: draft.description.trim(),
    display_order: draft.displayOrder === '' ? 0 : Number(draft.displayOrder),
    protected: draft.protected,
  }
}
