import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import AnalysisMethodPanel from './AnalysisMethodPanel.vue'

/**
 * AnalysisMethodPanel —— board-level-deep-analysis 2.6。
 *
 * 覆盖：CRUD payload（含四组元数据 textarea → 数组的 trim/滤空/保序转换）、
 * 编辑清空 title/summary 显式发空串、启停 setEnabled（含 legacy 首次启用确认）、
 * 启停开关可访问名、保存失败不关闭编辑框、legacy 迁移提示、删除确认、
 * 旧 reference-roles API 不再被调用。
 */

const apiMock = vi.hoisted(() => ({
  listMethods: vi.fn(),
  createMethod: vi.fn(),
  getMethod: vi.fn(),
  updateMethod: vi.fn(),
  setEnabled: vi.fn(),
  deleteMethod: vi.fn(),
}))

const refRolesMock = vi.hoisted(() => ({
  listRoles: vi.fn(),
  createRole: vi.fn(),
  getRole: vi.fn(),
  updateRole: vi.fn(),
  deleteRole: vi.fn(),
}))

vi.mock('~/api/analysisMethods', () => ({
  useAnalysisMethodsApi: () => apiMock,
}))

vi.mock('~/api/referenceRoles', () => ({
  useReferenceRolesApi: () => refRolesMock,
}))

const notifyMock = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  warn: vi.fn(),
}))

vi.mock('~/composables/useNotify', () => ({
  useNotify: () => notifyMock,
}))

const legacyMethod = {
  id: 1,
  name: 'inside-america',
  title: '内部看美国',
  summary: '旧参考角色迁移的画像',
  selection_meta: { applicable_when: [], avoid_when: [], required_evidence: [], failure_modes: [] },
  content: '分析基因条目…',
  enabled: false,
  legacy: true,
  created_at: '2026-08-28T00:00:00Z',
  updated_at: '2026-08-28T00:00:00Z',
}

const normalMethod = {
  id: 2,
  name: 'causal-check',
  title: '因果链检验',
  summary: '检验因果而非相关',
  selection_meta: {
    applicable_when: ['有时间序列'],
    avoid_when: ['无时间序列'],
    required_evidence: ['数据序列'],
    failure_modes: ['相关误当因果'],
  },
  content: '步骤一\n步骤二',
  enabled: true,
  legacy: false,
  created_at: '2026-08-29T00:00:00Z',
  updated_at: '2026-08-29T00:00:00Z',
}

const stubs = {
  Icon: { name: 'Icon', template: '<i class="icon-stub" />' },
  AppSectionHeader: { name: 'AppSectionHeader', template: '<div class="am-header-stub" />' },
  AppButton: {
    name: 'AppButton',
    props: ['variant', 'size', 'disabled', 'loading', 'type'],
    template: '<button type="button" class="am-btn-stub" :disabled="disabled || loading"><slot /></button>',
  },
  AppToggle: {
    name: 'AppToggle',
    props: ['modelValue', 'disabled', 'label'],
    emits: ['update:modelValue'],
    template: '<button type="button" class="am-toggle-stub" :disabled="disabled" @click="$emit(\'update:modelValue\', !modelValue)"><span>{{ label || \'\' }}</span></button>',
  },
  AppInput: {
    name: 'AppInput',
    props: ['modelValue', 'disabled', 'placeholder', 'type', 'error'],
    emits: ['update:modelValue'],
    template: '<input class="am-input-stub" :disabled="disabled" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
  },
  AppDialog: {
    name: 'AppDialog',
    props: ['modelValue', 'title', 'width'],
    template: '<div v-if="modelValue" class="am-dialog-stub"><div class="am-dialog-body"><slot /></div><div class="am-dialog-footer"><slot name="footer" /></div></div>',
  },
}

function mountPanel(): VueWrapper {
  return mount(AnalysisMethodPanel, { global: { stubs } })
}

function findButton(wrapper: VueWrapper, text: string) {
  return wrapper.findAll('.am-btn-stub').find(b => b.text().includes(text))
}

