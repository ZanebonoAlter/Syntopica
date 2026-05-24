function getConfigApiBase(): string {
  const config = useRuntimeConfig()
  return config.public.apiBase as string
}

function isDev(): boolean {
  if (import.meta.server) return true
  return window.location.port === '3000'
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
  return window.location.origin
}

export function getApiBaseUrl(): string {
  if (import.meta.server) return 'http://localhost:5000/api'
  return resolveApiBase()
}

export function getApiOrigin(): string {
  if (import.meta.server) return 'http://localhost:5000'
  return resolveApiOrigin()
}
