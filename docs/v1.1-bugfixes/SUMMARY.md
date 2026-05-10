# Milestone v1.1 — 业务漏洞修复总结

**生成时间:** 2026-04-12
**用途:** 团队入职和项目回顾
**状态:** 4/6 阶段完成，第5-6阶段延期

---

## 1. 项目概览

**项目名称:** RSS Reader 漏洞修复 (v1.1)
**核心价值:** 修复代码审查发现的 6 类后端业务逻辑漏洞，确保定时任务触发、标签提取流程、状态一致性等核心业务正确运行。
**部署模式:** 个人/单用户，无认证系统，SQLite 持久化。

本里程碑起因是用户运行 RSS Reader 一段时间后发现多个影响业务正确性的问题：
- 定时任务并发执行不完整，可能丢失任务或重复执行
- 标签提取流程绕过统一队列，导致流程混乱
- 文章状态转换有遗漏，导致卡死不处理
- 前端 API 调用不一致，造成状态漂移
- 多处缺少 panic recovery，服务有崩溃风险
- stale 状态无恢复机制，任务永久卡死

**里程碑当前状态:** Phase 01-04 已完成，Phase 05 (错误处理) 和 Phase 06 (恢复机制) 尚未执行。额外完成一个快速修复任务 (stale feed recovery)。

---

## 2. 架构与技术决策

### 技术栈

| 层 | 技术 |
|----|------|
| Frontend | Nuxt 4, Vue 3, TypeScript, Pinia, Tailwind CSS v4 |
| Backend | Go (Gin, GORM), SQLite |
| 实时通信 | WebSocket (ws.Hub) |
| 定时任务 | robfig/cron + 自定义 scheduler |
| 标签提取 | TagJobQueue 异步队列 |

### 关键技术决策

- **Decision:** Auto-refresh 完成通知使用 WebSocket 广播而非新增轮询 API
  - **Why:** 复用现有 ws.Hub 基础设施，减少改动量
  - **Phase:** 01 (CONC-01)

- **Decision:** Firecrawl TriggerNow 返回 batch_id，与 runCrawlCycle 复用同一批次号
  - **Why:** 前端可按 batch_id 关联 WebSocket 进度消息，建立触发-跟踪闭环
  - **Phase:** 01 (CONC-02)

- **Decision:** 手动打标签 API 改为异步入队 TagJobQueue，不再同步调用 RetagArticle
  - **Why:** 统一所有标签提取入口（Firecrawl / ContentCompletion / 手动），消除绕过队列的两套路径
  - **Phase:** 02 (TAG-03)

- **Decision:** TagQueue 启动失败改为后台非阻塞重试（30s 间隔，最多 10 次）
  - **Why:** 不阻塞应用启动，允许数据库/表延迟可用
  - **Phase:** 02 (TAG-04)

- **Decision:** Feed 删除时使用 CASCADE 级联删除文章，而非标记 "abandoned"
  - **Why:** REQUIREMENTS 允许"标记或清理"，CASCADE 更简洁且已存在约束
  - **Phase:** 03 (STAT-01)

- **Decision:** 新建独立 BlockedArticleRecoveryScheduler 恢复阻塞文章
  - **Why:** 每小时检查 feed 状态变化，将 waiting_for_firecrawl 重置为 pending
  - **Phase:** 03 (STAT-04)

- **Decision:** Scheduler trigger 前端统一走 apiClient，未读数本地精准更新
  - **Why:** 避免全量 refetch，保持即时 UI 反馈
  - **Phase:** 04 (API-01/02)

- **Decision:** 后端 scheduler status 统一返回 `SchedulerStatusResponse` 五字段结构
  - **Why:** 前端可用同一解析逻辑消费所有 scheduler 状态
  - **Phase:** 04 (API-04)

- **Decision:** `next_run` 统一为 Unix 时间戳，`name` 为展示名
  - **Why:** 消除不同 scheduler 混用 time.Time / RFC3339 string 的不一致
  - **Phase:** 04 (API-04)

---

## 3. 交付阶段

