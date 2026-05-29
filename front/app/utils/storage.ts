/**
 * 本地存储工具函数
 */

/**
 * 从 localStorage 获取数据
 * @param key - 存储键名
 * @returns 存储的数据或 null
 */
export function getLocalStorage<T>(key: string): T | null {
  if (typeof window === 'undefined') return null

  try {
    const item = localStorage.getItem(key)
    return item ? JSON.parse(item) : null
  } catch (error) {
    console.error(`读取 localStorage 失败: ${key}`, error)
    return null
  }
}

/**
 * 设置 localStorage 数据
 * @param key - 存储键名
 * @param value - 要存储的数据
 */
export function setLocalStorage<T>(key: string, value: T): void {
  if (typeof window === 'undefined') return

  try {
    localStorage.setItem(key, JSON.stringify(value))
  } catch (error) {
    console.error(`写入 localStorage 失败: ${key}`, error)
  }
}

/**
 * 从 localStorage 移除数据
 * @param key - 存储键名
 */
export function removeLocalStorage(key: string): void {
  if (typeof window === 'undefined') return

  try {
    localStorage.removeItem(key)
  } catch (error) {
    console.error(`删除 localStorage 失败: ${key}`, error)
  }
}

/**
 * 监听 localStorage 变化
 * @param key - 存储键名
 * @param callback - 变化回调函数
 */
export function onLocalStorageChange<T>(
  key: string,
  callback: (value: T | null) => void
): () => void {
  const handler = (e: StorageEvent) => {
    if (e.key === key) {
      const value = e.newValue ? JSON.parse(e.newValue) : null
      callback(value)
    }
  }

  window.addEventListener('storage', handler)

  // 返回清理函数
  return () => window.removeEventListener('storage', handler)
}
