/**
 * 收藏切换（乐观更新 + 失败回滚）。
 *
 * 背景：站内原文预览的星标状态来自宿主持有的 Article 对象。若等接口返回才翻转，
 * 用户看到的是「点了没反应」；连点还会因读到旧值而丢更新（两次点击算出同一个目标值）。
 * 本 composable 统一契约：
 *   ① 点击即翻转（乐观）→ 立即写回宿主，界面零等待；
 *   ② 请求失败 → 回滚到点击前状态 + 可见错误提示；
 *   ③ 回滚仅在宿主当前值仍等于本次乐观值时执行（迟到的失败响应不覆盖后续点击）。
 *
 * 规范依据：docs/reference/standard/frontend/code-style.md「调用 API + 乐观更新 + 回滚」。
 */
import { useArticlesApi } from '~/api/articles'
import { useNotify } from '~/composables/useNotify'
import type { Article } from '~/types'

export interface FavoriteToggleHost {
  /** 读取宿主当前持有的文章（基准值 + 回滚判定） */
  get: () => Article | null
  /** 把新文章写回宿主 */
  apply: (article: Article) => void
}

export function useArticleFavoriteToggle() {
  const articlesApi = useArticlesApi()
  const notify = useNotify()

  async function toggleFavorite(articleId: string, host: FavoriteToggleHost): Promise<void> {
    const current = host.get()
    // 无当前文章 / 事件与当前文章对不上 → 忽略（不误改别的文章）
    if (!current || current.id !== articleId) return

    const next: Article = { ...current, favorite: !current.favorite }
    host.apply(next)

    try {
      const response = await articlesApi.updateArticle(Number(articleId), { favorite: next.favorite })
      if (!response.success) throw new Error(response.error || '收藏更新失败')
    } catch (error) {
      const live = host.get()
      // 仅当宿主当前值仍是本次乐观值时才回滚，避免覆盖用户后续点击
      if (live && live.id === articleId && live.favorite === next.favorite) {
        host.apply({ ...live, favorite: current.favorite })
      }
      notify.error(error instanceof Error && error.message ? error.message : '收藏更新失败')
    }
  }

  return { toggleFavorite }
}
