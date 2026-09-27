import { nextTick, onMounted, onScopeDispose, watch } from 'vue'
import type { Ref } from 'vue'

/**
 * mermaid 图客户端渲染（render-mermaid-diagrams design D1–D4）。
 *
 * 宿主传入容器 ref + 数据源 getter：数据源变化（deep watch）后 nextTick 扫描容器内
 * `pre > code.language-mermaid`，逐块调 mermaid 原地替换为 SVG——方案 B 轻容器：
 * `.mermaid-block` + `.fig-cap` 图题「图 N · 标题」（标题取源码首个 %% 注释行，
 * 截 40 字）。mermaid chunk 动态按需加载（无 mermaid 块的页面零下载），模块级
 * 单例防重复加载，失败后允许后续扫描重试；渲染失败无损降级——保留源码块 +
 * 一行 warning 提示（spec: markdown-mermaid-render），单块失败不影响同宿主其他块。
 *
 * 主题（documentElement data-theme）切换经 MutationObserver 触发已渲染图按新主题
 * 重绘（editorial→default，dark→dark），版本 token 丢弃过期异步任务防快速连点竞态。
 * 全部副作用位于 onMounted 之后（客户端），SSR 零路径。
 *
 * CSS 契约：components/article/ArticleContent.css `.mermaid-block` 段（纯增量，
 * 共享宿主红线合规）。
 */

type MermaidModule = typeof import('mermaid')

// —— 模块级单例：全应用只加载一次 mermaid chunk（design D3） ——
let mermaidPromise: Promise<MermaidModule> | null = null
let initializedTheme = ''
let renderSeq = 0 // 全局递增，避免 mermaid.render 内部 DOM id 冲突

function currentThemeName(): 'dark' | 'default' {
	return document.documentElement.dataset.theme === 'dark' ? 'dark' : 'default'
}

function loadMermaid(): Promise<MermaidModule> {
	if (!mermaidPromise) {
		mermaidPromise = import('mermaid')
			.then((mod) => {
				mod.default.initialize({ startOnLoad: false, securityLevel: 'strict', theme: currentThemeName() })
				initializedTheme = currentThemeName()
				return mod
			})
			.catch((err: unknown) => {
				// 加载失败后清空缓存，允许后续扫描重试（如网络恢复）
				mermaidPromise = null
				throw err
			})
	}
	return mermaidPromise
}

function ensureTheme(mod: MermaidModule): void {
	const theme = currentThemeName()
	if (theme !== initializedTheme) {
		mod.default.initialize({ startOnLoad: false, securityLevel: 'strict', theme })
		initializedTheme = theme
	}
}

// 已渲染容器 → 原 mermaid 源码（主题重绘复用；容器随 v-html 重建后 WeakMap 自然回收）
const sourceByContainer = new WeakMap<HTMLElement, string>()

/** 图题文案：源码首个 `%%` 注释行作标题（截 40 字），否则仅「图 N」。 */
function captionText(source: string, index: number): string {
	const m = source.match(/^\s*%%\s*(\S.*)$/m)
	const title = m ? (m[1] ?? '').trim().slice(0, 40) : ''
	return title ? `图 ${index} · ${title}` : `图 ${index}`
}

function failureMessage(err: unknown): string {
	const raw = String((err as Error | null)?.message ?? err).split('\n')[0] ?? ''
	return raw.length > 80 ? `${raw.slice(0, 80)}…` : raw
}

// 裸围栏识别（render-mermaid-diagrams apply 修正：firecrawl 抓取常丢语言标记，
// 围栏变为无类名 `code`；按 mermaid 图类型首词识别，误伤面≈0，误判时解析失败
// 仍会降级回源码）。
const MERMAID_BARE_FENCE_RE = new RegExp(
	'^\\s*(?:flowchart|graph\\s+(?:TD|TB|LR|RL|BT)|sequenceDiagram|classDiagram|stateDiagram|erDiagram|journey|gantt|pie|mindmap|timeline|gitGraph|xychart-beta|quadrantChart|sankey-beta|block-beta|architecture-beta|C4(?:Context|Container|Dynamic|Deployment))\\b',
	'i',
)

function isMermaidCode(code: HTMLElement): boolean {
	if (code.classList.contains('language-mermaid')) return true
	// 带其它语言标注（hljs/js/py…）的块不猜，仅识别无语言标注的裸代码块
	if (/[\w-]*language-[\w-]/.test(code.className)) return false
	return MERMAID_BARE_FENCE_RE.test((code.textContent ?? '').trimStart())
}

