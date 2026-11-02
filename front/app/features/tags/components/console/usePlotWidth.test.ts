/**
 * usePlotWidth（R37/R40）：SVG 异步出现/消失时重新测宽，观察/断开 RO。
 * happy-dom 无布局：统一打桩真实宽度。
 */
import { describe, expect, it } from 'vitest'
import { defineComponent, nextTick, ref } from 'vue'
import { mount } from '@vue/test-utils'
import { usePlotWidth } from './usePlotWidth'

const RECT = { left: 0, top: 0, width: 375, height: 88, right: 375, bottom: 88, x: 0, y: 0 }

const Host = defineComponent({
  props: { show: { type: Boolean, default: false } },
  setup() {
    const plot = ref<SVGSVGElement | null>(null)
    const width = usePlotWidth(plot)
    return { plot, width }
  },
  template: '<div><svg v-if="show" ref="plot" /><span data-testid="width">{{ width }}</span></div>',
})

describe('usePlotWidth — SVG 晚到/消失', () => {
  it('首挂无 SVG 宽 0；晚到后 375；消失回 0（watch flush post）', async () => {
    SVGSVGElement.prototype.getBoundingClientRect = () => RECT as DOMRect
    const wrapper = mount(Host, { props: { show: false } })
    expect(wrapper.find('[data-testid="width"]').text()).toBe('0')

    await wrapper.setProps({ show: true })
    await nextTick()
    await nextTick()
    expect(wrapper.find('[data-testid="width"]').text()).toBe('375')

    await wrapper.setProps({ show: false })
    await nextTick()
    await nextTick()
    expect(wrapper.find('[data-testid="width"]').text()).toBe('0')
  })
})
