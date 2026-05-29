# Scheduler Status And Trigger Design

**Date:** 2026-03-08
**Status:** approved
**Scope:** `auto_refresh`、`auto_summary` 后端状态落库、手动触发、前端定时任务 tab 反馈

---

## 1. 目标

把 scheduler 面板从“只能看大概状态”改成“能知道有没有真跑、为什么没跑、手动触发有没有真的生效”。

本次只处理两个任务：

- `auto_refresh`
- `auto_summary`

并保持入口继续放在 `GlobalSettingsDialog` 的 `schedulers` tab。

## 2. 当前问题

### 2.1 `auto_refresh`

- cron 会启动，但执行时没有持续更新 `scheduler_tasks`
- SQLite 里看起来像一直没跑
- 前端只能看到静态 interval，看不到这轮是否扫描了 feed、是否真的触发刷新

### 2.2 `auto_summary`

- 自动定时逻辑存在
- `/api/schedulers/auto_summary/trigger` 返回成功，但没有真正执行一次
- 前端会误以为按钮有效

### 2.3 前端

- 当前 tab 对 `ai_summary` 说明很多
- 对 `auto_refresh` 和 `auto_summary` 缺少“有效性判断”和“为什么没动”的解释
- trigger 成功提示过于乐观，没区分“真的开始执行”和“接口只是接受请求”

## 3. 方案

### 3.1 后端

- 给 `auto_refresh` 增加完整运行状态更新
- 每轮记录：开始时间、结束时间、扫描 feed 数、符合条件数、实际触发数、跳过原因、错误信息
- 给 `auto_summary` 增加真正的 `Trigger()`，内部复用已有执行逻辑
- scheduler trigger 接口改成返回更明确的结果字段，比如是否 accepted、是否 actually_started、reason/message

### 3.2 前端

- 继续使用 `GlobalSettingsDialog` 的 `schedulers` tab
- 为 `auto_refresh` 增加说明卡，展示：扫描数量、触发数量、无任务原因、最近运行摘要
- 为 `auto_summary` 增加 trigger 结果提示，区分：已启动、正在执行中、缺配置、无可处理 feed
- trigger 按钮点击后根据返回值决定提示文案，不再统一写“任务已触发”

## 4. 不做的事

- 不重做整个 scheduler UI
- 不修改其他 scheduler 的结构
- 不改成新页面

## 5. 验收标准

- `auto_refresh` 在 SQLite 的 `scheduler_tasks` 中能看到持续变化的运行信息
- `auto_summary` 手动 trigger 会真实执行，或者明确返回为什么没执行
- 前端 tab 能区分“按钮有效”和“只是返回 200”
- 前端能看到 `auto_refresh` 与 `auto_summary` 的状态说明
