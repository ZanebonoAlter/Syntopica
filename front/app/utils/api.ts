type BrowserLocationLike = {
  origin: string
  port: string
}

function getBrowserLocation(): BrowserLocationLike | null {
  if (import.meta.server) return null
  return (globalThis as { window?: { location: BrowserLocationLike } }).window?.location ?? null
}

function getConfigApiBase(): string {
  const config = useRuntimeConfig()
  return config.public.apiBase as string
}

function isDev(): boolean {
  const location = getBrowserLocation()
  if (!location) return true
  return location.port === '3000'
}

function resolveApiBase(): string {
  const base = getConfigApiBase()
  if (base.startsWith('http')) return base
  if (isDev()) return 'http://localhost:5000/api'
  return base
}

function resolveApiOrigin(): string {
  const base = getConfigApiBase()
  if (base.startsWith('http')) return new URL(base).origin
  if (isDev()) return 'http://localhost:5000'
  return getBrowserLocation()?.origin ?? 'http://localhost:5000'
}

export function getApiBaseUrl(): string {
  if (import.meta.server) return 'http://localhost:5000/api'
  return resolveApiBase()
}

export function getApiOrigin(): string {
  if (import.meta.server) return 'http://localhost:5000'
  return resolveApiOrigin()
}
