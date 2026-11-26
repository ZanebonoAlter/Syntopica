import { afterEach, describe, expect, it, vi } from 'vitest'
import { sanitizeHtml, toPlainText } from './htmlSanitize'

/**
 * 第三方 HTML 清洗（openspec change fix-reader-lede-blank-space task 4.1）。
 *
 * 锚定的是真实事故形态：智源社区等源的 RSS 把整篇文章的 HTML 塞进 description，
 * 根节点带 `style="visibility: visible; opacity: 0;"` —— 内容不可见却照样占位
 * （实测导语段高 8046 px），且 `<img onerror=...>` 一类事件属性经 v-html 直出会真实执行。
 */

/** 智源社区真实样本片段（公众号文章原样搬运，根节点整体不可见）。 */
const ZHIYUAN_SAMPLE = '<div class="rich_media_content js_underline_content defaultNoSetting" id="js_content" style="visibility: visible; opacity: 0; ">'
  + '<h5 style="text-align: center;">克雷西 发自 凹非寺</h5>'
  + '<p style="text-indent: 2em;">正文第一段，介绍本次更新的背景与动机。</p>'
  + '<section style="max-width: 100%;"><img loading="lazy" data-src="https://simg.baai.ac.cn/hub-detail/3a8fed6844ebaaaae1a18986c50781fa1790748001235.webp" class="rich_pages wxw-img"></section>'
  + '</div>'

/** 智源社区真实样本（另含 display:none 藏块与 onerror 事件属性）——两类隐藏分流的判定基准。 */
const ZHIYUAN_SAMPLE_WITH_HIDDEN_BLOCK = '<div class="rich_media_content js_underline_content defaultNoSetting" id="js_content" style="visibility: visible; opacity: 0; ">'
  + '<h5 style="text-align: center;">克雷西 发自 凹非寺</h5>'
  + '<p style="text-indent: 2em;">正文第一段，介绍本次更新的背景与动机。</p>'
  + '<section style="display: none;"><span>SEO关键词堆砌藏身处</span></section>'
  + '<img src="x" onerror="alert(1)">'
  + '</div>'

/**
 * 跨实现共享样本表（design D10）。
 *
 * 后端 `internal/platform/htmltext/htmltext_test.go` 用同一组样本断言**逐字相同**的输出——
 * 两套实现（Go / TS）语言不同、无法共用代码，只能靠这张表锚定「同源同口径」。
 * 期望值不得擅自改：两侧必须同时对齐。
 */
const SHARED_SAMPLES: Array<{ name: string, input: string, expected: string }> = [
  {
    name: '实体样本（先解实体 → 再解析，转义标记被当标签丢弃）',
    input: 'a &amp; b &lt;c&gt; &quot;d&quot; &#39;e&#39; &nbsp;f',
    expected: 'a & b "d" \'e\' f',
  },
  {
    name: '视觉隐藏样本（opacity:0 子树整体剔除）',
    input: '<div style="opacity:0"><p>藏起来的</p></div><p>可见的</p>',
    expected: '可见的',
  },
  {
    name: '标签分隔样本（标签边界视作空白）',
    input: '使用 <br> 标签',
    expected: '使用 标签',
  },
  {
    name: '纯文本样本（无标记 → 原样，仅折叠空白）',
    input: '普通的一段摘要文字，不含任何标记。',
    expected: '普通的一段摘要文字，不含任何标记。',
  },
  {
    name: '语义隐藏样本（display:none 子树整体剔除）',
    input: '<p style="display:none">藏块</p><p>正文</p>',
    expected: '正文',
  },
]

