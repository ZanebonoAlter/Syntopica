
## 任务1.3「现行红」未复现：包锚点 baseline 归属现状已可判外部

aggregate-concurrent-gate-warns apply 现场发现：任务 1.3 括号注记「包锚点 # <pkg> 目录∩已知归属集全 foreign → 现行 fail-open → 红」与实际不符——实测（behavior smoke 场景 R1）：会话启动基线含外部文件 + lint 输出仅 `# syntopica-backend/internal/pkgowner [build failed]`，现行 classifyFailureOwnership 经 pathMemberOf 前缀扫描即判 foreign → [外部]，用例绿。原因：extractFailurePaths 现已将 #/FAIL 包锚点映射为 backend-go/<pkg>/ 目录前缀（failure-classify.ts GO_PKG_ANCHOR_RE + GO_MODULE_DIR_MAP，B3/B4 用例锁定），pathMemberOf 对尾部 / 前缀做成员扫描，等价满足「目录∩known 全 foreign」。真正救不了的 fail-open 是「?? 新文件会话中新建、不在基线也不在对方 edit.map」（design Risks 已 scope-out 为诊断项）。故 R1/R1b/R2 均作防回归锚点；4.1 的实现按 design D3 做「目录前缀 → 目录∩已知归属集实际文件路径」的字面化重构，须保持 R1/R2 绿。module 名已核对 = syntopica-backend（go.mod），映射表命中。

<!-- pinned 2026-09-22T09:16:01Z -->

## 诊断项：board-signal-reports 的 edit.map 未覆盖其 ?? 后端新文件（design Migration Plan 指定，不阻塞归档）

2026-09-22 apply 阶段取证（harness-facts 配方：`SELECT ... FROM events WHERE kind='edit.map' AND change='board-signal-reports'`）：

- board-signal-reports 历史 edit.map **仅 17 条 distinct 路径，全部是 `openspec/changes/board-signal-reports/**` 文档**；`backend-go/%` 前缀查询**零条**。
- 同期 `git ls-files --others --exclude-standard` 可见该 change 的在途代码新文件（`backend-go/internal/dataenrichment/service/signal_*.go`、`signal_*_test.go`、`repository/signal_*.go`、`handler/signal_*.go` 等 20+ 个 `??` 文件）。
- 结论：**缺口坐实**——该 change 的 `??` 后端新文件从未进 edit.map 归属地图。根因方向：edit.map 仅在「本回合有纯编辑工具（edit/write/apply_patch）调用」时记账（门控见 quality-gate step 2.5，fix-doc-impact-misattribution 收紧），这些新文件大概率经 bash（heredoc/代码生成/sed -i）产生 → 不进 edit.map。
- 影响：其他会话对这些包的失败（含本 change 修好的包锚点解析）在 `foreign` 侧查不到归属 → 按 spec 保守 fail-open 成 [回归]/[中间态] 催错人（即 proposal 里 2026-09-22 现场的直接根因）。
- 处置（按 design 预定）：**另行立项**（候选方向：bash 生成文件的归属补录/edit.map 门控扩展），不改本 change 的 specs/tasks。
