import { afterEach, describe, expect, it } from 'vitest'

let mockApiBase = '/api'

Object.defineProperty(globalThis, 'useRuntimeConfig', {
  value: () => ({ public: { apiBase: mockApiBase } }),
  writable: true,
  configurable: true,
})

function setLocation(origin: string) {
  Object.defineProperty(globalThis, 'location', {
    value: { origin },
    writable: true,
    configurable: true,
  })
}

describe('api url resolution', () => {
  const originalLocation = globalThis.location

  afterEach(() => {
    Object.defineProperty(globalThis, 'location', {
      value: originalLocation,
      writable: true,
      configurable: true,
    })
    mockApiBase = '/api'
  })

  it('returns default absolute backend base untouched (direct connect, port 5100)', async () => {
    mockApiBase = 'http://localhost:5100/api'
    const { getApiBaseUrl, getApiOrigin } = await import('./api')
    setLocation('http://localhost:3000')
    expect(getApiBaseUrl()).toBe('http://localhost:5100/api')
    expect(getApiOrigin()).toBe('http://localhost:5100')
  })

  it('keeps custom absolute base when NUXT_PUBLIC_API_BASE is set explicitly', async () => {
    mockApiBase = 'http://my-backend:5000/api'
    const { getApiBaseUrl, getApiOrigin } = await import('./api')
    setLocation('http://localhost:3000')
    expect(getApiBaseUrl()).toBe('http://my-backend:5000/api')
    expect(getApiOrigin()).toBe('http://my-backend:5000')
  })

  it('supports relative base resolving to page origin (same-origin deployment)', async () => {
    mockApiBase = '/api'
    const { getApiBaseUrl, getApiOrigin } = await import('./api')
    setLocation('http://example.com')
    expect(getApiBaseUrl()).toBe('/api')
    expect(getApiOrigin()).toBe('http://example.com')
  })

  it('supports custom relative prefix', async () => {
    mockApiBase = '/custom-api'
    const { getApiBaseUrl, getApiOrigin } = await import('./api')
    setLocation('https://syntopica.example.org')
    expect(getApiBaseUrl()).toBe('/custom-api')
    expect(getApiOrigin()).toBe('https://syntopica.example.org')
  })
})
