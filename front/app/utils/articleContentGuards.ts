function stripMarkup(value: string): string {
  return value
    .replace(/<style[\s\S]*?<\/style>/gi, ' ')
    .replace(/<script[\s\S]*?<\/script>/gi, ' ')
    .replace(/<[^>]+>/g, ' ')
    .replace(/&nbsp;/gi, ' ')
    .replace(/&amp;/gi, '&')
    .replace(/&lt;/gi, '<')
    .replace(/&gt;/gi, '>')
    .replace(/&quot;/gi, '"')
    .replace(/&#39;/gi, "'")
}

function normalizeText(value: string | null | undefined): string {
  if (!value) return ''

  return stripMarkup(value)
    .replace(/!\[[^\]]*\]\([^)]*\)/g, ' ')
    .replace(/\[([^\]]+)\]\([^)]*\)/g, '$1')
    .replace(/[`*_>#~-]/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
    .toLowerCase()
}

export function shouldShowArticleDescription(description: string | null | undefined, content: string | null | undefined): boolean {
  const normalizedDescription = normalizeText(description)
  // guard 收紧（redesign-reading-pane design D5）：
  // 近空/纯符号（normalize 后 < 4 字符）不展示；纯图片 markdown/HTML 经
  // normalizeText 剥离后为空，已被上面的空判定覆盖，同样不渲染占位块。
  if (!normalizedDescription || normalizedDescription.length < 4) return false

  const normalizedContent = normalizeText(content)
  if (!normalizedContent) return true

  if (normalizedDescription === normalizedContent) return false

  if (normalizedDescription.length >= 40 && normalizedContent.includes(normalizedDescription)) {
    return false
  }

  if (normalizedContent.length >= 40 && normalizedDescription.includes(normalizedContent)) {
    return false
  }

  return true
}
