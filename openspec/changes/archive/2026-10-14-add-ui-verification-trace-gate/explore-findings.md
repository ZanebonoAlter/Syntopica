
## UI 验收痕迹对账脚本判定契约与双形态解析边界

## ui-verification-trace.sh 判定契约（实现锚点）

**入口**：`bash scripts/harness/ui-verification-trace.sh <change-dir>`，仿 review-trace.sh（cd REPO_ROOT、只读、exit 0/1）。

**判定流**：
1. `.openspec.yaml` 无 `^schema: syntopica-ui$` → exit 0 fail-open（旧 schema 零检查）
2. proposal.md **头部注释区**（awk head_markers，空行可穿插、遇正文停，与 check-tasks-tail/review-trace 同实现）无合法 ui-impact → exit 0 fail-open（声明完整性归 ui-design-gate lib 管）
3. none：ui-design.md 含 `^##\s+.*(N/A|不适用)` 节 + 头部 ui-impact marker（若有）须=none；缺 N/A 节 → 红
4. minor：抽 ui-design.md Acceptance/验收 节（`^## .*Acceptance|验收` 到下一 `## `）：
   - 组件测试词表（组件测试|component|单测|单元测试，-i）→ 节内须有仓库根相对测试路径 `(front|backend-go)/...*(.test|.spec).(ts|tsx|js...)` 或 `*_test.go`，且对 `$UI_VERIFICATION_TRACE_REPO_ROOT`（smoke 缝，缺省 REPO_ROOT）存在
   - agent-browser/人工/opencli（opencli 历史口径按人工同通道，design D3）→ `ui-verification/report.md` ≥1 合法条目
   - 两词表都无 → 红「裸关键词/无映射」
5. major：`ui-verification/` 目录 + report.md 存在；report.md ≥1 合法条目；点名截图在盘；未覆盖带原因

**report.md 条目双形态**（spec「格式对齐实战形态」要求存量零改造可解析，实战报告是表格态）：
- 标题式：`### A-<N> <标题>【pass|fail|未覆盖】(（说明）可选)` + 空行穿插后首个非空行=依据行
- 表格式（add-topic-key-event-markers 实战）：行 `| A<N> <标题> | **verdict**(（说明）) | 依据 |`，cell1 `^A-?[0-9]+`，cell2 `^\*{0,2}(pass|fail|未覆盖)(（…）)?\*{0,2}$`，cell3 非空
- 未覆盖带原因 = 判定标注（…）+ 依据合并去空白 ≥6 字；不满足 → 该条不计入合法（其余合法条目 >0 时过但打 ⚠，=0 时空壳红——对应 spec「未覆盖场景无原因阻断」THEN）
- 词表外判定（如【搞定】）不计入合法条目

**截图对账**：token 正则 `[A-Za-z0-9][A-Za-z0-9._/-]*\.(png|jpg|jpeg|webp|gif)`，按 `/` 拆段取带图扩展名的段；命中规则 = `ui-verification/<段>` 精确存在，或目录内某文件名以该段结尾且**前邻字符非字母数字**（兜住实战速记 `ke-overview-lanes-star-1440.png` / `-star-zoom-1440.png` 与无杠尾段 `star-zoom-1440.png` 两种形态，不放松到任意子串）。缺失 → 红并列缺失清单。

**spec-gate 接线**（task 2.2）：checkUiEvidence 改 async = lib 规划制品检查（approval/原型/none N/A，D1 保留在扩展）快照判定 + spawn 脚本；runScript code==-1（spawn 异常）→ warn fail-open 不阻断，exit 1 → failures + ui-design-gate block ui-verification-missing（reasonCode 沿用）。

**smoke 驱动**（spec-gate.smoke.cjs 8 系列改写）：仿 7i/7j REAL_RT 先例——mock exec 对 `bash scripts/harness/ui-verification-trace.sh <rel>` 用 spawnSync 真跑 `bash <REAL_UVT> <abs fixture dir>`，其余 passExec；8 系列 fixture 须补 ui-verification/report.md+截图实物。archive-readiness.smoke ① 的 stub 列表要加 ui-verification-trace.sh、✓ 计数 5→6、删「UI 验收证据项不在自查范围」断言。

**check-tasks-tail 第三道防线**：proposal 头 `ui-impact: major` 且 .openspec.yaml schema=syntopica-ui → tasks.md 须有 `^- \[[x ]\]` 且含 `ui-verification/report.md` 的任务行；smoke 补 major 缺行红/含行绿/none 不受影响/schema legacy 不受影响。

**引用**：scripts/harness/review-trace.sh、.pi/extensions/lib/ui-design-gate.ts、.pi/extensions/spec-gate.ts、openspec/changes/add-topic-key-event-markers/ui-acceptance/report.md

<!-- pinned 2026-10-13T02:07:59Z -->
