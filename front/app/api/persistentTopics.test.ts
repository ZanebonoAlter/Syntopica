import { beforeEach, describe, expect, it, vi } from 'vitest'

const { postMock, getMock } = vi.hoisted(() => ({
  postMock: vi.fn(),
  getMock: vi.fn(),
}))

vi.mock('./client', () => ({
  apiClient: {
    get: getMock,
    post: postMock,
  },
}))

import { usePersistentTopicsApi } from './persistentTopics'

describe('usePersistentTopicsApi — 手动建泳道编排态（切片③）', () => {
  beforeEach(() => {
    postMock.mockReset()
    getMock.mockReset()
  })

  describe('getComposeCandidates', () => {
    it('GETs compose-candidates with days and normalizes snake_case + string ids', async () => {
      getMock.mockResolvedValue({
        success: true,
        data: {
          sections: [
            {
              id: 101,
              period_date: '2026-06-18T00:00:00Z',
              cluster_label: '霍尔木兹油轮遇袭',
              embedding: [0.12, 0.34, 0.56],
              persistent_topic_id: 7,
              topic_match_confidence: 'anchor_hit',
              persistent_topic: { id: 7, label: '中东局势', status: 'active', color: '#b44f45' },
            },
            {
              id: 102,
              period_date: '2026-06-19T00:00:00Z',
              cluster_label: '无归属 section',
              embedding: [0.01, 0.02],
            },
          ],
          match_threshold: 0.3,
        },
      })

      const api = usePersistentTopicsApi()
      const res = await api.getComposeCandidates(1974, 14)

      expect(getMock).toHaveBeenCalledWith('/semantic-boards/1974/persistent-topics/compose-candidates?days=14')
      expect(res.success).toBe(true)
      const data = res.data!
      expect(data.matchThreshold).toBe(0.3)
      expect(data.sections).toHaveLength(2)
      // snake_case → camelCase
      expect(data.sections[0]).toMatchObject({
        id: '101',
        periodDate: '2026-06-18T00:00:00Z',
        clusterLabel: '霍尔木兹油轮遇袭',
        persistentTopicId: '7',
        topicMatchConfidence: 'anchor_hit',
      })
      // numeric id → string
      expect(typeof data.sections[0]!.id).toBe('string')
      expect(data.sections[0]!.persistentTopic).toEqual({ id: '7', label: '中东局势', status: 'active' })
      // embedding 原样透传（number[]）
      expect(data.sections[0]!.embedding).toEqual([0.12, 0.34, 0.56])
      // 无归属 section 不带 persistent* 字段
      expect(data.sections[1]!.persistentTopicId).toBeUndefined()
      expect(data.sections[1]!.persistentTopic).toBeUndefined()
    })

    it('tolerates empty sections array', async () => {
      getMock.mockResolvedValue({ success: true, data: { sections: [], match_threshold: 0.3 } })
      const api = usePersistentTopicsApi()
      const res = await api.getComposeCandidates(1, 7)
      expect(res.success).toBe(true)
      expect(res.data!.sections).toEqual([])
    })

    it('propagates failure without throwing', async () => {
      getMock.mockResolvedValue({ success: false, error: 'boom' })
      const api = usePersistentTopicsApi()
      const res = await api.getComposeCandidates(1, 14)
      expect(res.success).toBe(false)
      expect(res.error).toBe('boom')
    })
  })

  describe('getComposeCandidates — 编排窗口与候选链（12.2.1）', () => {
    it('自选起止：请求带 from/to（成对给出），并归一化 relations / candidate_chains / window', async () => {
      getMock.mockResolvedValue({
        success: true,
        data: {
          sections: [],
          relations: [
            { from_id: 7345, to_id: 7409, distance: 0.12, relation_type: 'identity' },
            { from_id: 7409, to_id: 7514, distance: 0.2, relation_type: 'similarity' },
          ],
          candidate_chains: [
            {
              key: '1984:7345',
              topic_id: 1984,
              topic_label: '以色列议会选举',
              topic_status: 'candidate',
              section_ids: [7345, 7409, 7514],
              first_date: '2026-10-27',
              last_date: '2026-10-30',
              span_days: 3,
            },
          ],
          match_threshold: 0.3,
          window: { from: '2026-10-27', to: '2026-10-30', days: 4, latest_completed_date: '2026-10-30' },
        },
      })

      const api = usePersistentTopicsApi()
      const res = await api.getComposeCandidates(1974, { from: '2026-10-27', to: '2026-10-30' })

      expect(getMock).toHaveBeenCalledWith(
        '/semantic-boards/1974/persistent-topics/compose-candidates?from=2026-10-27&to=2026-10-30',
      )
      const data = res.data!
      // relations：id 字符串化 + camelCase
      expect(data.relations).toEqual([
        { fromId: '7345', toId: '7409', distance: 0.12, relationType: 'identity' },
        { fromId: '7409', toId: '7514', distance: 0.2, relationType: 'similarity' },
      ])
      // candidate_chains：服务端唯一口径，前端只渲染
      expect(data.candidateChains).toEqual([
        {
          key: '1984:7345',
          topicId: '1984',
          topicLabel: '以色列议会选举',
          topicStatus: 'candidate',
          sectionIds: ['7345', '7409', '7514'],
          firstDate: '2026-10-27',
          lastDate: '2026-10-30',
          spanDays: 3,
        },
      ])
      expect(data.window).toEqual({ from: '2026-10-27', to: '2026-10-30', days: 4, latestCompletedDate: '2026-10-30' })
    })

    it('快捷档：对象形态的 days 与数字形态等价', async () => {
      getMock.mockResolvedValue({ success: true, data: { sections: [], match_threshold: 0.3 } })
      const api = usePersistentTopicsApi()
      await api.getComposeCandidates(1974, { days: 30 })
      expect(getMock).toHaveBeenLastCalledWith('/semantic-boards/1974/persistent-topics/compose-candidates?days=30')
    })

    it('后端未升级（无 relations/chains/window）时降级为空，不炸', async () => {
      getMock.mockResolvedValue({ success: true, data: { sections: [], match_threshold: 0.3 } })
      const api = usePersistentTopicsApi()
      const res = await api.getComposeCandidates(1974, 30)
      expect(res.success).toBe(true)
      expect(res.data!.relations).toEqual([])
      expect(res.data!.candidateChains).toEqual([])
      expect(res.data!.window).toBeNull()
    })
  })

  describe('getComposeCandidates — clueCount 归一（unify-lane-workbench 2.2 / D2）', () => {
    it('0 与正数保留；缺失 / null / 负数 / 非有限数 → null（未知，不冒充 0）', async () => {
      getMock.mockResolvedValue({
        success: true,
        data: {
          sections: [
            { id: 1, report_id: 11, period_date: '2026-11-01', cluster_label: 'a', embedding: [0.1], clue_count: 0 },
            { id: 2, report_id: 11, period_date: '2026-11-01', cluster_label: 'b', embedding: [0.1], clue_count: 3 },
            { id: 3, report_id: 11, period_date: '2026-11-01', cluster_label: 'c', embedding: [0.1] },
            { id: 4, report_id: 11, period_date: '2026-11-01', cluster_label: 'd', embedding: [0.1], clue_count: null },
            { id: 5, report_id: 11, period_date: '2026-11-01', cluster_label: 'e', embedding: [0.1], clue_count: -1 },
            { id: 6, report_id: 11, period_date: '2026-11-01', cluster_label: 'f', embedding: [0.1], clue_count: Number.NaN },
            { id: 7, report_id: 11, period_date: '2026-11-01', cluster_label: 'g', embedding: [0.1], clue_count: '4' },
          ],
          match_threshold: 0.3,
        },
      })
      const api = usePersistentTopicsApi()
      const res = await api.getComposeCandidates(1974, 30)
      expect(res.success).toBe(true)
      const sections = res.data!.sections
      expect(sections[0]!.clueCount).toBe(0) // 0 是合法已知值，不吞
      expect(sections[1]!.clueCount).toBe(3)
      expect(sections[2]!.clueCount).toBeNull() // 旧后端缺字段 → 未知
      expect(sections[3]!.clueCount).toBeNull()
      expect(sections[4]!.clueCount).toBeNull() // 负数 = 非法 → 未知
      expect(sections[5]!.clueCount).toBeNull() // NaN → 未知
      expect(sections[6]!.clueCount).toBeNull() // 字符串数字不是 number → 未知
    })

    it('全综合稿 section 的 0 仍序列化为 0（不缺字段）', async () => {
      getMock.mockResolvedValue({
        success: true,
        data: {
          sections: [
            { id: 9, report_id: 12, period_date: '2026-11-02', cluster_label: '纯综合稿', embedding: [0.2], clue_count: 0 },
          ],
          match_threshold: 0.3,
        },
      })
      const api = usePersistentTopicsApi()
      const res = await api.getComposeCandidates(1974, 7)
      expect(res.data!.sections[0]!.clueCount).toBe(0)
    })
  })

  describe('getComposeCandidates — confirmed_key_events 归一（add-lane-compose-topic-event-tags）', () => {
    it('原样映射为 camelCase；空数组/缺字段 → []；thread_id / confirmed_at 为 null 时省略', async () => {
      getMock.mockResolvedValue({
        success: true,
        data: {
          sections: [
            {
              id: 1,
              report_id: 11,
              period_date: '2026-11-01',
              cluster_label: 'a',
              embedding: [0.1],
              confirmed_key_events: [
                { marker_id: 7, thread_id: 91, title: '降息预期升温', confirmed_at: '2026-11-20T10:00:00Z' },
                { marker_id: 8, thread_id: null, title: '非农超预期', confirmed_at: null },
              ],
            },
            { id: 2, report_id: 11, period_date: '2026-11-01', cluster_label: 'b', embedding: [0.1], confirmed_key_events: [] },
            { id: 3, report_id: 11, period_date: '2026-11-01', cluster_label: 'c', embedding: [0.1] },
          ],
          match_threshold: 0.3,
        },
      })
      const api = usePersistentTopicsApi()
      const res = await api.getComposeCandidates(1974, 14)
      const sections = res.data!.sections
      expect(sections[0]!.confirmedKeyEvents).toEqual([
        { markerId: '7', threadId: '91', title: '降息预期升温', confirmedAt: '2026-11-20T10:00:00Z' },
        { markerId: '8', title: '非农超预期' },
      ])
      expect(sections[1]!.confirmedKeyEvents).toEqual([])
      expect(sections[2]!.confirmedKeyEvents).toEqual([]) // 旧后端缺字段 → 无标记，不炸
    })
  })

  describe('getComposeCandidates — early_members[].embedding 透传（unify-lane-workbench R-1 / D6）', () => {
    it('有向量时原样透传（更早成员要撑起推荐质心）', async () => {
      getMock.mockResolvedValue({
        success: true,
        data: {
          sections: [],
          early_members: [
            {
              id: 801,
              report_id: 71,
              period_date: '2026-10-01T00:00:00Z',
              cluster_label: '更早成员（有向量）',
              embedding: [0.9, 0.1],
              persistent_topic_id: 103,
              topic_match_distance: 0.12,
            },
          ],
          match_threshold: 0.3,
        },
      })

      const api = usePersistentTopicsApi()
      const res = await api.getComposeCandidates(1974, { days: 7, allMembersTopicId: 103 })

      expect(res.success).toBe(true)
      expect(res.data!.earlyMembers).toEqual([
        {
          id: '801',
          reportId: '71',
          periodDate: '2026-10-01T00:00:00Z',
          clusterLabel: '更早成员（有向量）',
          embedding: [0.9, 0.1],
          persistentTopicId: '103',
          topicMatchDistance: 0.12,
        },
      ])
    })

    it('缺省 / 空数组 → undefined（不塞 []，避免把「无向量」伪装成零向量）', async () => {
      getMock.mockResolvedValue({
        success: true,
        data: {
          sections: [],
          early_members: [
            { id: 802, period_date: '2026-10-02T00:00:00Z', cluster_label: '缺向量' },
            { id: 803, period_date: '2026-10-03T00:00:00Z', cluster_label: '空向量', embedding: [] },
          ],
          match_threshold: 0.3,
        },
      })

      const api = usePersistentTopicsApi()
      const res = await api.getComposeCandidates(1974, { days: 7, allMembersTopicId: 103 })

      const earlyMembers = res.data!.earlyMembers!
      expect(earlyMembers).toHaveLength(2)
      expect(earlyMembers[0]!.embedding).toBeUndefined()
      expect(earlyMembers[1]!.embedding).toBeUndefined()
      expect('embedding' in earlyMembers[0]!).toBe(false)
    })
  })

  describe('createManualLane', () => {
    it('POSTs {label, section_ids:number[]} and normalizes the topic + skipped ids', async () => {
      postMock.mockResolvedValue({
        success: true,
        data: {
          topic: { id: 20, label: '美伊博弈', status: 'active', source: 'manual' },
          skipped: [999],
        },
        message: '1 条 section 因无向量被跳过',
      })

      const api = usePersistentTopicsApi()
      const res = await api.createManualLane(1974, '美伊博弈', ['101', '102', '999'])

      // string ids → numbers in request body (backend receives []uint)
      expect(postMock).toHaveBeenCalledWith(
        '/semantic-boards/1974/persistent-topics/manual',
        { label: '美伊博弈', section_ids: [101, 102, 999] },
      )
      expect(res.success).toBe(true)
      expect(res.data!.topic).toEqual({ id: '20', label: '美伊博弈', status: 'active', source: 'manual' })
      expect(res.data!.skipped).toEqual(['999'])
      expect(res.message).toContain('跳过')
    })

    it('defaults skipped to [] when absent', async () => {
      postMock.mockResolvedValue({
        success: true,
        data: { topic: { id: 21, label: 'X', status: 'active', source: 'manual' } },
      })
      const api = usePersistentTopicsApi()
      const res = await api.createManualLane(1, 'X', ['1'])
      expect(res.success).toBe(true)
      expect(res.data!.skipped).toEqual([])
    })

    it('propagates backend error without throwing', async () => {
      postMock.mockResolvedValue({ success: false, error: 'no usable vector' })
      const api = usePersistentTopicsApi()
      const res = await api.createManualLane(1, 'X', ['1'])
      expect(res.success).toBe(false)
      expect(res.error).toBe('no usable vector')
    })
  })

  describe('embedQuery', () => {
    it('POSTs {query} and returns embedding number[]', async () => {
      postMock.mockResolvedValue({
        success: true,
        data: { embedding: [0.1, 0.2, 0.3] },
      })
      const api = usePersistentTopicsApi()
      const res = await api.embedQuery(1974, '半导体出口管制')
      expect(postMock).toHaveBeenCalledWith(
        '/semantic-boards/1974/persistent-topics/embed-query',
        { query: '半导体出口管制' },
      )
      expect(res.success).toBe(true)
      expect(res.data!.embedding).toEqual([0.1, 0.2, 0.3])
    })

    it('tolerates missing embedding (defaults to [])', async () => {
      postMock.mockResolvedValue({ success: true, data: {} })
      const api = usePersistentTopicsApi()
      const res = await api.embedQuery(1, 'x')
      expect(res.success).toBe(true)
      expect(res.data!.embedding).toEqual([])
    })

    it('propagates failure without throwing', async () => {
      postMock.mockResolvedValue({ success: false, error: 'model down' })
      const api = usePersistentTopicsApi()
      const res = await api.embedQuery(1, 'x')
      expect(res.success).toBe(false)
      expect(res.error).toBe('model down')
    })
  })
})
