import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { Icon } from '@iconify/vue'
import FeedIcon from './FeedIcon.vue'

// FeedIcon's load-bearing behavior is graceful degradation: when an icon URL
// fails to load (common for favicons behind the GFW or stale aggregator URLs),
// it must fall back to the Iconify placeholder (rendered as <svg>) rather than
// leaving a blank gap (the old display:none behavior).
//
// Note: @iconify/vue renders <Icon> as an inline <svg>, not as text containing
// the icon name. So we assert on the <svg>/<img> element presence instead.
describe('FeedIcon', () => {
  it('renders an <img> when icon is an http URL', () => {
    const wrapper = mount(FeedIcon, {
      props: { icon: 'https://example.com/favicon.ico' },
    })
    expect(wrapper.find('img').exists()).toBe(true)
    expect(wrapper.find('img').attributes('src')).toBe('https://example.com/favicon.ico')
  })

  it('renders an <img> with the backend origin for a relative path', () => {
    // Backend-served local icon: /icons/feeds/<id>.<ext> (default apiBase is
    // absolute http://localhost:5100/api, so origin is the backend).
    const wrapper = mount(FeedIcon, {
      props: { icon: '/icons/feeds/42.png' },
    })
    expect(wrapper.find('img').exists()).toBe(true)
    expect(wrapper.find('img').attributes('src')).toBe('http://localhost:5100/icons/feeds/42.png')
  })

  it('falls back to the Iconify placeholder when a local path image fails to load', async () => {
    const wrapper = mount(FeedIcon, {
      props: { icon: '/icons/feeds/42.png' },
    })
    expect(wrapper.find('img').exists()).toBe(true)

    await wrapper.find('img').trigger('error')

    expect(wrapper.find('img').exists()).toBe(false)
    expect(wrapper.find('svg').exists()).toBe(true)
  })

  // Regression: the placeholder used to receive the raw icon value, so a local
  // path became an unresolvable iconify name and <Icon> rendered an empty <svg>
  // — a blank gap, exactly what the degradation is supposed to prevent.
  it('passes mdi:rss (not the image path) to the placeholder when a local path fails', async () => {
    const wrapper = mount(FeedIcon, {
      props: { icon: '/icons/feeds/2.ico' },
    })
    await wrapper.find('img').trigger('error')

    expect(wrapper.findComponent(Icon).props('icon')).toBe('mdi:rss')
  })

  it('passes mdi:rss (not the remote URL) to the placeholder when a remote URL fails', async () => {
    const wrapper = mount(FeedIcon, {
      props: { icon: 'https://example.com/broken.ico' },
    })
    await wrapper.find('img').trigger('error')

    expect(wrapper.findComponent(Icon).props('icon')).toBe('mdi:rss')
  })

  it('keeps a real iconify name when one is supplied', () => {
    const wrapper = mount(FeedIcon, {
      props: { icon: 'mdi:github' },
    })

    expect(wrapper.findComponent(Icon).props('icon')).toBe('mdi:github')
  })

  it('keeps a hyphenated third-party iconify name', () => {
    const wrapper = mount(FeedIcon, {
      props: { icon: 'simple-icons:nuxtdotjs' },
    })

    expect(wrapper.findComponent(Icon).props('icon')).toBe('simple-icons:nuxtdotjs')
  })

  it('renders mdi:rss for legacy / non-name icon values instead of a blank gap', () => {
    for (const icon of ['rss', 'icons/feeds/2.ico', 'data:image/png;base64,AAAA']) {
      const wrapper = mount(FeedIcon, { props: { icon } })
      expect(wrapper.findComponent(Icon).props('icon')).toBe('mdi:rss')
      expect(wrapper.find('img').exists()).toBe(false)
    }
  })

  it('falls back to the Iconify placeholder (svg) when the image fails to load', async () => {
    const wrapper = mount(FeedIcon, {
      props: { icon: 'https://example.com/favicon.ico' },
    })
    // Initially renders the <img>
    expect(wrapper.find('img').exists()).toBe(true)

    // Simulate image load failure
    await wrapper.find('img').trigger('error')

    // <img> is replaced by the Iconify <svg> placeholder (no blank gap)
    expect(wrapper.find('img').exists()).toBe(false)
    expect(wrapper.find('svg').exists()).toBe(true)
  })

  it('recovers when the icon prop changes after a failure', async () => {
    const wrapper = mount(FeedIcon, {
      props: { icon: 'https://example.com/broken.ico' },
    })
    await wrapper.find('img').trigger('error')
    expect(wrapper.find('img').exists()).toBe(false)

    // A new valid URL should render the <img> again (failure flag reset)
    await wrapper.setProps({ icon: 'https://example.com/good.ico' })
    expect(wrapper.find('img').exists()).toBe(true)
    expect(wrapper.find('img').attributes('src')).toBe('https://example.com/good.ico')
  })

  it('renders the Iconify placeholder (svg) directly when icon is empty', () => {
    const wrapper = mount(FeedIcon, {
      props: { icon: '' },
    })
    expect(wrapper.find('img').exists()).toBe(false)
    expect(wrapper.find('svg').exists()).toBe(true)
  })

  it('renders an iconify id as svg when icon is not a URL', () => {
    const wrapper = mount(FeedIcon, {
      props: { icon: 'mdi:github' },
    })
    expect(wrapper.find('img').exists()).toBe(false)
    expect(wrapper.find('svg').exists()).toBe(true)
  })
})
