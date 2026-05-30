import { afterEach, describe, expect, it } from 'vitest'

let mockApiBase = 'http://localhost:5000/api'

Object.defineProperty(globalThis, 'useRuntimeConfig', {
  value: () => ({ public: { apiBase: mockApiBase } }),
  writable: true,
  configurable: true,
})

describe('api url resolution', () => {
  const originalLocation = globalThis.location

  afterEach(() => {
    Object.defineProperty(globalThis, 'location', {
      value: originalLocation,
      writable: true,
      configurable: true,
    })
    mockApiBase = 'http://localhost:5000/api'
  })

  it('returns /api when on port 5000 with relative config (production)', async () => {
    mockApiBase = '/api'
    const { getApiBaseUrl, getApiOrigin } = await import('./api')
    Object.defineProperty(globalThis, 'location', {
      value: { port: '5000', origin: 'http://localhost:5000' },
      writable: true,
      configurable: true,
    })
    expect(getApiBaseUrl()).toBe('/api')
    expect(getApiOrigin()).toBe('http://localhost:5000')
  })

  it('returns full dev URL when config has http prefix (dev)', async () => {
    mockApiBase = 'http://localhost:5000/api'
    const { getApiBaseUrl, getApiOrigin } = await import('./api')
    Object.defineProperty(globalThis, 'location', {
      value: { port: '3000', origin: 'http://localhost:3000' },
      writable: true,
      configurable: true,
    })
    expect(getApiBaseUrl()).toBe('http://localhost:5000/api')
    expect(getApiOrigin()).toBe('http://localhost:5000')
  })

  it('returns /api when on port 80/443 with relative config', async () => {
    mockApiBase = '/api'
    const { getApiBaseUrl, getApiOrigin } = await import('./api')
    Object.defineProperty(globalThis, 'location', {
      value: { port: '', origin: 'http://example.com' },
      writable: true,
      configurable: true,
    })
    expect(getApiBaseUrl()).toBe('/api')
    expect(getApiOrigin()).toBe('http://example.com')
  })
})
