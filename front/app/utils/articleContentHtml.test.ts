import { describe, expect, it } from 'vitest'
import { buildOriginalContentHtml } from '~/utils/articleContentHtml'

/**
 * 原文源加工链的行为契约（fix-reader-lede-blank-space spec
 * article-description-sanitization「正文源切换后正文可见」）。
 *
 * 这条链的输入是订阅源的整篇 HTML，未经信任：智源社区的公众号文章整篇被
 * `opacity: 0` 的根节点包裹，实测在修复前产生 8046 px 空白；`<img onerror>`
 * 一类事件属性会真实执行。断言打在输出 HTML 上，而不是源码字符串上。
 */
describe('buildOriginalContentHtml — 原文源渲染前加工', () => {
  it('视觉隐藏声明被剥离、正文重新可见（智源形态）', () => {
    const html = buildOriginalContentHtml(
      '<div class="rich_media_content" style="visibility: visible; opacity: 0; "><h5>克雷西 发自 凹非寺</h5><p>正文第一段</p></div>',
    )
    expect(html).toContain('正文第一段')
    expect(html).not.toContain('opacity')
  })

  it('语义不展示的块连同子树被移除', () => {
    const html = buildOriginalContentHtml('<p style="display: none">SEO 藏块</p><p>可见正文</p>')
    expect(html).not.toContain('SEO 藏块')
    expect(html).toContain('可见正文')
  })

  it('事件属性与脚本不出现在输出里', () => {
    const html = buildOriginalContentHtml(
      '<script>alert(1)</script><p onclick="alert(1)">正文</p><img src="https://cdn.example.com/a.png" onerror="alert(2)">',
    )
    expect(html).not.toContain('<script')
    expect(html).not.toMatch(/onclick|onerror/i)
    expect(html).toContain('正文')
  })

  it('可执行嵌入与危险 URL 方案被剔除', () => {
    const html = buildOriginalContentHtml(
      '<iframe srcdoc="<script>x</script>"></iframe><form action="/x"></form><a href="javascript:alert(1)">链接</a><p>正文</p>',
    )
    expect(html).not.toContain('<iframe')
    expect(html).not.toContain('<form')
    expect(html).not.toContain('javascript:')
    expect(html).toContain('正文')
  })

  it('清洗排在外链图片代理之前（顺序契约）', () => {
    const html = buildOriginalContentHtml('<img src="https://cdn.example.com/a.png" onerror="alert(1)">')
    // 图片已被改写为同源代理，且原来挂在它上面的 onerror 没有跟过来
    expect(html).toContain('/api/image-proxy?url=')
    expect(html).not.toContain('onerror')
  })

  it('Base64 占位图被剔除', () => {
    const html = buildOriginalContentHtml('<p>正文</p><img src="Base64-Image-Removed">')
    expect(html).toContain('正文')
    expect(html).not.toContain('Base64-Image-Removed')
  })

  it('空输入返回空串（不产生空容器）', () => {
    expect(buildOriginalContentHtml('')).toBe('')
  })
})
