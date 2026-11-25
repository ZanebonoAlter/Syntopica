/**
 * 文章级 AI 生成疑似度标记的展示口径（add-ai-generated-article-detection）。
 *
 * 口径（design D13/D14）：
 * - 仅判定状态为 suspect 才展示「疑似 AI 生成」标记；
 * - unknown（未判定 / 未开启该订阅源的检测）与 not_ai（已确认非 AI）
 *   一律不展示、不占位、不写「未检测」文案——界面与变更前完全一致；
 * - 结论是「疑似」不是「出错」，故标记只用中性色，不占警示色与规则色语义。
 */
import type { AiDetectionStatus } from '~/types'

/** 四处展示位（列表角标 / 详情 chip / 归档行标记 / 日报线索标记）统一的文案。 */
export const AI_SUSPECT_LABEL = '疑似 AI 生成'

/** 是否处于「疑似 AI 生成」态：只有 suspect 为真。 */
export function isAiSuspect(status: AiDetectionStatus | undefined | null): boolean {
  return status === 'suspect'
}
