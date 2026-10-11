import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import SettingsSectionDatasources from './SettingsSectionDatasources.vue'

/**
 * 研究数据源 section（add-fast-frequency-datasources tasks 4.2）：
 * 数据驱动七源卡（目录扩容自动多卡）、fred 无 key disabled 带原因、
 * FRED key 管理块（同 Comtrade 模式：脱敏回填提示 / 空串保存不清除）、
 * probe 成功/失败行内展示。API 层整体 mock（不拉真实后端）。
 */

const listCatalog = vi.fn()
const probe = vi.fn()
const getComtradeStatus = vi.fn()
const saveComtradeSettings = vi.fn()
const getFredStatus = vi.fn()
const saveFredSettings = vi.fn()

vi.mock('~/api/datasources', () => ({
  useDatasourcesApi: () => ({
    listCatalog,
    probe,
    getComtradeStatus,
    saveComtradeSettings,
    getFredStatus,
    saveFredSettings,
  }),
}))

const sevenSources = [
  { code: 'eia_wpsr', name: 'EIA WPSR', coverage: '美国（仅美国）', frequency: 'weekly', typical_lag: '约 3-5 天', status: 'enabled' },
  { code: 'jodi_oil_primary', name: 'JODI Oil', coverage: '96 经济体', frequency: 'monthly', typical_lag: '约 1.5-2 个月', status: 'enabled' },
  { code: 'wb_wdi', name: '世行 WDI', coverage: '全球宏观年度', frequency: 'annual', typical_lag: '随源端更新', status: 'enabled' },
  { code: 'un_comtrade', name: 'UN Comtrade', coverage: '全球双边贸易', frequency: 'monthly+annual', typical_lag: '月度约 4 个月+', status: 'disabled', status_reason: '未配置 COMTRADE_API_KEY', requires_key: true },
  { code: 'yahoo_finance', name: 'Yahoo Finance 日度行情', coverage: '核定 15 symbol', frequency: 'daily', typical_lag: 'T+0', status: 'enabled' },
  { code: 'fred', name: 'FRED 宏观/价格序列', coverage: '80 万+ 序列', frequency: 'daily', typical_lag: 'T+0~T+1', status: 'disabled', status_reason: '未配置 FRED_API_KEY：可在设置界面「研究数据源」配置（即时生效），或用环境变量/config.yaml 兜底', requires_key: true },
  { code: 'gdelt', name: 'GDELT 新闻事件热度', coverage: '全球新闻聚合', frequency: 'daily', typical_lag: '小时级', status: 'enabled' },
]

function mountSection() {
  return mount(SettingsSectionDatasources, {
    global: {
      stubs: { AppToggle: { template: '<button data-testid="toggle" />' } },
    },
  })
}

beforeEach(() => {
  vi.clearAllMocks()
  listCatalog.mockResolvedValue({ success: true, data_sources: sevenSources })
  getComtradeStatus.mockResolvedValue({ success: true, data: { enabled: true, api_key_configured: false } })
  getFredStatus.mockResolvedValue({ success: true, data: { enabled: true, api_key_configured: false } })
})

describe('SettingsSectionDatasources（七源目录 + FRED key）', () => {
  it('数据驱动渲染七张源卡（含三个快频源）', async () => {
    const wrapper = mountSection()
    await flushPromises()

    const cards = wrapper.findAll('.grid > div')
    expect(cards.length).toBe(7)
    const names = cards.map(c => c.text())
    expect(names.join()).toContain('Yahoo Finance 日度行情')
    expect(names.join()).toContain('FRED 宏观/价格序列')
    expect(names.join()).toContain('GDELT 新闻事件热度')
    // 日度频度标签生效
    expect(wrapper.text()).toContain('日度')
  })

  it('fred 无 key 时卡为 disabled 且展示原因（配置 FRED_API_KEY）', async () => {
    const wrapper = mountSection()
    await flushPromises()

    const fredCard = wrapper.findAll('.grid > div').find(c => c.text().includes('FRED'))!
    expect(fredCard.text()).toContain('FRED_API_KEY')
    expect(fredCard.text()).toContain('未配置')
  })

  it('FRED key 管理块：已配置时占位提示末 4 位；空串保存不清除（提交 api_key 空串）', async () => {
    getFredStatus.mockResolvedValue({ success: true, data: { enabled: true, api_key_configured: true, api_key_hint: '9911' } })
    saveFredSettings.mockResolvedValue({ success: true })
    listCatalog.mockResolvedValue({ success: true, data_sources: sevenSources })
    const wrapper = mountSection()
    await flushPromises()

    const fredBlock = wrapper.findAll('h4').find(h => h.text() === 'FRED API Key')!
    expect(fredBlock.exists()).toBe(true)
    const block = fredBlock.element.closest('.border')!
    expect(block.textContent).toContain('留空保存不修改')
    const input = block.querySelector('input') as HTMLInputElement
    expect(input.placeholder).toContain('9911')

    // 空串保存（表单回填空场景）：提交 api_key=""，后端语义=保留原值。
    const saveBtn = wrapper.findAll('button').find(b => b.text() === '保存设置' && b.element.closest('.border') === block)!
    await saveBtn.trigger('click')
    await flushPromises()
    expect(saveFredSettings).toHaveBeenCalledWith({ enabled: true, api_key: '' })
    expect(wrapper.text()).toContain('已保存，即时生效')
  })

  it('probe 成功行内展示耗时；失败展示 detail', async () => {
    probe.mockResolvedValueOnce({ success: true, data: { code: 'yahoo_finance', elapsed_ms: 412, summary: { rows: 21 } } })
      .mockResolvedValueOnce({ success: false, error_code: 'SOURCE_UNAVAILABLE', message: '缺少 FRED API key', detail: '请在设置界面「研究数据源」配置 FRED key' })
    const wrapper = mountSection()
    await flushPromises()

    const cards = wrapper.findAll('.grid > div')
    const yahooCard = cards.find(c => c.text().includes('Yahoo Finance'))!
    const fredCard = cards.find(c => c.text().includes('FRED'))!

    await yahooCard.findAll('button').find(b => b.text() === '测试连通')!.trigger('click')
    await flushPromises()
    expect(yahooCard.text()).toContain('连通正常')
    expect(yahooCard.text()).toContain('412')

    await fredCard.findAll('button').find(b => b.text() === '测试连通')!.trigger('click')
    await flushPromises()
    expect(fredCard.text()).toContain('配置 FRED key')
  })
})