describe('htmlSanitize — toPlainText', () => {
  it('隐藏子树文本不出现（display:none / visibility:hidden / opacity:0 / font-size:0 / hidden 属性）', () => {
    const html = [
      '<div style="display: none;">隐藏一</div>',
      '<div style="visibility: hidden;">隐藏二</div>',
      '<div style="opacity: 0;">隐藏三</div>',
      '<div style="font-size: 0;">隐藏四</div>',
      '<div hidden>隐藏五</div>',
      '<p>可见正文</p>',
    ].join('')
    const text = toPlainText(html)
    expect(text).toBe('可见正文')
    for (const hidden of ['隐藏一', '隐藏二', '隐藏三', '隐藏四', '隐藏五']) {
      expect(text).not.toContain(hidden)
    }
  })

  it('整篇被 opacity:0 包裹的智源样本取不到任何文本（空白占位随之消失）', () => {
    expect(toPlainText(ZHIYUAN_SAMPLE)).toBe('')
  })

  it('智源样本摘要侧整体剔除隐藏子树（不解隐藏，与后端 htmltext.PlainText 同口径）', () => {
    expect(toPlainText(ZHIYUAN_SAMPLE_WITH_HIDDEN_BLOCK)).toBe('')
  })

  it('script / style 内容不出现', () => {
    const html = '<script>window.__x = 1; alert(1)</script><style>.a{color:red}</style><p>正文</p>'
    const text = toPlainText(html)
    expect(text).toBe('正文')
    expect(text).not.toContain('alert')
    expect(text).not.toContain('color:red')
  })

  it('还原 HTML 实体（具名 + 数字）', () => {
    // 与后端同口径：先解实体 → 再解析（`&lt;c&gt;` 解成 `<c>` 后被当标记丢弃）
    expect(toPlainText('a &amp; b &lt;c&gt; &quot;d&quot; &#39;e&#39; &nbsp;f')).toBe('a & b "d" \'e\' f')
  })

  it('实体只解一次（`&amp;lt;` 不被二次解码成 `<`）', () => {
    expect(toPlainText('a&amp;lt;b')).toBe('a&lt;b')
    expect(toPlainText('&amp;hellip;')).toBe('&hellip;')
  })

  it('解实体后转义标记被当标记丢弃（`&lt;b&gt;x` 读作 `x`）', () => {
    expect(toPlainText('&lt;b&gt;x')).toBe('x')
    expect(toPlainText('a&lt;b&gt;c')).toBe('a c')
  })

  it('折叠连续空白为单空格', () => {
    expect(toPlainText('<p>  多个   空白\n\t换行  </p>')).toBe('多个 空白 换行')
  })

  it('超长按 rune 截断并以省略号收尾（总长 ≤ 上限）', () => {
    const text = toPlainText('a'.repeat(500), 300)
    expect(Array.from(text).length).toBe(300)
    expect(text.endsWith('…')).toBe(true)
  })

  it('rune 截断按码点计（代理对不截半）', () => {
    const text = toPlainText('😀'.repeat(400), 300)
    expect(Array.from(text).length).toBe(300)
    expect(text.endsWith('…')).toBe(true)
    expect(text.startsWith('😀')).toBe(true)
  })

  it('不超长时不截断、不加省略号', () => {
    expect(toPlainText('短文本', 300)).toBe('短文本')
  })

  it('空输入与纯文本输入行为正确', () => {
    expect(toPlainText('')).toBe('')
    expect(toPlainText('   ')).toBe('')
    expect(toPlainText('纯文本摘要')).toBe('纯文本摘要')
  })

  it('幂等：已清洗结果再清洗不变', () => {
    const once = toPlainText(ZHIYUAN_SAMPLE, 300)
    expect(toPlainText(once, 300)).toBe(once)
    const mixed = toPlainText('<p>正文</p><script>x</script>', 300)
    expect(toPlainText(mixed, 300)).toBe(mixed)
    // 共享样本也必须幕等（「先解实体 → 再解析」选这个顺序的全部理由）
    for (const sample of SHARED_SAMPLES) {
      expect(toPlainText(sample.expected)).toBe(sample.expected)
    }
  })
})

describe('跨实现共享样本（与后端 htmltext 逐字对齐）', () => {
  it.each(SHARED_SAMPLES)('$name', ({ input, expected }) => {
    expect(toPlainText(input)).toBe(expected)
  })
})