| Phase | Name | Status | Summary |
|-------|------|--------|---------|
| 01 | 并发控制修复 | ✅ 完成 (gaps: 前端未消费 WS) | TriggerNow 统一错误格式、Firecrawl 返回 batch_id、Auto-refresh 完成广播 |
| 02 | 标签流程统一 | ✅ 完成 (gaps: handler 回查不稳定、前端未消费 WS) | 手动打标签改为异步入队、TagQueue 后台重试、tag_completed 广播 |
| 03 | 状态一致性修复 | ✅ 完成 (passed) | summary_status 初始化修正、BlockedArticleRecoveryScheduler、阻塞告警 |
| 04 | API 规范化 | ✅ 完成 (gaps: digest 未统一、前后端 name/next_run 语义不对齐) | apiClient 统一、unreadCount 同步、SchedulerStatusResponse 契约 |
| 05 | 错误处理完善 | ⏳ 未执行 | panic recovery、错误持久化、Digest 执行记录 |
| 06 | 恢复机制 | ⏳ 未执行 | stale 状态自动恢复、TagQueue 失败 backoff |
| — | Quick: stale feed recovery | ✅ 完成 | Feed 刷新卡住超过 5 分钟被重置 |

### 验证得分

| Phase | Score | Status |
|-------|-------|--------|
| 01 | 6/8 | gaps_found |
| 02 | 4/6 | gaps_found |
| 03 | 5/5 (1 override) | **passed** |
| 04 | 3/5 | gaps_found |

---

## 4. 需求覆盖度

### ✅ 已满足 (14/23)

| ID | Description | Phase |
|----|-------------|-------|
| CONC-01 | Auto-refresh 正确等待所有 goroutine 完成再触发 auto-summary | 01 |
| CONC-02 | Firecrawl TriggerNow 返回实际执行状态 | 01 |
| CONC-03 | 所有 TriggerNow 锁定失败返回一致错误格式 | 01 |
| CONC-04 | 每个 goroutine 独立 panic recovery | 01 |
| CONC-05 | Digest scheduler reload 优雅停止再启动 | 01 |
| TAG-01 | Firecrawl 完成后走 TagJobQueue | 02 |
| TAG-02 | ContentCompletion 完成后走 TagJobQueue | 02 |
| TAG-03 | 手动打标签走 TagJobQueue | 02 |
| TAG-04 | TagQueue 启动失败后台重试 | 02 |
| TAG-05 | TagArticle 幂等检查 | 02 |
| TAG-06 | RetagArticle 清理旧标签 | 02 |
| STAT-03 | summary-only feed 的 summary_status 初始化为 pending | 03 |
| STAT-04 | 阻塞文章自动恢复 | 03 |
| STAT-05 | 阻塞数量超过阈值告警 | 03 |
| API-01 | Scheduler trigger 使用 apiClient | 04 |
| API-02 | UpdateArticle 刷新 unreadCount | 04 |
| API-03 | MarkAllAsRead 覆盖所有边界 | 04 |

### ⚠️ 部分满足 (3/23)

| ID | Description | Phase | Gap |
|----|-------------|-------|-----|
| STAT-01 | Feed 删除时文章处理 | 03 | 使用 CASCADE 删除（满足"清理"选项），但 ROADMAP 措辞要求 "abandoned" |
| STAT-02 | CleanupOldArticles feed 不存在处理 | 03 | 有意跳过，CASCADE 已清理相关文章 |
| API-04 | 所有 scheduler status 格式一致 | 04 | 5/6 scheduler 已统一，digest 仍返回 legacy 格式 |

### ❌ 未满足 (6/23)

| ID | Description | Phase | Reason |
|----|-------------|-------|--------|
| ERR-01 | Firecrawl panic recovery | 05 | Phase 未执行 |
| ERR-02 | Preference update panic recovery | 05 | Phase 未执行 |
| ERR-03 | Digest panic recovery | 05 | Phase 未执行 |
| ERR-04 | Scheduler error 持久化 | 05 | Phase 未执行 |
| ERR-05 | Digest 执行状态记录 | 05 | Phase 未执行 |
| REC-01~04 | Stale 状态恢复机制 | 06 | Phase 未执行 (REC-01 的 stale feed reset 已通过 quick task 部分解决) |

---

## 5. 关键决策记录

