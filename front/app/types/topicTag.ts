export interface TagHierarchyNode {
  id: number
  label: string
  slug: string
  category: string
  icon: string
  feedCount: number
  articleCount: number
  similarityScore?: number
  isActive: boolean
  qualityScore?: number
  isLowQuality?: boolean
  children: TagHierarchyNode[]
}

export interface TagHierarchyResponse {
  nodes: TagHierarchyNode[]
  total: number
}

export interface UpdateAbstractNameRequest {
  newName: string
}

export interface DetachChildRequest {
  childId: number
}
