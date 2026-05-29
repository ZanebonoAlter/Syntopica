import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import TagHierarchy from './TagHierarchy.vue'
import TagHierarchyRow from './TagHierarchyRow.vue'
import type { TagHierarchyNode } from '~/types/topicTag'

const fetchHierarchy = vi.fn()

vi.mock('~/api/abstractTags', () => ({
  useAbstractTagApi: () => ({
    fetchHierarchy,
    updateAbstractName: vi.fn(),
    detachChild: vi.fn(),
    reassignTag: vi.fn(),
  }),
}))

function createNode(overrides: Partial<TagHierarchyNode> = {}): TagHierarchyNode {
  return {
    id: 1,
    label: 'AI Agent',
    slug: 'ai-agent',
    category: 'keyword',
    icon: 'mdi:tag',
    feedCount: 3,
    articleCount: 5,
    isActive: true,
    children: [],
    ...overrides,
  }
}

describe('TagHierarchyRow', () => {
  it('emits select when clicking a tag label', async () => {
    const node = createNode()
    const wrapper = mount(TagHierarchyRow, {
      props: {
        node,
        depth: 0,
        editingId: null,
        saving: false,
      },
    })

    await wrapper.find('.th-label').trigger('click')

    expect(wrapper.emitted('select')).toEqual([[node]])
  })
})

describe('TagHierarchy', () => {
  it('re-emits selected tag from hierarchy rows', async () => {
    fetchHierarchy.mockResolvedValue({
      success: true,
      data: {
        nodes: [createNode()],
        total: 1,
      },
    })

    const TagHierarchyRowStub = {
      name: 'TagHierarchyRow',
      template: '<button @click="$emit(\'select\', node)">Select</button>',
      props: ['node'],
      emits: ['select'],
    }

    const wrapper = mount(TagHierarchy, {
      global: {
        stubs: {
          TagHierarchyRow: TagHierarchyRowStub,
          Teleport: true,
        },
      },
    })

    await Promise.resolve()
    await Promise.resolve()

    wrapper.getComponent({ name: 'TagHierarchyRow' }).vm.$emit('select', createNode())

    expect(wrapper.emitted('select-tag')).toEqual([['ai-agent', 'keyword']])
  })
})
