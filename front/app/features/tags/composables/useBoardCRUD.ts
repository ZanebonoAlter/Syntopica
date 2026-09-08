import { ref, nextTick } from 'vue'
import { useSemanticBoardsApi, type SemanticBoard, type AuxiliaryLabelItem, type BoardCompositeMount } from '~/api/semanticBoards'

export function useBoardCRUD() {
  const sbApi = useSemanticBoardsApi()

  const boards = ref<SemanticBoard[]>([])
  const selectedBoardId = ref<number | null>(null)
  const boardsLoading = ref(false)
  const boardsError = ref<string | null>(null)

  const compositionLabels = ref<AuxiliaryLabelItem[]>([])
  const compositionComposites = ref<BoardCompositeMount[]>([])
  const compositionLoading = ref(false)

  const editingBoard = ref<SemanticBoard | null>(null)
  const editLabel = ref('')
  const editDescription = ref('')
  const editEnrichmentEnabled = ref(false)
const editRelationAutoDiscoveryEnabled = ref(false)
  const editWindowDays = ref(14)
  const editContextLayers = ref<string[]>(['week', 'month', 'year', 'all'])
  const editSaving = ref(false)
  const editError = ref<string | null>(null)

  const showAddDialog = ref(false)

  const DEFAULT_CONTEXT_LAYERS = ['week', 'month', 'year', 'all']

  async function loadBoards() {
    boardsLoading.value = true
    boardsError.value = null
    const res = await sbApi.getBoards()
    if (res.success && res.data) {
      boards.value = res.data.items
    } else {
      boardsError.value = res.error || '加载板块失败'
    }
    boardsLoading.value = false
  }

  async function loadComposition(boardId: number) {
    compositionLoading.value = true
    const res = await sbApi.getComposition(boardId)
    if (res.success && res.data) {
      compositionLabels.value = res.data.items
      compositionComposites.value = res.data.composites ?? []
    } else {
      compositionLabels.value = []
      compositionComposites.value = []
    }
    compositionLoading.value = false
  }

  function handleSelectBoard(id: number | null) {
    selectedBoardId.value = id
    if (id !== null) {
      void loadComposition(id)
    } else {
      compositionLabels.value = []
      compositionComposites.value = []
    }
  }

  function openEditBoard(board: SemanticBoard) {
    editingBoard.value = board
    editLabel.value = board.label
    editDescription.value = board.description || ''
    editEnrichmentEnabled.value = board.enrichment_enabled ?? false
    editWindowDays.value = board.window_days ?? 14
    editRelationAutoDiscoveryEnabled.value = board.relation_auto_discovery_enabled ?? false
    editContextLayers.value = Array.isArray(board.context_layers) && board.context_layers.length > 0
      ? [...board.context_layers]
      : [...DEFAULT_CONTEXT_LAYERS]
    editError.value = null
  }

  function closeEditBoard() {
    if (editSaving.value) return
    editingBoard.value = null
    editLabel.value = ''
    editDescription.value = ''
    editEnrichmentEnabled.value = false
    editWindowDays.value = 14
    editContextLayers.value = [...DEFAULT_CONTEXT_LAYERS]
    editError.value = null
  }

  async function handleSaveBoardEdit() {
    const board = editingBoard.value
    const label = editLabel.value.trim()
    if (!board || !label) return
    editSaving.value = true
    editError.value = null
    try {
      const res = await sbApi.updateBoard(board.id, {
        label,
        description: editDescription.value.trim(),
        enrichment_enabled: editEnrichmentEnabled.value,
        window_days: editWindowDays.value,
        relation_auto_discovery_enabled: editRelationAutoDiscoveryEnabled.value,
        context_layers: [...editContextLayers.value],
      })
      if (res.success) {
        await loadBoards()
        editSaving.value = false
        closeEditBoard()
      } else {
        editError.value = res.error || '保存失败'
      }
    } finally {
      editSaving.value = false
    }
  }

  function handleAddBoard(data: { label: string; description: string; display_order: number; protected: boolean; auxiliary_labels?: number[] }) {
    sbApi.createBoard(data).then((res) => {
      if (res.success) {
        showAddDialog.value = false
        void nextTick().then(() => loadBoards())
      } else {
        boardsError.value = res.error || '添加失败'
      }
    })
  }

  function handleDeleteBoard(id: number) {
    const board = boards.value.find(b => b.id === id)
    if (!board) return
    if (!confirm(board.protected ? `此板块受保护，确认删除？` : `确认删除板块"${board.label}"？`)) return
    sbApi.deleteBoard(id).then((res) => {
      if (res.success) {
        if (selectedBoardId.value === id) selectedBoardId.value = null
        void loadBoards()
      } else {
        boardsError.value = res.error || '删除失败'
      }
    })
  }

  function handleRemoveComposition(auxiliaryLabelId: number) {
    if (selectedBoardId.value === null) return
    sbApi.removeFromComposition(selectedBoardId.value, auxiliaryLabelId).then(() => {
      void loadComposition(selectedBoardId.value!)
    })
  }

  function sourceIcon(source: string): string {
    switch (source) {
      case 'manual': return 'mdi:lock'
      case 'llm_extract': return 'mdi:robot'
      default: return 'mdi:lightning-bolt'
    }
  }

  function sourceTitle(source: string): string {
    switch (source) {
      case 'manual': return '手动创建'
      case 'llm_extract': return 'LLM 生成'
      default: return '自动生成'
    }
  }

  return {
    boards, selectedBoardId, boardsLoading, boardsError,
    compositionLabels, compositionComposites, compositionLoading,
    editingBoard, editLabel, editDescription,
    editEnrichmentEnabled, editRelationAutoDiscoveryEnabled, editWindowDays, editContextLayers,
    editSaving, editError,
    showAddDialog,
    loadBoards, loadComposition, handleSelectBoard,
    openEditBoard, closeEditBoard, handleSaveBoardEdit,
    handleAddBoard, handleDeleteBoard, handleRemoveComposition,
    sourceIcon, sourceTitle,
  }
}