describe('htmlSanitize — sanitizeHtml', () => {
  it('移除全部 on* 事件属性', () => {
    const out = sanitizeHtml('<img src="https://example.com/a.png" onerror="alert(1)" onload="steal()">')
    expect(out).not.toContain('onerror')
    expect(out).not.toContain('onload')
    expect(out).toContain('<img')
  })

  it('移除 script / style 元素及其内容', () => {
    const out = sanitizeHtml('<script>alert(1)</script><style>.a{color:red}</style><p>正文</p>')
    expect(out).not.toContain('script')
    expect(out).not.toContain('alert')
    expect(out).not.toContain('style')
    expect(out).toContain('正文')
  })

  it('语义不展示（display:none / hidden 属性）连子树一起移除', () => {
    const out = sanitizeHtml('<div style="display: none;">藏一</div><span hidden>藏二</span><p>可见正文</p>')
    expect(out).not.toContain('藏一')
    expect(out).not.toContain('藏二')
    expect(out).toContain('可见正文')
  })

  it('视觉隐藏声明被剥离、元素与内容保留（剥后重新可见）', () => {
    const out = sanitizeHtml('<div style="opacity: 0">视觉藏</div><p>可见正文</p>')
    expect(out).toContain('视觉藏')
    expect(out).not.toContain('opacity')
    expect(out).toContain('可见正文')
  })

  it('visibility:hidden 与 display:none 分流：前者剥声明留内容、后者移除子树', () => {
    const out = sanitizeHtml('<div style="visibility: hidden;">视觉藏</div><div style="display: none;">语义藏</div><p>可见正文</p>')
    expect(out).toContain('视觉藏')
    expect(out).not.toContain('语义藏')
    expect(out).not.toContain('visibility: hidden')
    expect(out).toContain('可见正文')
  })

  it('视觉隐藏判定的边界写法（opacity:0 / 0.0 / font-size:0 / 0px 命中；0.5 / 14px 不命中）', () => {
    const hidden = sanitizeHtml('<p style="opacity: 0;">命中一</p><p style="opacity: 0.0;">命中二</p><p style="font-size: 0;">命中三</p><p style="font-size: 0px;">命中四</p>')
    expect(hidden).toContain('命中一')
    expect(hidden).toContain('命中二')
    expect(hidden).toContain('命中三')
    expect(hidden).toContain('命中四')
    expect(hidden).not.toContain('opacity')
    expect(hidden).not.toContain('font-size')

    const kept = sanitizeHtml('<p style="opacity: 0.5;">留存一</p><p style="font-size: 14px;">留存二</p>')
    expect(kept).toContain('opacity: 0.5')
    expect(kept).toContain('font-size: 14px')
  })

  it('智源真实样本清洗后正文保留、隐藏声明被剥离', () => {
    const out = sanitizeHtml(ZHIYUAN_SAMPLE)
    expect(out).not.toContain('opacity')
    expect(out).toContain('正文第一段')
    expect(out).toContain('克雷西 发自 凹非寺')
  })

  it('智源真实样本（含 display:none 藏块与 onerror）清洗后保留正文、剔除藏块与事件属性', () => {
    const out = sanitizeHtml(ZHIYUAN_SAMPLE_WITH_HIDDEN_BLOCK)
    expect(out).toContain('正文第一段')
    expect(out).not.toContain('opacity')
    expect(out).not.toContain('SEO关键词堆砌藏身处')
    expect(out).not.toContain('<script')
    expect(out).not.toContain('onerror')
  })

  it('保留可见结构与内联排版', () => {
    const out = sanitizeHtml('<p style="color: red;">正文</p><h3>标题</h3>')
    expect(out).toContain('<p')
    expect(out).toContain('color: red')
    expect(out).toContain('标题')
  })

  it('移除可执行嵌入元素（iframe/object/embed/form/applet/base/frame/frameset，连同子树）', () => {
    const out = sanitizeHtml([
      '<iframe srcdoc="<script>alert(1)</script>"></iframe>',
      '<object data="x.swf"><param name="a"></object>',
      '<embed src="x.swf">',
      '<form action="/x"><input name="a"><button>提交</button></form>',
      '<applet code="x"></applet>',
      '<base href="https://example.com/">',
      '<p>正文</p>',
    ].join(''))
    expect(out).not.toContain('<iframe')
    expect(out).not.toContain('<object')
    expect(out).not.toContain('<embed')
    expect(out).not.toContain('<form')
    expect(out).not.toContain('<applet')
    expect(out).not.toContain('<base')
    expect(out).not.toContain('<input')
    expect(out).not.toContain('提交')
    expect(out).toContain('正文')
  })

  it('移除 srcdoc 属性（内联文档是可执行载荷）', () => {
    const out = sanitizeHtml('<div srcdoc="<script>alert(1)</script>">正文</div>')
    expect(out).not.toContain('srcdoc')
    expect(out).not.toContain('<script')
    expect(out).toContain('正文')
  })

  it('移除危险 URL 方案（javascript: / vbscript: / data:）', () => {
    const out = sanitizeHtml([
      '<a href="javascript:alert(1)">甲</a>',
      '<a href="vbscript:msgbox(1)">乙</a>',
      '<img src="javascript:alert(1)">',
      '<svg><use xlink:href="javascript:alert(1)"></use></svg>',
    ].join(''))
    expect(out).not.toContain('javascript:')
    expect(out).not.toContain('vbscript:')
    expect(out).toContain('甲')
    expect(out).toContain('乙')
  })

  it('data:text/html 被移除、data:image/* 保留', () => {
    const out = sanitizeHtml('<a href="data:text/html;base64,PHNjcmlwdD4=">甲</a><img src="data:image/png;base64,iVBORw0KGgo=">')
    expect(out).not.toContain('data:text/html')
    expect(out).toContain('data:image/png')
  })

  it('危险 scheme 的变形写法（控制字符/大小写/实体）同样被移除', () => {
    const out = sanitizeHtml([
      '<a href="java&#9;script:alert(1)">甲</a>',
      '<a href="  JaVaScRiPt:alert(1)">乙</a>',
      '<a href="&#106;avascript:alert(1)">丙</a>',
    ].join(''))
    expect(out).not.toContain('script:')
    expect(out).not.toContain('alert(1)')
  })

  it('空输入返回空串', () => {
    expect(sanitizeHtml('')).toBe('')
  })

  it('幂等：二次清洗结果不变', () => {
    const once = sanitizeHtml('<div style="opacity:0">隐藏</div><p style="color:red">正文</p>')
    expect(sanitizeHtml(once)).toBe(once)
  })
})

describe('htmlSanitize — 非浏览器上下文降级', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('无 DOMParser 时不崩：纯文本降级为可读文本', () => {
    vi.stubGlobal('DOMParser', undefined)
    expect(toPlainText('<p>正文</p>')).toBe('正文')
  })

  it('无 DOMParser 时 sanitizeHtml 降级为转义文本，绝不回吐原始 HTML', () => {
    vi.stubGlobal('DOMParser', undefined)
    const out = sanitizeHtml('<p>正文</p><img src="x" onerror="alert(1)">')
    expect(out).not.toContain('<img')
    expect(out).not.toContain('onerror')
    expect(out).toContain('正文')
  })
})
