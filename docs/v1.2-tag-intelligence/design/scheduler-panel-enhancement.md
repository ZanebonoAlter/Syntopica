# Scheduler Panel Enhancement Design

Date: 2026-04-16

## Problem

GlobalSettings 定时任务面板存在两个问题：

1. **Digest 冗余**：Digest 有独立的配置页面 (`/api/digest/*`)，但同时在定时任务列表中重复展示。
2. **5 个辅助任务缺专属 UI**：`preference_update`、`blocked_article_recovery`、`auto_tag_merge`、`tag_quality_score`、`narrative_summary` 在前端只显示通用卡片（名称+状态+间隔），没有中文名、图标、颜色映射，也没有运行摘要面板。

## Solution (Approach A: Minimal Enhancement)

### Part 1: Remove Digest from Scheduler List

**Backend** (`handler.go`):
- Remove `digest` entry from `schedulerDescriptors()`. Digest keeps its own page at `/api/digest/*`.

**Frontend**:
- No special digest rendering needed — it simply won't appear in the list.

### Part 2: Enhance 5 Auxiliary Task Panels

#### Backend Changes

**`preference_update`** — Enhance `GetStatus()` to return structured overview:
- `total_preferences`: total preference records
- `updated_count`: preferences updated in last run
- `orphan_repaired_count`: orphan reading behaviors repaired
- `deleted_count`: unrecoverable behaviors deleted

**`blocked_article_recovery`** — Enhance `GetStatus()` to return structured overview:
- `recovered_count`: articles recovered in last run
- `current_blocked_count`: currently blocked articles
- `threshold`: warning threshold (50)

The other 3 tasks (`auto_tag_merge`, `tag_quality_score`, `narrative_summary`) already store structured `last_execution_result` JSON in `scheduler_tasks` table. Frontend `enrichStatus()` already picks this up as `last_run_summary`.

#### Frontend Changes

**`schedulerMeta.ts`** — Add display metadata:

| Task | Chinese Name | Icon | Color |
|------|-------------|------|-------|
| `preference_update` | 偏好更新 | `mdi:heart-outline` | `from-pink-500 to-rose-500` |
| `blocked_article_recovery` | 阻塞恢复 | `mdi:shield-check-outline` | `from-emerald-500 to-teal-500` |
| `auto_tag_merge` | 标签合并 | `mdi:merge` | `from-violet-500 to-purple-500` |
| `tag_quality_score` | 标签评分 | `mdi:star-outline` | `from-indigo-500 to-blue-500` |
| `narrative_summary` | 叙事摘要 | `mdi:book-open-page-variant-outline` | `from-cyan-500 to-sky-500` |

**`GlobalSettingsDialog.vue`**:
- Add dedicated summary panels for each auxiliary task (similar to auto_refresh panel style)
- Update bottom description text to cover all 8 tasks

#### Summary Panel Specs

**`preference_update`** panel:
- Card with gradient `from-pink-50 to-white`
- Metrics: 偏好总数, 已更新, 孤儿修复, 已删除

**`blocked_article_recovery`** panel:
- Card with gradient `from-emerald-50 to-white`
- Metrics: 已恢复, 当前阻塞数, 告警阈值

**`auto_tag_merge`** panel:
- Card with gradient `from-violet-50 to-white`
- Metrics: 检查配对数, 已合并, 跳过, 失败

**`tag_quality_score`** panel:
- Card with gradient `from-indigo-50 to-white`
- Metrics: 评分标签数

**`narrative_summary`** panel:
- Card with gradient `from-cyan-50 to-white`
- Metrics: 已保存叙事摘要数

## Files to Modify

### Backend
- `backend-go/internal/jobs/handler.go` — Remove digest descriptor
- `backend-go/internal/jobs/preference_update.go` — Enhance GetStatus with overview
- `backend-go/internal/jobs/blocked_article_recovery.go` — Enhance GetStatus with overview

### Frontend
- `front/app/utils/schedulerMeta.ts` — Add 5 task display metadata
- `front/app/components/dialog/GlobalSettingsDialog.vue` — Add panels, update description text
- `front/app/utils/schedulerMeta.test.ts` — Update tests if needed

## Success Criteria

1. Digest does not appear in `/api/schedulers/status` response
2. All 5 auxiliary tasks show Chinese name, icon, and colored badge
3. Each auxiliary task shows a dedicated summary panel with business metrics
4. Bottom description text covers all 8 tasks
5. `pnpm exec nuxi typecheck` passes
6. `go build ./...` passes