function markFailed(pre: HTMLElement, message: string): void {
	if (pre.dataset.mermaidFailed) return
	pre.dataset.mermaidFailed = 'true'
	const hint = document.createElement('div')
	hint.className = 'mermaid-error-hint'
	hint.textContent = `⚠ 图渲染失败：${message}。已保留源码，可复制排查。`
	pre.insertAdjacentElement('afterend', hint)
}

/**
 * 挂接 mermaid 渲染到宿主容器。
 * @param host   宿主容器 ref（扫描范围 = 其子树；v-for/多块宿主挂公共祖先即可）
 * @param source 数据源 getter（deep watch；内容变化 → 重扫描；幂等，已渲染块跳过）
 */
export function useMermaidRender(host: Ref<HTMLElement | null | undefined>, source: () => unknown): void {
	let seq = 0 // 版本 token：过期扫描/重绘直接丢弃（快速切主题/内容连变竞态）
	let stopWatch: (() => void) | null = null
	let themeObserver: MutationObserver | null = null

	async function scanAndRender(): Promise<void> {
		const mySeq = ++seq
		const root = host.value
		if (!root) return
		await nextTick()
		if (mySeq !== seq) return
		const codes = Array.from(root.querySelectorAll<HTMLElement>('pre > code')).filter(isMermaidCode)
		if (codes.length === 0) return
		// 已失败块跳过（保留源码 + 提示，不重复插提示）；重新挂载后 DOM 全新，自然重试
		const pending = codes.filter((code) => {
			const pre = code.parentElement
			return pre !== null && !pre.dataset.mermaidFailed
		})
		if (pending.length === 0) return

		let mod: MermaidModule
		try {
			mod = await loadMermaid()
		} catch {
			for (const code of pending) {
				const pre = code.parentElement
				if (pre) markFailed(pre, '渲染资源加载失败')
			}
			return
		}
		if (mySeq !== seq) return
		ensureTheme(mod)

		let figureNo = root.querySelectorAll('.mermaid-block').length
		for (const code of pending) {
			if (mySeq !== seq) return
			const pre = code.parentElement as HTMLElement
			const src = (code.textContent ?? '').trim()
			if (!src) continue
			renderSeq += 1
			try {
				const { svg } = await mod.default.render(`mmd-syntopica-${renderSeq}`, src)
				if (mySeq !== seq) return
				figureNo += 1
				const container = document.createElement('div')
				container.className = 'mermaid-block'
				container.insertAdjacentHTML('afterbegin', svg)
				const cap = document.createElement('span')
				cap.className = 'fig-cap'
				cap.textContent = captionText(src, figureNo)
				container.appendChild(cap)
				sourceByContainer.set(container, src)
				pre.replaceWith(container)
			} catch (err) {
				markFailed(pre, failureMessage(err))
			}
		}
	}

	/** 主题切换：已渲染图按新主题重绘（同源重渲染，失败保留旧图），随后补扫遗漏块。 */
	async function rerenderForTheme(theme: 'dark' | 'default'): Promise<void> {
		const root = host.value
		if (!root) return
		const mySeq = ++seq // 先占版本号：load 期间来新任务/卸载（seq++）即自动过期
		let mod: MermaidModule
		try {
			mod = await loadMermaid()
		} catch {
			return // 加载不可用：保留现有图，不重绘
		}
		if (mySeq !== seq) return
		mod.default.initialize({ startOnLoad: false, securityLevel: 'strict', theme })
		initializedTheme = theme
		const containers = Array.from(root.querySelectorAll<HTMLElement>('.mermaid-block'))
		let figureNo = 0
		for (const container of containers) {
			const src = sourceByContainer.get(container)
			if (src === undefined) continue
			figureNo += 1
			renderSeq += 1
			try {
				const { svg } = await mod.default.render(`mmd-syntopica-${renderSeq}`, src)
				if (mySeq !== seq) return
				// 只替换 svg、保留 .fig-cap（innerHTML 整体赋值会把图题一起清掉）
				container.querySelectorAll('svg').forEach(el => el.remove())
				container.insertAdjacentHTML('afterbegin', svg)
				const cap = container.querySelector('.fig-cap')
				if (cap) cap.textContent = captionText(src, figureNo)
			} catch {
				// 同源重绘失败（理论罕见）：保留旧图
			}
		}
		// 主题切换可能打断了首次扫描（seq 已过期中止），补扫剩余源码块
		await scanAndRender()
	}

	onMounted(() => {
		stopWatch = watch(source, () => { void scanAndRender() }, { immediate: true, deep: true })
		themeObserver = new MutationObserver(() => {
			const theme = currentThemeName()
			if (theme !== initializedTheme) void rerenderForTheme(theme)
		})
		themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] })
	})

	onScopeDispose(() => {
		stopWatch?.()
		themeObserver?.disconnect()
		seq += 1 // 在途异步任务全部过期
	})
}
