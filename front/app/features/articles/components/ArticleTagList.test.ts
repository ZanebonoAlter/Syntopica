import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ArticleTagList from './ArticleTagList.vue'

describe('ArticleTagList', () => {
  it('renders aggregated article tags and highlights matched slugs', () => {
    const wrapper = mount(ArticleTagList, {
      props: {
        tags: [
          { slug: 'ai-agent', label: 'AI Agent', category: 'keyword', articleCount: 2 },
          { slug: 'sam-altman', label: 'Sam Altman', category: 'person', articleCount: 1 },
        ],
        highlightedSlugs: ['ai-agent'],
      },
    })

    expect(wrapper.text()).toContain('AI Agent')
    expect(wrapper.text()).toContain('Sam Altman')
    expect(wrapper.find('[data-tag-slug="ai-agent"]').classes()).toContain('article-tag--highlighted')
  })

  it('truncates long tag lists in compact mode', () => {
    const wrapper = mount(ArticleTagList, {
      props: {
        compact: true,
        maxVisible: 2,
        tags: [
          { slug: 'ai-agent', label: 'AI Agent', category: 'keyword', articleCount: 2 },
          { slug: 'openai', label: 'OpenAI', category: 'keyword', articleCount: 2 },
          { slug: 'gpt-5', label: 'GPT-5', category: 'keyword', articleCount: 1 },
        ],
      },
    })

    expect(wrapper.text()).toContain('AI Agent')
    expect(wrapper.text()).toContain('OpenAI')
    expect(wrapper.text()).toContain('+1')
  })
})
