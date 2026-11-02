/**
 * 控制台右轨/单级宽工作面数据装配（8C G4b 从 `BoardConsoleShell` 按内聚行为抽出）。
 *
 * 只做「同一份真实数据的取用与下发」：体检快照、构成标签/组合、右轨上下文与收编动作。
 * 不拥有矩阵读取/选中日/阅读槽；`useConsoleRail` 仍是数据实现，本文件只聚合其子状态，
 * 让壳层不再堆 30 行解构。
 */
import { computed } from 'vue'
import { useConsoleRail, type ConsoleRailDeps } from './useConsoleRail'

export function useConsoleShellRail(deps: ConsoleRailDeps) {
  const {
    health,
    composition,
    healthRailSnapshot,
    railContext,
    railBinding,
    removeComposition,
    capture,
  } = useConsoleRail(deps)

  const {
    snapshot: healthSnapshot,
    capturingId: healthCapturingId,
    load: loadHealth,
  } = health

  const {
    labels: compositionLabels,
    composites: compositionComposites,
    loading: compositionLoading,
    error: compositionError,
    load: loadComposition,
  } = composition

  /** 归档视图的构成标签下拉（只 id+label，不发第二份请求）。 */
  const archiveLabels = computed(() => compositionLabels.value.map(label => ({ id: label.id, label: label.label })))

  return {
    healthRailSnapshot,
    railContext,
    railBinding,
    removeComposition,
    captureHealth: capture,
    healthSnapshot,
    healthCapturingId,
    loadHealth,
    compositionLabels,
    compositionComposites,
    compositionLoading,
    compositionError,
    loadComposition,
    archiveLabels,
  }
}
