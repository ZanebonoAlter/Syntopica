import { describe, expect, it } from 'vitest'
import {
  BOARD_DESCRIPTION_MAX,
  BOARD_EDIT_FIELDS,
  BOARD_LABEL_MAX,
  createBoardEditDraft,
  toBoardEditPayload,
  validateBoardEditDraft,
  type BoardEditDraft,
} from './boardEditFields'

/**
 * 版块可编辑字段的共用定义（change `lane-manual-correction-loop` 6.2）。
 *
 * 覆盖点：字段表齐备性（两个消费方按同一份元数据渲染）、草稿生成、校验分支、载荷映射。
 * 弹窗层的渲染与提交由 `BoardQuickEditDialog.test.ts` 兜底。
 */
describe('boardEditFields', () => {
  it('字段表含名称 / 描述 / 排序 / 受保护，顺序即界面顺序', () => {
    expect(BOARD_EDIT_FIELDS.map(field => field.key)).toEqual([
      'label',
      'description',
      'display_order',
      'protected',
    ])
  })

  it('长度上限取后端口径（名称 60 / 描述 500）', () => {
    expect(BOARD_LABEL_MAX).toBe(60)
    expect(BOARD_DESCRIPTION_MAX).toBe(500)
    expect(BOARD_EDIT_FIELDS.find(f => f.key === 'label')?.maxlength).toBe(BOARD_LABEL_MAX)
    expect(BOARD_EDIT_FIELDS.find(f => f.key === 'description')?.maxlength).toBe(BOARD_DESCRIPTION_MAX)
  })

  it('createBoardEditDraft：按版块记录回填四字段', () => {
    const draft = createBoardEditDraft({
      label: 'AI 前沿',
      description: '模型与工具链',
      display_order: 3,
      protected: true,
    })
    expect(draft).toEqual({
      label: 'AI 前沿',
      description: '模型与工具链',
      displayOrder: 3,
      protected: true,
    })
  })

  it('createBoardEditDraft：版块不存在 → 空草稿（排序 0 / 未受保护）', () => {
    expect(createBoardEditDraft(null)).toEqual({
      label: '',
      description: '',
      displayOrder: 0,
      protected: false,
    })
  })

  it('validateBoardEditDraft：名称只有空白 → 报错', () => {
    const draft: BoardEditDraft = {
      label: '   ',
      description: '',
      displayOrder: 0,
      protected: false,
    }
    expect(validateBoardEditDraft(draft)).toContain('版块名称不能为空')
  })

  it('validateBoardEditDraft：排序非整数 → 报错', () => {
    const draft: BoardEditDraft = {
      label: 'AI 前沿',
      description: '',
      displayOrder: 1.5,
      protected: false,
    }
    expect(validateBoardEditDraft(draft)).toContain('排序需为整数')
  })

  it('validateBoardEditDraft：合法草稿 → null（可保存）', () => {
    const draft: BoardEditDraft = {
      label: 'AI 前沿',
      description: '模型与工具链',
      displayOrder: 3,
      protected: false,
    }
    expect(validateBoardEditDraft(draft)).toBeNull()
  })

  it('toBoardEditPayload：名称与描述去空白，排序为空按 0', () => {
    const payload = toBoardEditPayload({
      label: '  AI 前沿  ',
      description: '  模型与工具链  ',
      displayOrder: '',
      protected: true,
    })
    expect(payload).toEqual({
      label: 'AI 前沿',
      description: '模型与工具链',
      display_order: 0,
      protected: true,
    })
  })

  it('toBoardEditPayload：排序按数值提交，不动受保护开关', () => {
    const payload = toBoardEditPayload({
      label: 'AI 前沿',
      description: '',
      displayOrder: 7,
      protected: false,
    })
    expect(payload.display_order).toBe(7)
    expect(payload.protected).toBe(false)
  })
})
