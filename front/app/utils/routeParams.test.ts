import { describe, expect, it } from 'vitest'
import {
  buildRouteDocUrl,
  buildRouteParamSpecs,
  buildRSSHubFeedUrl,
  DEFAULT_RSSHUB_BASE_URL,
  DEFAULT_RSSHUB_DOC_BASE,
  parseParameterDescriptions,
  parseParameterOptions,
  parsePathParams,
  stripOptionalParams,
} from './routeParams'

describe('parsePathParams', () => {
  it('returns empty for path without params', () => {
    expect(parsePathParams('/81rc/news')).toEqual([])
  })

  it('marks bare :param as required', () => {
    expect(parsePathParams('/bilibili/user/video/:uid')).toEqual([
      { name: 'uid', required: true },
    ])
  })

  it('marks :param? as optional', () => {
    expect(parsePathParams('/81rc/:category?')).toEqual([
      { name: 'category', required: false },
    ])
  })

  it('strips regex constraint and treats :param{.+}? as optional', () => {
    expect(parsePathParams('/81rc/:category{.+}?')).toEqual([
      { name: 'category', required: false },
    ])
  })

  it('strips regex constraint on required param', () => {
    expect(parsePathParams('/weibo/user/:uid{[0-9]+}')).toEqual([
      { name: 'uid', required: true },
    ])
  })

  it('parses multiple params preserving order', () => {
    expect(parsePathParams('/zhihu/question/:questionId/:sort?')).toEqual([
      { name: 'questionId', required: true },
      { name: 'sort', required: false },
    ])
  })
})

describe('parseParameterDescriptions', () => {
  it('parses object with plain string descriptions', () => {
    expect(parseParameterDescriptions('{"category":"分类，默认为全部"}')).toEqual({
      category: '分类，默认为全部',
    })
  })

  it('parses object with nested description field', () => {
    expect(parseParameterDescriptions('{"uid":{"description":"用户 id"}}')).toEqual({
      uid: '用户 id',
    })
  })

  it('parses string array form', () => {
    expect(parseParameterDescriptions('["uid","sort"]')).toEqual({ uid: '', sort: '' })
  })

  it('parses array of objects form', () => {
    expect(parseParameterDescriptions('[{"name":"uid","description":"用户 id"}]')).toEqual({
      uid: '用户 id',
    })
  })

  it('returns empty object for invalid JSON', () => {
    expect(parseParameterDescriptions('not-json')).toEqual({})
  })

  it('returns empty object for empty input', () => {
    expect(parseParameterDescriptions('')).toEqual({})
    expect(parseParameterDescriptions('{}')).toEqual({})
  })
})

describe('parseParameterOptions', () => {
  it('extracts options from object form with description', () => {
    const raw = JSON.stringify({
      name: { options: [{ label: '早报', value: '早报' }, { label: '评测', value: '评测' }], description: '分类名称' },
    })
    expect(parseParameterOptions(raw)).toEqual({
      name: [{ value: '早报', label: '早报' }, { value: '评测', label: '评测' }],
    })
  })

  it('uses value as label when label missing', () => {
    expect(parseParameterOptions('{"cat":{"options":[{"value":"a"},{"value":"b"}]}}')).toEqual({
      cat: [{ value: 'a', label: 'a' }, { value: 'b', label: 'b' }],
    })
  })

  it('extracts options from array-of-objects form', () => {
    expect(parseParameterOptions('[{"name":"cat","options":[{"value":"x","label":"X"}]}]')).toEqual({
      cat: [{ value: 'x', label: 'X' }],
    })
  })

  it('returns empty when no options field', () => {
    expect(parseParameterOptions('{"category":"分类，见下表"}')).toEqual({})
    expect(parseParameterOptions('{"uid":{"description":"用户 id"}}')).toEqual({})
  })

  it('returns empty for invalid JSON or empty input', () => {
    expect(parseParameterOptions('not-json')).toEqual({})
    expect(parseParameterOptions('')).toEqual({})
    expect(parseParameterOptions('{}')).toEqual({})
  })

  it('skips non-array options and items without value', () => {
    expect(parseParameterOptions('{"cat":{"options":"not-array"}}')).toEqual({})
    expect(parseParameterOptions('{"cat":{"options":[{}, { "value": "" }]}')).toEqual({})
  })
})

