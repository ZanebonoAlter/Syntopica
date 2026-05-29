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
})
