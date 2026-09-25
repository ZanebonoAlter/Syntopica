import { defineComponent, h, ref as vueRef } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useMermaidRender } from './useMermaidRender'

/**
 * useMermaidRender（render-mermaid-diagrams tasks 1.1–1.4）：
 * 合法块成图（源码 pre 消失）、多块按序编号 + %% 标题、无 mermaid 块零加载、
 * 非法语法无损降级（源码保留 + 提示行）、混合块互不影响、主题切换重绘、
 * 内容变化后重渲染。chunk 加载失败场景见 useMermaidRender.offline.test.ts
 * （vi.mock 工厂不同须分文件）。
 */

const { renderMock, initializeMock } = vi.hoisted(() => ({
	renderMock: vi.fn(),
	initializeMock: vi.fn(),
}))

vi.mock('mermaid', () => ({
	default: { initialize: initializeMock, render: renderMock },
}))

const SVG_A = '<svg data-test="svg-a"><g>graph-a</g></svg>'
const SVG_B = '<svg data-test="svg-b"><g>graph-b</g></svg>'

function mermaidPre(src: string): string {
	const escaped = src.replace(/&/g, '&amp;').replace(/</g, '&lt;')
	return `<pre><code class="language-mermaid">${escaped}</code></pre>`
}

/** 最小宿主：innerHTML 渲染 content，composable 扫描其子树。 */
function mountHarness(initialContent: string) {
	const host = vueRef<HTMLElement | null>(null)
	const content = vueRef(initialContent)
	const bump = vueRef(0) // 仅进 source getter 不进 DOM：触发重扫但 DOM 不重建（测同 DOM 幂等）
	const Harness = defineComponent({
		setup() {
			useMermaidRender(host, () => `${bump.value}|${content.value}`)
			return () => h('div', { ref: host, innerHTML: content.value })
		},
	})
	const wrapper = mount(Harness)
	lastWrapper = wrapper
	return {
		wrapper,
		setContent(next: string) { content.value = next },
		rescanSameDom() { bump.value += 1 },
	}
}

let lastWrapper: ReturnType<typeof mount> | null = null

async function settle(times = 4): Promise<void> {
	for (let i = 0; i < times; i++) await flushPromises()
	await new Promise(resolve => setTimeout(resolve, 0))
	await flushPromises()
}

beforeEach(() => {
	renderMock.mockReset()
	initializeMock.mockReset()
	delete document.documentElement.dataset.theme
})

afterEach(() => {
	// 卸载触发 onScopeDispose：断开 observer + 在途任务过期，避免跨测试污染 mock 计数
	lastWrapper?.unmount()
	lastWrapper = null
})