describe('buildRouteParamSpecs', () => {
  it('merges path params with descriptions, required first', () => {
    const specs = buildRouteParamSpecs(
      '/zhihu/question/:questionId/:sort?',
      '{"questionId":"问题 id","sort":"排序方式"}',
    )
    expect(specs).toEqual([
      { name: 'questionId', required: true, description: '问题 id' },
      { name: 'sort', required: false, description: '排序方式' },
    ])
  })

  it('falls back to empty description when catalog has no entry', () => {
    const specs = buildRouteParamSpecs('/bilibili/user/video/:uid', '{}')
    expect(specs).toEqual([{ name: 'uid', required: true, description: '' }])
  })

  it('injects options and docUrl when paramOptions / docUrl provided', () => {
    const specs = buildRouteParamSpecs(
      '/realtime/:category?',
      '{"category":"類別，見下表"}',
      {
        category: [
          { value: 'world', label: '国际', source: 'manual' },
          { value: 'cn', label: '国内', source: 'manual' },
        ],
      },
      'https://docs.rsshub.app/routes/81rc#realtime',
    )
    expect(specs).toEqual([
      {
        name: 'category',
        required: false,
        description: '類別，見下表',
        options: [
          { value: 'world', label: '国际' },
          { value: 'cn', label: '国内' },
        ],
        docUrl: 'https://docs.rsshub.app/routes/81rc#realtime',
      },
    ])
  })

  it('ignores empty option arrays and unknown param names', () => {
    const specs = buildRouteParamSpecs(
      '/bilibili/user/video/:uid',
      '{}',
      { uid: [], other: [{ value: 'x', label: 'X' }] },
    )
    expect(specs).toEqual([{ name: 'uid', required: true, description: '' }])
  })

  it('injects catalog options when no dictionary provided', () => {
    const raw = JSON.stringify({
      name: { options: [{ label: '早报', value: '早报' }], description: '分类名称' },
    })
    const specs = buildRouteParamSpecs('/ifanr/category/:name', raw)
    expect(specs).toEqual([
      { name: 'name', required: true, description: '分类名称', options: [{ value: '早报', label: '早报' }] },
    ])
  })

  it('dictionary options take precedence over catalog options', () => {
    const raw = JSON.stringify({
      name: { options: [{ label: '目录项', value: 'catalog' }], description: '分类' },
    })
    const specs = buildRouteParamSpecs('/x/:name', raw, {
      name: [{ value: 'dict', label: '字典项', source: 'manual' }],
    })
    expect(specs[0]!.options).toEqual([{ value: 'dict', label: '字典项' }])
  })

  it('catalog options fall back when dictionary empty for the param', () => {
    const raw = JSON.stringify({ name: { options: [{ value: 'a', label: 'A' }] } })
    const specs = buildRouteParamSpecs('/x/:name', raw, { name: [] })
    expect(specs[0]!.options).toEqual([{ value: 'a', label: 'A' }])
  })

  it('stays backward compatible when new args omitted', () => {
    const specs = buildRouteParamSpecs('/81rc/:category?', '{"category":"分类"}')
    expect(specs).toEqual([{ name: 'category', required: false, description: '分类' }])
    expect(specs[0]).not.toHaveProperty('options')
    expect(specs[0]).not.toHaveProperty('docUrl')
  })
})

describe('buildRouteDocUrl', () => {
  it('builds url with param segments stripped from slug', () => {
    expect(buildRouteDocUrl('https://docs.rsshub.app', '81rc', '/realtime/:category?'))
      .toBe('https://docs.rsshub.app/routes/81rc#realtime')
  })

  it('joins multiple literal segments with dash and strips regex constraints', () => {
    expect(buildRouteDocUrl('https://docs.rsshub.app', 'bilibili', '/bilibili/user/video/:uid{[0-9]+}'))
      .toBe('https://docs.rsshub.app/routes/bilibili#bilibili-user-video')
  })

  it('trims trailing slashes on doc base', () => {
    expect(buildRouteDocUrl('https://docs.rsshub.app/', 'weibo', '/weibo/user/:uid'))
      .toBe('https://docs.rsshub.app/routes/weibo#weibo-user')
  })

  it('falls back to default doc base when empty', () => {
    expect(buildRouteDocUrl('', '81rc', '/realtime/:category?'))
      .toBe(`${DEFAULT_RSSHUB_DOC_BASE}/routes/81rc#realtime`)
  })
})

describe('stripOptionalParams', () => {
  it('drops optional param segments with leading slash', () => {
    expect(stripOptionalParams('/81rc/realtime/:category?')).toBe('/81rc/realtime')
  })

  it('strips regex constraints inline', () => {
    expect(stripOptionalParams('/bilibili/user/video/:uid{[0-9]+}')).toBe('/bilibili/user/video/:uid')
  })

  it('keeps required param segments', () => {
    expect(stripOptionalParams('/weibo/user/:uid')).toBe('/weibo/user/:uid')
  })
})

describe('buildRSSHubFeedUrl', () => {
  it('usable_directly prefers example path', () => {
    expect(buildRSSHubFeedUrl({
      baseUrl: 'https://rsshub.app/',
      namespace: '81rc',
      path: '/realtime/:category?',
      parameters: {},
      example: '/81rc/realtime/1',
      usableDirectly: true,
    })).toBe('https://rsshub.app/81rc/realtime/1')
  })

  it('usable_directly falls back to namespace+path', () => {
    expect(buildRSSHubFeedUrl({
      baseUrl: 'https://rsshub.app',
      namespace: 'test',
      path: '/plain',
      parameters: {},
      usableDirectly: true,
    })).toBe('https://rsshub.app/test/plain')
  })

  it('fills params and drops unfilled optional segments', () => {
    expect(buildRSSHubFeedUrl({
      baseUrl: 'https://rsshub.app',
      namespace: 'weibo',
      path: '/user/:uid/:tab?',
      parameters: { uid: '123' },
    })).toBe('https://rsshub.app/weibo/user/123')
  })

  it('encodes param values', () => {
    expect(buildRSSHubFeedUrl({
      baseUrl: 'https://rsshub.app',
      namespace: 'test',
      path: '/list/:name',
      parameters: { name: 'a b/c' },
    })).toBe('https://rsshub.app/test/list/a%20b%2Fc')
  })

  it('returns empty string when required param missing', () => {
    expect(buildRSSHubFeedUrl({
      baseUrl: 'https://rsshub.app',
      namespace: 'weibo',
      path: '/user/:uid',
      parameters: {},
    })).toBe('')
  })

  it('falls back to default base url when empty', () => {
    expect(buildRSSHubFeedUrl({
      baseUrl: '',
      namespace: 'test',
      path: '/plain',
      parameters: {},
      usableDirectly: true,
    })).toBe(`${DEFAULT_RSSHUB_BASE_URL}/test/plain`)
  })
})
