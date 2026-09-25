import { defineComponent, h, ref as vueRef } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useMermaidRender } from './useMermaidRender'

/**
 * chunk 加载失败降级（render-mermaid-diagrams spec：渲染资源加载失败 → 源码保留 + 提示）。
 * 单独成文件：vi.mock 工厂直接抛错模拟 import('mermaid') 拒绝，与主测试文件的成功 mock 互斥。
 */

vi.mock('mermaid', () => {
	throw new Error('Failed to fetch dynamically imported module')
})

function mermaidPre(src: string): string {
	return `<pre><code class="language-mermaid">${src}</code></pre>`
}

beforeEach(() => {
	delete document.documentElement.dataset.theme
})

describe('useMermaidRender · chunk 加载失败', () => {
	it('mermaid 模块加载拒绝：全部块降级为源码 + 加载失败提示，不抛异常', async () => {
		const host = vueRef<HTMLElement | null>(null)
		const content = vueRef(`<p>正文</p>${mermaidPre('graph TD; A-->B')}`)
		const Harness = defineComponent({
			setup() {
				useMermaidRender(host, () => content.value)
				return () => h('div', { ref: host, innerHTML: content.value })
			},
		})
		const wrapper = mount(Harness)

		for (let i = 0; i < 4; i++) await flushPromises()
		await new Promise(resolve => setTimeout(resolve, 0))
		await flushPromises()

		expect(wrapper.find('.mermaid-block').exists()).toBe(false)
		expect(wrapper.find('pre code.language-mermaid').exists()).toBe(true)
		const hint = wrapper.find('.mermaid-error-hint')
		expect(hint.exists()).toBe(true)
		expect(hint.text()).toContain('渲染资源加载失败')
		expect(hint.text()).toContain('已保留源码')
	})
})
