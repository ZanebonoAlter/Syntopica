## Context

feed 图标本地化（`localize-icons`，2026-08-01 归档）把图标从「DB 存远程 URL、前端直连」改成「后端下载落盘 `data/icons/feeds/<id>.<ext>`、DB 存 `/icons/...` 同源路径」。这次改造留下一个隐含假设：**DB 里的 `/icons/...` 路径等于磁盘文件一定在**。生产迁移（dump 恢复 DB 到树莓派，图标目录未随行）把这个假设打穿，20 个 feed 全部 404，且状态机因冻结判据只看 DB 而永不重试。

后端进程 cwd 即 `backend-go/`，`storage.icon_dir` 配的是相对路径 `data/icons`（`backend-go/configs/config.yaml`），解析为 `<cwd>/data/icons`；`/icons/*filepath` 路由在启动时 `MkdirAll` 该目录（`backend-go/internal/app/router.go`），所以「目录空但存在」是稳定状态——启动不会失败、只是全部 404。

## Goals / Non-Goals

**Goals:**
- DB 与磁盘不一致时**自愈**：图标文件缺失的 feed 在下一次刷新时自动重抓落盘，无需人工重置数据。
- 前端在图标不可用时**有占位、不留白**，符合既有 spec 的降级要求。
- 迁移操作清单不再漏掉运行时资产目录。

**Non-Goals:**
- 不改成「DB 不做缓存、每次刷新都重抓图标」（会造成无谓的外网请求与图标抖动）。
- 不新增定时全量磁盘巡检 job / 不做 DB↔磁盘对账接口（当前 YAGNI；自愈走既有刷新链路即可）。
- 不改 API 形态、DB schema、配置项语义；不改 `/icons` 静态路由的安全头。
- 不自动帮用户补数据（一次性数据修复是可选的运维命令，代码修复本身足以让它逐渐自愈）。

## Decisions

### D1：自愈判据放在 `resolveFeedIcon` 的冻结分支，做一次 `os.Stat`

冻结条件从 `icon_source == "auto" && strings.HasPrefix(icon, "/icons/")` 收紧为「且本地文件存在」。存在 → 保持原语义（冻结，绝不覆盖好图标）；不存在 → 落回候选管线，走与 fallback 完全相同的路径。

**为什么不是「DB 只存来源语义、图标一律重抓」**：那会让每次刷新都发 3 个外部请求、并把「远程临时故障」变成覆盖本地好图标的路径——正是当年加冻结判据要避免的。

**为什么不是「刷新前统一 stat 一遍全表」**：无谓 I/O 与批处理复杂度；单 feed 刷新时 stat 一次成本可忽略（Linux 上 ~µs 级，且仅在 auto + `/icons/` 分支）。

### D2：存在性查询落在 `IconStore`，不散落在 service

新增 `IconStore.LocalIconExists(localPath string) bool`：入参是 DB 里的 `/icons/...` 形式路径，内部换算回存储目录下的相对路径（`/icons/` 前缀剥离后 `filepath.Join(dir, ...)`），并拒绝路径越界（`..`、绝对路径）与非法前缀。让「存储布局知识」只存在于 `IconStore`，`feed_service` 只问「这个路径的图标还在不在」。

替代方案：service 里直接 `os.Stat` 拼路径——会把存储布局泄漏到 service，且删除/改名存储目录时容易漏改，弃。

### D3：前端降级只接受合法 iconify 名

`FeedIcon.vue` 增加 `iconifyName` 判定（非 `/` 开头且非 http(s) → 视为 iconify 名），降级分支渲染 `iconifyName ?? 'mdi:rss'`。这样以后任何未知来源的字符串都不会被当成图标名丢给 Iconify 渲染成空白。

## Risks / Trade-offs

| 风险 | 缓解 |
| --- | --- |
| stat 失败被误判为「文件缺失」→ 好图标被重新下载 | 只有 `os.Stat` 明确 `IsNotExist` 才判定缺失；其他错误（权限等）保守视作存在（保持冻结），宁可 404 也不抖动 |
| 每次 stat 引入 I/O 开销 | 仅在 auto + `/icons/` 分支执行一次 `Stat`，无网络调用；刷新频率按 feed 分钟级，可忽略 |
| 自愈依赖 feed 刷新，不主动触发 | 预期行为：刷新周期内自然恢复。急用可手工重置 `icon_source='fallback'`（change 归档后写进运维说明） |
| 迁移类事故再次发生（其它运行时资产：`logs/`、容器卷） | 本 change 只补 `data/icons/` 一节；general 化的「运行时资产清单」超出必要范围，文档里按「git/dump 不携带」原则提示 |
