/**
 * 分类相关类型定义
 */

/**
 * 分类数据模型
 */
export interface Category {
  id: string
  name: string
  slug: string
  icon: string
  color: string
  description: string
  feedCount: number
}

/**
 * 分类创建数据
 */
export interface CreateCategoryData {
  name: string
  icon?: string
  color?: string
  description?: string
}

/**
 * 分类更新数据
 */
export interface UpdateCategoryData {
  name?: string
  icon?: string
  color?: string
  description?: string
}