describe('useMermaidRender', () => {
	it('合法 mermaid 块原位替换为图：源码 pre 消失、.mermaid-block svg 出现、图题为「图 1」', async () => {
		renderMock.mockResolvedValue({ svg: SVG_A })
		const h = mountHarness(`<p>正文</p>${mermaidPre('graph TD; A-->B')}<p>尾段</p>`)
		await settle()

		const block = h.wrapper.find('.mermaid-block')
		expect(block.exists()).toBe(true)
		expect(block.find('svg').exists()).toBe(true)
		expect(h.wrapper.find('pre code.language-mermaid').exists()).toBe(false)
		expect(block.find('.fig-cap').text()).toBe('图 1')
		// 原位：容器仍在正文段落之间
		expect(h.wrapper.find('.mermaid-block + p').text()).toBe('尾段')
	})

	it('多块按宿主顺序编号，%% 注释行提为图题（截 40 字）', async () => {
		renderMock.mockResolvedValueOnce({ svg: SVG_A }).mockResolvedValueOnce({ svg: SVG_B })
		const h = mountHarness(
			`${mermaidPre('%% 文章处理链全景\ngraph TD; A-->B')}${mermaidPre('flowchart LR; C-->D')}`,
		)
		await settle()

		const caps = h.wrapper.findAll('.fig-cap')
		expect(caps).toHaveLength(2)
		expect(caps[0]!.text()).toBe('图 1 · 文章处理链全景')
		expect(caps[1]!.text()).toBe('图 2')
		expect(h.wrapper.findAll('.mermaid-block svg')).toHaveLength(2)
	})

	it('无 mermaid 块零行为：不加载 mermaid（initialize/render 均不调用）', async () => {
		const h = mountHarness('<p>普通正文</p><pre><code class="language-js">const a = 1</code></pre>')
		await settle()

		expect(h.wrapper.find('.mermaid-block').exists()).toBe(false)
		expect(initializeMock).not.toHaveBeenCalled()
		expect(renderMock).not.toHaveBeenCalled()
	})

	it('非法语法无损降级：源码 pre 保留 + 一行 warning 提示 + failed 标记', async () => {
		renderMock.mockRejectedValue(new Error('Parse error on line 1:\ngraph TD; A->'))
		const src = 'graph TD; A->'
		const h = mountHarness(mermaidPre(src))
		await settle()

		const pre = h.wrapper.find('pre')
		expect(pre.exists()).toBe(true)
		expect(pre.attributes('data-mermaid-failed')).toBe('true')
		const hint = h.wrapper.find('.mermaid-error-hint')
		expect(hint.exists()).toBe(true)
		expect(hint.text()).toContain('图渲染失败')
		expect(hint.text()).toContain('Parse error on line 1:')
		expect(hint.text()).toContain('已保留源码')
		expect(h.wrapper.find('.mermaid-block').exists()).toBe(false)
	})

	it('混合块互不影响：非法块降级的同时合法块正常成图；同 DOM 重扫不重复插提示', async () => {
		renderMock.mockRejectedValueOnce(new Error('Parse error'))
		renderMock.mockResolvedValueOnce({ svg: SVG_B })
		const failedSrc = 'broken -->'
		const h = mountHarness(`${mermaidPre(failedSrc)}${mermaidPre('graph TD; OK-->YES')}`)
		await settle()

		expect(h.wrapper.findAll('.mermaid-block svg')).toHaveLength(1)
		expect(h.wrapper.findAll('.mermaid-error-hint')).toHaveLength(1)

		// 同 DOM 重扫（bump 只改 source getter，innerHTML 不重建）：失败块不重试、提示不重复
		h.rescanSameDom()
		await settle()
		expect(renderMock).toHaveBeenCalledTimes(2) // 只含首扫两块，无失败块重试
		expect(h.wrapper.findAll('.mermaid-error-hint')).toHaveLength(1)
	})

	it('主题切换：data-theme 变 dark 后按新主题 re-initialize 并重绘已渲染图', async () => {
		renderMock.mockResolvedValue({ svg: SVG_A })
		const h = mountHarness(mermaidPre('graph TD; A-->B'))
		await settle()
		expect(renderMock).toHaveBeenCalledTimes(1)

		document.documentElement.dataset.theme = 'dark'
		await settle()

		// 重绘：initialize 以 dark 再执行，同源再次 render，容器仍是同一个
		expect(initializeMock.mock.calls.some(c => (c[0] as Record<string, unknown>).theme === 'dark')).toBe(true)
		expect(renderMock).toHaveBeenCalledTimes(2)
		expect(h.wrapper.findAll('.mermaid-block svg')).toHaveLength(1)
		expect(h.wrapper.find('.fig-cap').text()).toBe('图 1')
	})

	it('内容变化后重渲染：旧容器随 v-html 消失，新源码块重新成图', async () => {
		renderMock.mockResolvedValue({ svg: SVG_A })
		const h = mountHarness(mermaidPre('graph TD; A-->B'))
		await settle()
		expect(h.wrapper.findAll('.mermaid-block')).toHaveLength(1)

		h.setContent(`<p>换了一篇文章</p>${mermaidPre('flowchart LR; X-->Y')}`)
		await settle()

		expect(h.wrapper.findAll('.mermaid-block')).toHaveLength(1)
		expect(h.wrapper.find('.mermaid-block svg').attributes()['data-test']).toBe('svg-a')
		expect(h.wrapper.find('pre code.language-mermaid').exists()).toBe(false)
	})

	it('裸围栏（无语言标注）首词为图类型时识别为 mermaid；带语言标注的块不猜', async () => {
		renderMock.mockResolvedValue({ svg: SVG_A })
		const h = mountHarness(
			'<p>前</p>'
			+ '<pre><code>flowchart TD\n    A[定义SLO] --&gt; B{控制?}</code></pre>'
			+ '<pre><code class="language-js">graph = { TD: 1 }</code></pre>'
			+ '<pre><code>const x = 1</code></pre>',
		)
		await settle()

		// 裸围栏 flowchart 开头 → 成图；language-js 与普通代码块 → 保持源码
		expect(h.wrapper.findAll('.mermaid-block svg')).toHaveLength(1)
		expect(h.wrapper.find('pre code.language-js').exists()).toBe(true)
		expect(h.wrapper.findAll('pre')).toHaveLength(2)
		expect(renderMock).toHaveBeenCalledTimes(1)
	})

	it('空宿主 ref 与卸载后扫描安全无异常', async () => {
		const host = vueRef<HTMLElement | null>(null)
		const Harness = defineComponent({
			setup() {
				useMermaidRender(host, () => 'x')
				return () => h('div')
			},
		})
		const w = mount(Harness)
		await settle()
		w.unmount()
		await settle()
		expect(renderMock).not.toHaveBeenCalled()
	})
})
