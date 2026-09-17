// @vitest-environment node
import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

/**
 * SPA 首屏加载模板 pre-FCP 背景色（fix-spa-nav-loading-ux specs「首屏 pre-FCP 背景色」）：
 * - <style> 含 html（editorial 底）与 html[data-theme="dark"]（dark 底）背景规则，
 *   FCP 前空窗不呈现浏览器默认白底
 * - 主题判定脚本先于 <style> 出现：data-theme 属性先于首帧设置，
 *   dark 分支背景规则才能在首绘前命中
 *
 * 只断言规则与顺序存在，不断言色值（色值与 main.css 令牌同源维护，避免脆断言）。
 */

// readFileSync 直接收 URL 对象（happy-dom 下 node:url 的 fileURLToPath 不可靠）
const template = readFileSync(
  new URL('./spa-loading-template.html', import.meta.url),
  'utf8',
)

describe('spa-loading-template.html - pre-FCP 背景色', () => {
  it('含 html 默认背景规则（editorial 底）', () => {
    expect(template).toMatch(/(?:^|\n)\s*html\s*\{[^}]*background\s*:/)
  })

  it('含 html[data-theme="dark"] 深色背景规则', () => {
    expect(template).toMatch(/html\[data-theme=["']dark["']\]\s*\{[^}]*background\s*:/)
  })

  it('主题判定脚本先于样式块出现（data-theme 先于首帧设置）', () => {
    // 正则匹配真实标签起始，避免被注释/说明文字里的字面量干扰
    const styleMatch = template.match(/[\r\n][^\S\r\n]*<style[\s>]|^[^\S\r\n]*<style[\s>]/)
    const styleIdx = styleMatch ? template.indexOf(styleMatch[0]) : -1
    const themeScriptIdx = template.indexOf("localStorage.getItem('syntopica-theme')")
    expect(styleIdx).toBeGreaterThan(-1)
    expect(themeScriptIdx).toBeGreaterThan(-1)
    expect(themeScriptIdx).toBeLessThan(styleIdx)
  })
})
