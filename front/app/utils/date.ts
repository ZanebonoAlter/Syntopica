/**
 * 日期工具函数
 */

/**
 * 格式化相对时间
 * @param dateString - 日期字符串
 * @returns 格式化后的相对时间字符串
 */
export function formatRelativeTime(dateString: string): string {
  const date = new Date(dateString)
  const now = new Date()
  const diff = now.getTime() - date.getTime()
  const minutes = Math.floor(diff / 60000)
  const hours = Math.floor(minutes / 60)
  const days = Math.floor(hours / 24)

  if (minutes < 1) return '刚刚'
  if (minutes < 60) return `${minutes} 分钟前`
  if (hours < 24) return `${hours} 小时前`
  return `${days} 天前`
}

/**
 * 格式化日期
 * @param dateString - 日期字符串
 * @param format - 格式字符串，默认为 'YYYY年MM月DD日 HH:mm'
 * @returns 格式化后的日期字符串
 */
export function formatDate(
  dateString: string,
  format: string = 'YYYY年MM月DD日 HH:mm'
): string {
  const date = new Date(dateString)
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  const hours = String(date.getHours()).padStart(2, '0')
  const minutes = String(date.getMinutes()).padStart(2, '0')

  return format
    .replace('YYYY', String(year))
    .replace('MM', month)
    .replace('DD', day)
    .replace('HH', hours)
    .replace('mm', minutes)
}

/**
 * 检查日期是否为今天
 * @param dateString - 日期字符串
 * @returns 是否为今天
 */
export function isToday(dateString: string): boolean {
  const date = new Date(dateString)
  const today = new Date()
  return (
    date.getDate() === today.getDate() &&
    date.getMonth() === today.getMonth() &&
    date.getFullYear() === today.getFullYear()
  )
}
