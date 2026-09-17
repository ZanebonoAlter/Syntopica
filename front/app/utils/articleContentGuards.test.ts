import { describe, expect, it } from 'vitest'

import { shouldShowArticleDescription } from './articleContentGuards'

describe('shouldShowArticleDescription', () => {
  it('shows description when article body is empty', () => {
    expect(shouldShowArticleDescription('<p>Short summary</p>', '')).toBe(true)
  })

  it('hides description when body text is identical', () => {
    expect(shouldShowArticleDescription('<p>Same text</p>', '<div>Same text</div>')).toBe(false)
  })

  it('hides description when body already contains the full description', () => {
    expect(
      shouldShowArticleDescription(
        '<p>This is a longer article summary that should not repeat.</p>',
        '<article><p>This is a longer article summary that should not repeat.</p><p>More body text follows here.</p></article>',
      ),
    ).toBe(false)
  })

  it('keeps description when it adds distinct context', () => {
    expect(
      shouldShowArticleDescription(
        '<p>Editor note: this post was updated later.</p>',
        '<article><p>The body explains a different thing entirely.</p></article>',
      ),
    ).toBe(true)
  })

  // ===== redesign-reading-pane task 4.2：guard 收紧边界用例 =====

  it('hides empty description', () => {
    expect(shouldShowArticleDescription('', '<p>body</p>')).toBe(false)
    expect(shouldShowArticleDescription(null, '<p>body</p>')).toBe(false)
  })

  it('hides whitespace-only description (full-width space + tab)', () => {
    expect(shouldShowArticleDescription('　	  ', '<p>body</p>')).toBe(false)
  })

  it('hides pure-image description (markdown and HTML)', () => {
    expect(
      shouldShowArticleDescription('![这是一张很长的图片描述文字](https://example.com/cover.png)', '<p>body</p>'),
    ).toBe(false)
    expect(
      shouldShowArticleDescription('<figure><img src="https://example.com/cover.png" alt="配图"></figure>', '<p>body</p>'),
    ).toBe(false)
  })

  it('hides near-empty or symbol-only description (normalized length < 4)', () => {
    expect(shouldShowArticleDescription('···', '<p>body</p>')).toBe(false)
    expect(shouldShowArticleDescription('<p>——</p>', '<p>body</p>')).toBe(false)
  })

  it('keeps a short but substantive description under duplication checks', () => {
    expect(shouldShowArticleDescription('<p>这是一段简短但不重复的导语。</p>', '<p>body</p>')).toBe(true)
  })
})