| ID | Decision | Phase | Rationale |
|----|----------|-------|-----------|
| D-01 | TriggerNow 异步+完成通知模式 | 01 | 立即返回"触发成功"，通过 WebSocket 感知完成 |
| D-02 | WebSocket 广播替代轮询 API | 01 | 复用 ws.Hub，减少改动 |
| D-03 | batch_id 在 TriggerNow 内生成 | 01 | 同一值供 HTTP 响应和 WebSocket 广播 |
| D-04 | 手动打标签异步入队 | 02 | 统一入口，消除绕过队列的两套路径 |
| D-05 | TagQueue 非阻塞后台重试 | 02 | 不阻塞应用启动 |
| D-06 | CASCADE 删除替代状态标记 | 03 | REQUIREMENTS 允许"标记或清理" |
| D-07 | 新建 BlockedArticleRecoveryScheduler | 03 | 独立定时任务，每小时恢复 |
| D-08 | 前端本地精准更新 unreadCount | 04 | 避免全量 refetch |
| D-09 | SchedulerStatusResponse 统一五字段 | 04 | name/status/check_interval/next_run/is_executing |
| D-10 | next_run 统一 Unix 时间戳 | 04 | 消除 time.Time/RFC3339/string 混用 |

---

## 6. 技术债务与延期项

### 已知 Gap（跨 Phase 共性问题）

1. **前端 WebSocket 消费缺失** (Phase 01/02)
   - `auto_refresh_complete`、`tag_completed`、`firecrawl_progress` 后端已广播，但前端无消费代码
   - 需要在 `useSummaryWebSocket.ts` 或新建 composable 中添加监听

2. **手动打标签 API 回查不稳定** (Phase 02)
   - `handler.go:227-231` 只按 `pending` 回查，leased/快速 claim 场景误报 500
   - 建议：让 Enqueue 直接返回 job 记录

3. **Digest status 未统一** (Phase 04)
   - `/api/digest/status` 仍返回 legacy 字段结构
   - 需要迁移到 `SchedulerStatusResponse` 契约

4. **前后端 name/next_run 语义不对齐** (Phase 04)
   - 后端 name 改为展示名、next_run 改为 Unix 秒
   - 前端 `schedulerMeta.ts` 仍按 slug 判断，`GlobalSettingsDialog.vue` 把 next_run 当字符串
   - 需要统一：要么后端返回 slug+展示名，要么前端全面改为展示名

### Phase 05-06 未执行

- Phase 05 (ERR-01~05): panic recovery、错误持久化
- Phase 06 (REC-01~04): stale 状态恢复、TagQueue 失败 backoff
- **建议作为下一个里程碑执行**

### 其他已知问题

- `TopicTimeline.test.ts` 在 `pnpm test:unit` 中失败（与本里程碑无关）
- TagQueue 启动日志在后台重试中会误导（显示"started successfully"但实际仍在重试）
- tag job 查询 API 不存在，失败态无回传闭环

---

## 7. 快速开始

### 运行项目

```bash
# Backend
cd backend-go
go run cmd/server/main.go    # http://localhost:5000

# Frontend
cd front
pnpm install
pnpm dev                      # http://localhost:3000
```

### 关键目录

| 目录 | 说明 |
|------|------|
| `backend-go/internal/jobs/` | 所有定时任务 scheduler |
| `backend-go/internal/domain/topicextraction/` | 标签队列、标签提取 |
| `backend-go/internal/domain/contentprocessing/` | 内容补全、Firecrawl 集成 |
| `backend-go/internal/domain/feeds/` | Feed 服务、文章创建 |
| `backend-go/internal/app/runtime.go` | Scheduler 注册与启动 |
| `front/app/api/` | HTTP 客户端封装 |
| `front/app/stores/api.ts` | Pinia 主 store |
| `front/app/components/dialog/GlobalSettingsDialog.vue` | 调度器管理面板 |

### 测试

```bash
# Backend tests
cd backend-go
go test ./internal/jobs -v          # scheduler 测试
go test ./internal/domain/feeds -v  # feed 服务测试
go test ./...                        # 全量测试

# Frontend tests
cd front
pnpm test:unit                       # Vitest 单测
pnpm exec nuxi typecheck             # 类型检查

# Integration tests (需要后端运行)
cd tests/workflow
pytest test_*.py -v
```

### 首先阅读

1. `backend-go/internal/app/runtime.go` — 理解所有 scheduler 如何注册启动
2. `backend-go/internal/jobs/handler.go` — HTTP 触发入口和统一 status 契约
3. `backend-go/internal/jobs/auto_refresh.go` — 最完整的 scheduler 参考实现
4. `front/app/stores/api.ts` — 前端数据流核心

---

## 统计信息

- **时间线:** 2026-04-10 → 2026-04-12 (~2 天)
- **阶段:** 4 完成 / 6 总 (Phase 05-06 延期)
- **提交:** 66
- **文件变更:** 161 (+14,293 / -2,156)
- **贡献者:** zanebonoalter
- **快速任务:** 1 (stale feed recovery)