describe('AnalysisMethodPanel 分析方法管理', () => {
  beforeEach(() => {
    apiMock.listMethods.mockResolvedValue({ success: true, data: [legacyMethod, normalMethod] })
    apiMock.createMethod.mockResolvedValue({ success: true, data: normalMethod })
    apiMock.updateMethod.mockResolvedValue({ success: true, data: normalMethod })
    apiMock.setEnabled.mockResolvedValue({ success: true, data: { ...normalMethod, enabled: false } })
    apiMock.deleteMethod.mockResolvedValue({ success: true, data: { deleted: 2 } })
  })

  afterEach(() => {
    vi.clearAllMocks()
    vi.restoreAllMocks()
  })

  it('渲染方法卡列表，legacy 卡显示迁移提示且默认停用', async () => {
    const wrapper = mountPanel()
    await flushPromises()

    expect(wrapper.text()).toContain('inside-america')
    expect(wrapper.text()).toContain('因果链检验')
    expect(wrapper.text()).toContain('旧画像迁移项：需人工整理适用条件 / 证据边界后再启用')
    expect(wrapper.text()).toContain('旧画像迁移')
    expect(wrapper.text()).toContain('启用中')
    expect(wrapper.text()).toContain('已停用')
    // 只有 legacy 卡带迁移提示
    expect(wrapper.findAll('.am-item__legacy')).toHaveLength(1)
  })

  it('新建方法卡：四组元数据按行转换（trim / 滤空 / 保序）并提交', async () => {
    const wrapper = mountPanel()
    await flushPromises()

    await findButton(wrapper, '新建方法卡')!.trigger('click')
    await flushPromises()

    const inputs = wrapper.findAll('.am-input-stub')
    await inputs[0]!.setValue('  causal-check  ')
    await inputs[1]!.setValue('因果链检验')
    await inputs[2]!.setValue('摘要一句话')

    // textarea 顺序：content + applicable_when + avoid_when + required_evidence + failure_modes
    const textareas = wrapper.findAll('textarea.am-textarea')
    await textareas[0]!.setValue('步骤一\n步骤二')
    await textareas[1]!.setValue('有时间序列\n\n无对照组  ')
    await textareas[2]!.setValue('无时间序列\n无对照组')
    await textareas[3]!.setValue('数据序列\n报告文献')
    await textareas[4]!.setValue('相关误当因果')

    await findButton(wrapper, '保存')!.trigger('click')
    await flushPromises()

    expect(apiMock.createMethod).toHaveBeenCalledWith({
      name: 'causal-check',
      title: '因果链检验',
      summary: '摘要一句话',
      selection_meta: {
        applicable_when: ['有时间序列', '无对照组'],
        avoid_when: ['无时间序列', '无对照组'],
        required_evidence: ['数据序列', '报告文献'],
        failure_modes: ['相关误当因果'],
      },
      content: '步骤一\n步骤二',
      enabled: true,
    })
  })

  it('编辑方法卡：元数据回填为每行一条，提交时转回数组', async () => {
    const wrapper = mountPanel()
    await flushPromises()

    // 列表顺序 [legacy(id1), normal(id2)]，点第二个「编辑」= normalMethod
    const editButtons = wrapper.findAll('.am-btn-stub').filter(b => b.text().includes('编辑'))
    await editButtons[1]!.trigger('click')
    await flushPromises()

    const textareas = wrapper.findAll('textarea.am-textarea')
    expect((textareas[1]!.element as HTMLTextAreaElement).value).toBe('有时间序列')
    expect((textareas[2]!.element as HTMLTextAreaElement).value).toBe('无时间序列')
    expect((textareas[3]!.element as HTMLTextAreaElement).value).toBe('数据序列')
    expect((textareas[4]!.element as HTMLTextAreaElement).value).toBe('相关误当因果')

    // 修改摘要与 required_evidence，提交应携带完整 selection_meta
    await wrapper.findAll('.am-input-stub')[2]!.setValue('已更新摘要')
    await textareas[3]!.setValue('数据序列\n报告文献\n历史对照')

    await findButton(wrapper, '保存')!.trigger('click')
    await flushPromises()

    expect(apiMock.updateMethod).toHaveBeenCalledWith(2, expect.objectContaining({
      summary: '已更新摘要',
      selection_meta: {
        applicable_when: ['有时间序列'],
        avoid_when: ['无时间序列'],
        required_evidence: ['数据序列', '报告文献', '历史对照'],
        failure_modes: ['相关误当因果'],
      },
      content: '步骤一\n步骤二',
      enabled: true,
    }))
  })

  it('新建留空 title/summary：保持可选语义不传', async () => {
    const wrapper = mountPanel()
    await flushPromises()

    await findButton(wrapper, '新建方法卡')!.trigger('click')
    await flushPromises()

    const inputs = wrapper.findAll('.am-input-stub')
    await inputs[0]!.setValue('blank-optional')
    await wrapper.findAll('textarea.am-textarea')[0]!.setValue('步骤一')

    await findButton(wrapper, '保存')!.trigger('click')
    await flushPromises()

    const body = apiMock.createMethod.mock.calls[0]![0] as Record<string, unknown>
    expect(body.name).toBe('blank-optional')
    expect(body.title).toBeUndefined()
    expect(body.summary).toBeUndefined()
  })

  it('编辑时清空 title/summary：payload 显式携带空串（而非丢字段）', async () => {
    const wrapper = mountPanel()
    await flushPromises()

    const editButtons = wrapper.findAll('.am-btn-stub').filter(b => b.text().includes('编辑'))
    await editButtons[1]!.trigger('click') // normalMethod(id 2)
    await flushPromises()

    // inputs 顺序：name（编辑态禁用）/ title / summary
    const inputs = wrapper.findAll('.am-input-stub')
    await inputs[1]!.setValue('')
    await inputs[2]!.setValue('')

    await findButton(wrapper, '保存')!.trigger('click')
    await flushPromises()

    expect(apiMock.updateMethod).toHaveBeenCalledWith(2, expect.objectContaining({
      title: '',
      summary: '',
    }))
  })

  it('启停调用 setEnabled 接口', async () => {
    const wrapper = mountPanel()
    await flushPromises()

    // 列表按顺序 [legacy(id1 停用), normal(id2 启用)]，每个条目一个 toggle
    const toggles = wrapper.findAll('.am-toggle-stub')
    await toggles[1]!.trigger('click')
    await flushPromises()

    expect(apiMock.setEnabled).toHaveBeenCalledWith(2, false)
  })

  it('每行启停开关带可访问名：label 指明目标方法', async () => {
    const wrapper = mountPanel()
    await flushPromises()

    // 表单未打开，仅列表两行各一个 toggle
    const toggles = wrapper.findAll('.am-toggle-stub')
    expect(toggles).toHaveLength(2)
    expect(toggles[0]!.text()).toContain('启用 内部看美国')
    expect(toggles[1]!.text()).toContain('启用 因果链检验')
  })

  it('legacy 卡首次启用前弹确认：提示已整理适用条件/证据边界，取消不调用 API', async () => {
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false)
    const wrapper = mountPanel()
    await flushPromises()

    await wrapper.findAll('.am-toggle-stub')[0]!.trigger('click') // legacyMethod(id1, 停用→启用)
    await flushPromises()

    expect(confirmSpy).toHaveBeenCalledTimes(1)
    expect(confirmSpy.mock.calls[0]![0]).toContain('适用条件')
    expect(confirmSpy.mock.calls[0]![0]).toContain('证据边界')
    expect(apiMock.setEnabled).not.toHaveBeenCalled()
  })

  it('legacy 卡确认后调用 setEnabled 启用', async () => {
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    apiMock.setEnabled.mockResolvedValue({ success: true, data: { ...legacyMethod, enabled: true } })
    const wrapper = mountPanel()
    await flushPromises()

    await wrapper.findAll('.am-toggle-stub')[0]!.trigger('click')
    await flushPromises()

    expect(apiMock.setEnabled).toHaveBeenCalledWith(1, true)
  })

  it('普通卡启停无需确认', async () => {
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false)
    const wrapper = mountPanel()
    await flushPromises()

    await wrapper.findAll('.am-toggle-stub')[1]!.trigger('click') // normalMethod(启用→停用)
    await flushPromises()

    expect(confirmSpy).not.toHaveBeenCalled()
    expect(apiMock.setEnabled).toHaveBeenCalledWith(2, false)
  })

  it('保存失败（如重名 409）：提示错误且不关闭编辑框', async () => {
    apiMock.updateMethod.mockResolvedValue({ success: false, error: '短名已存在' })
    const wrapper = mountPanel()
    await flushPromises()

    const editButtons = wrapper.findAll('.am-btn-stub').filter(b => b.text().includes('编辑'))
    await editButtons[1]!.trigger('click')
    await flushPromises()

    await findButton(wrapper, '保存')!.trigger('click')
    await flushPromises()

    expect(notifyMock.error).toHaveBeenCalledWith('短名已存在')
    expect(notifyMock.success).not.toHaveBeenCalled()
    // 编辑框保持打开，列表未刷新
    expect(wrapper.find('.am-dialog-stub').exists()).toBe(true)
    expect(apiMock.listMethods).toHaveBeenCalledTimes(1)
  })

  it('删除需确认，确认后调用 deleteMethod', async () => {
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true)
    const wrapper = mountPanel()
    await flushPromises()

    // 点第二个「删除」= normalMethod(id 2)
    const deleteButtons = wrapper.findAll('.am-btn-stub').filter(b => b.text().includes('删除'))
    await deleteButtons[1]!.trigger('click')
    await flushPromises()

    expect(confirmSpy).toHaveBeenCalled()
    expect(apiMock.deleteMethod).toHaveBeenCalledWith(2)
  })

  it('取消删除时不调用 deleteMethod', async () => {
    vi.spyOn(window, 'confirm').mockReturnValue(false)
    const wrapper = mountPanel()
    await flushPromises()

    const deleteButtons = wrapper.findAll('.am-btn-stub').filter(b => b.text().includes('删除'))
    await deleteButtons[1]!.trigger('click')
    await flushPromises()

    expect(apiMock.deleteMethod).not.toHaveBeenCalled()
  })

  it('旧参考角色 API 不再被调用', async () => {
    mountPanel()
    await flushPromises()

    expect(apiMock.listMethods).toHaveBeenCalled()
    expect(refRolesMock.listRoles).not.toHaveBeenCalled()
    expect(refRolesMock.createRole).not.toHaveBeenCalled()
    expect(refRolesMock.updateRole).not.toHaveBeenCalled()
    expect(refRolesMock.deleteRole).not.toHaveBeenCalled()
  })

  it('列表加载失败显示错误与重试', async () => {
    apiMock.listMethods.mockResolvedValue({ success: false, error: '数据库不可用' })
    const wrapper = mountPanel()
    await flushPromises()

    expect(wrapper.text()).toContain('数据库不可用')
    expect(findButton(wrapper, '重试')).toBeTruthy()
  })
})
