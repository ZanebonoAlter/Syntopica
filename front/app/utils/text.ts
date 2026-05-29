/**
 * 文本工具函数
 */

/**
 * 截断文本
 * @param text - 原始文本
 * @param maxLength - 最大长度
 * @returns 截断后的文本
 */
export function truncateText(text: string, maxLength: number): string {
  if (text.length <= maxLength) return text
  return text.substring(0, maxLength) + '...'
}

/**
 * 清理 HTML 标签
 * @param html - HTML 字符串
 * @param maxLength - 最大长度
 * @returns 清理后的纯文本
 */
export function cleanHtml(html: string, maxLength: number = 200): string {
  if (!html) return ''

  // 移除 HTML 标签
  const text = html.replace(/<[^>]*>/g, '')

  // 解码 HTML 实体
  const textarea = document.createElement('textarea')
  textarea.innerHTML = text
  return textarea.value.substring(0, maxLength)
}

/**
 * 从 HTML 中提取第一张图片的 URL
 * @param html - HTML 字符串
 * @returns 图片 URL 或 undefined
 */
export function extractFirstImage(html: string): string | undefined {
  if (!html) return undefined

  const imgMatch = html.match(/<img[^>]+src="([^">]+)"/i)
  return imgMatch?.[1] || undefined
}

/**
 * 高亮关键词
 * @param text - 原始文本
 * @param keyword - 关键词
 * @returns 高亮后的 HTML 字符串
 */
export function highlightKeyword(text: string, keyword: string): string {
  if (!keyword) return text
  const regex = new RegExp(`(${keyword})`, 'gi')
  return text.replace(regex, '<mark>$1</mark>')
}

/**
 * 生成随机颜色
 * @returns 随机颜色十六进制字符串
 */
export function generateRandomColor(): string {
  const colors = ['#3b82f6', '#ef4444', '#10b981', '#f59e0b', '#8b5cf6', '#ec4899', '#6b7280']
  return colors[Math.floor(Math.random() * colors.length)] || '#6b7280'
}

/**
 * 从分类 ID 获取颜色
 * @param categoryId - 分类 ID
 * @returns 颜色十六进制字符串
 */
export function getCategoryColor(categoryId: string): string {
  const colorMap: Record<string, string> = {
    tech: '#3b82f6',
    news: '#ef4444',
    design: '#8b5cf6',
    blog: '#10b981',
    ai: '#f59e0b',
    product: '#ec4899',
  }
  return colorMap[categoryId] || '#6b7280'
}
