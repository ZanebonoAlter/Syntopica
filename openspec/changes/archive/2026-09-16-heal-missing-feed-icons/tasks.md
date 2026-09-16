# Tasks: heal-missing-feed-icons

## 1. 后端自愈（IconStore 存在性查询）

- [x] 1.1 `icon_store.go` 新增 `LocalIconExists(localPath string) bool`：接受 DB 里的 `/icons/...` 形式路径，剥离前缀后与存储根拼接 `os.Stat`；仅在明确证明路径不可能存在时（`fs.ErrNotExist` / `syscall.ENOTDIR`）返回 false，其余 stat 错误（权限等）保守返回 true；`iconRelPath` 拒绝非 `/icons/` 前缀、反斜杠、空段与 `.`/`..` 段。验证：`TestLocalIconExists`（10 个子用例：落盘 true / 未写过 false / 目录 false / 空路径 false / 非 icons 路径 false / 远程 URL false / 父目录穿越 false / `.` 段 false / 反斜杠 false / `feeds/7.png/x`（ENOTDIR）false）+ 删除文件后转 false。实测 **PASS**。
- [x] 1.2 `feed_service.go` 的 `resolveFeedIcon` 冻结分支改为「`icon_source == auto` && `/icons/` 前缀 && `LocalIconExists(currentIcon)`」才返回 `ok=false`；文件缺失时继续走候选管线；`localIconPresent` 包装含 nil store 兜底（保持旧冻结语义，避免 nil 解引用），并置于状态机文档块**之上**（review L1：注释块归属 `resolveFeedIcon`）。验证：`TestResolveFeedIcon_AutoLocalIconSkipsPipeline`（文件在→冻结）、`_AutoLocalIconMissingFileHeals`（缺失→重抓落盘为新路径）、`_AutoLocalIconMissingFileAllCandidatesFail`（全失败→`mdi:rss`+`fallback`）。实测 **PASS**。

## 2. 前端降级修正

- [x] 2.1 `FeedIcon.vue` 新增 `ICONIFY_NAME_RE` 合法名判定（`<prefix>:<name>`，小写字母/数字/连字符段；覆盖 `mdi:rss`、`simple-icons:nuxtdotjs`，拒绝 `/icons/...`、http(s)、`data:` URL、历史值 `rss`、无前导斜杠路径），`<Icon>` 分支改用 `placeholderIcon`。验证：`FeedIcon.test.ts` 12 用例全绿（含 3 条回归断言：占位符拿到的 icon 名必须为 `mdi:rss` 而非路径——旧断言只查 `<svg>` 存在，对「空 svg 留白」是假绿）。
- [x] 2.2 阅读页头部 feed 徽标 `ArticleContentToolbar.vue` 由裸 `<Icon :icon="feed.icon">` 改为复用 `FeedIcon`（review M2：同一症状的另一处，DB 里 `/icons/feeds/N.ext` 被当图标名渲染成空 svg）；尺寸 16 / 颜色透传不变。验证：`pnpm exec nuxi typecheck` 通过；`FeedIcon` 组件测试覆盖该降级路径。
- [x] 2.3 前端门禁：`pnpm lint`（0 error，5 条既有 warning，均在本 change 未触碰文件）+ `pnpm exec nuxi typecheck`（exit 0）+ `pnpm exec vitest run app/components/feed/FeedIcon.test.ts`（12 passed）+ `pnpm build`（exit 0，Build complete）。本机为树莓派 Linux（无 cmd.exe），命令直接在本机执行，等价覆盖 `standard/frontend/testing.md` 的跨平台要求。

## 3. Review 后续（receiving-code-review）

- [x] 3.1 L1（本 diff 引入）：状态机文档块被 Go 归给 `localIconPresent` → 已把该函数移到文档块之上，`resolveFeedIcon` 重新拥有文档。
- [x] 3.2 L2：`ENOTDIR`（如 `/icons/feeds/7.png/x`）原先在 Linux 判「存在」、Windows 判「缺失」→ 统一为「缺失」（明确证明路径不可能存在），补 `/icons/feeds/7.png/x` 用例；`GOOS=windows go build ./internal/reader/service/` 验证 `syscall.ENOTDIR` 跨平台可编译。
- [x] 3.3 L3：`iconifyName` 由「非 `/` 且非 http(s)」收紧为 Iconify 名语法正则（`data:` URL / `rss` / 无前导斜杠路径都会被拒），补 3 条用例；用 `app/assets/iconify-subset.json` 全部 175 个名字回归校验正则（0 个被误拒）。
- [x] 3.4 M2（邻域同症状）：`ArticleContentToolbar.vue` 改用 `FeedIcon`（见 2.2）。
- [ ] 3.5 M1（既有隐患，**本 change 不修**）：`internal/platform/database/postgres_migrations.go` 的 `icon_source` 回填 `CASE` 只把 `http(s)://` 判为 `auto`，其余（含 `/icons/...`）判 `custom`——若 `schema_migrations` 缺失该版本（demo seed / dump-sanitizer 都会剔除），dump 恢复后这些行会被锁成 `custom`，冻结分支直接返回、磁盘校验永不执行 → 404 永久化。当前生产数据是 `auto`，不阻塞本 change；建议后续单独 change 加一条新版本迁移补 `WHEN icon LIKE '/icons/%' THEN 'auto'`（不改已应用的历史迁移）。已在完工汇报中提示用户。

## 4. 文档

<!-- doc-impact: reading, deployment -->
- [x] 4.1 `docs/reference/flow/reading.md`：图标链路补「auto + `/icons/` 路径先校验磁盘文件（在→冻结 / 丢失→重抓自愈）」与前端「降级只接受合法 iconify 名」；「业务约束与不变量」第 4 条红线句同步该语义 + DB 路径≠文件在的判据；「变更溯源」补本 change 行。
- [x] 4.2 `docs/reference/deployment.md`：数据持久化节新增「feed 图标目录（运行时资产，`data/icons/`）」——git（`.gitignore` 的 `data/`）与 DB dump 都不携带它，换机/恢复 dump 后三种处置（rsync / 等刷新自愈 / 手动触发单 feed 重抓）+ 验证命令。

## 5. 测试

- [x] 5.1 后端影响包：`cd backend-go && go test -short ./internal/reader/service ./internal/app -count=1` → **ok / ok**（不带 `-short` 时 `TestArticleContentFormColumnInPGSchema` 因 testcontainers 拉 `docker.io/testcontainers/ryuk` 超时失败，环境限制；`-race` 因树莓派 aarch64 TSan「unsupported VMA range」不可用——均为环境问题，与本 change 无关）。
- [x] 5.2 后端门禁命令：`go vet ./...` ✅ / `go build ./...` ✅ / `golangci-lint run ./...`（本机原缺 `golangci-lint`，已 `go install .../v2/cmd/golangci-lint@latest` 后实测 **0 issues**）/ `gofmt -l` 本次改动文件无输出 ✅。
- [x] 5.3 前端用例：`pnpm exec vitest run app/components/feed/FeedIcon.test.ts` → 12 passed（本机无 cmd.exe，直接本机执行）；全量单测 960 passed / 17 failed，失败全部集中在与本次改动无引用关系的 `app/composables/useOnboarding.test.ts`（既有/环境失败，如实记录）。

## 6. 验证

| Scenario | 测试文件 |
| --- | --- |
| custom 状态不被刷新覆盖 | backend-go/internal/reader/service/feed_service_unit_test.go |
| fallback 状态触发重算 | backend-go/internal/reader/service/feed_service_unit_test.go |
| auto + 本地路径跳过重算 | backend-go/internal/reader/service/feed_service_unit_test.go |
| auto + 本地路径但文件缺失时自愈重抓 | backend-go/internal/reader/service/feed_service_unit_test.go |
| 自愈重抓全失败时收敛为 fallback | backend-go/internal/reader/service/feed_service_unit_test.go |
| auto + 存量远程 URL 触发本地化 | backend-go/internal/reader/service/feed_service_unit_test.go |
| favicon URL 加载失败 | front/app/components/feed/FeedIcon.test.ts |
| 本地路径加载失败降级为占位符 | front/app/components/feed/FeedIcon.test.ts |
| 正常 URL 加载成功 | front/app/components/feed/FeedIcon.test.ts |
| iconify 名沿用 | front/app/components/feed/FeedIcon.test.ts |
| 非图标名形态的存量值降级为占位符 | front/app/components/feed/FeedIcon.test.ts |
| 阅读页 feed 徽标复用降级逻辑 | 人工：`front/app/features/articles/components/ArticleContentToolbar.vue` 改用 `FeedIcon`（`pnpm exec nuxi typecheck` 通过 + 线上目视阅读页徽标不空白） |
| 访问不存在的 icon 返回 404、已落盘返回 200 | backend-go/internal/app/icons_route_test.go（回归，未改动） |

- [x] 6.1 `cd backend-go && go test -short ./internal/reader/service ./internal/app` → 全绿（见 5.1）。
- [x] 6.2 前端测试全绿（见 2.3 / 5.3）。
- [x] 6.3 真实部署端到端（2026-09-16 16:08~16:10 实测，树莓派 10.11.12.55）：重启后端（新二进制 16:08:24 起，`/health` 200）→ 重启前 `data/icons/feeds/` 空目录、`GET /icons/feeds/2.ico` **404** → `POST /api/feeds/2/refresh`（202）→ 40s 后 `feeds.id=2` 仍 `auto` + `/icons/feeds/2.ico`，磁盘新落盘 `feeds/2.ico`（3638B，16:09）→ `GET /icons/feeds/2.ico` **200 image/vnd.microsoft.icon 3638B**。同批 `feeds/1.ico`（1184B）由调度器自动刷新一并自愈 → 自愈链路线上成立。
- [x] 6.4 `bash scripts/doc-impact.sh verify openspec/changes/heal-missing-feed-icons` → 通过（声明 reading, deployment，文件 2 个）。
- [x] 6.5 `bash scripts/check-standards.sh --change heal-missing-feed-icons` → 通过 146 / 失败 0。
- [x] 6.6 `bash scripts/scenario-trace.sh openspec/changes/heal-missing-feed-icons` → 通过（12 个 Scenario 映射齐全，自动测试 11 / 人工留痕 1）。
- [x] 6.7 `openspec validate heal-missing-feed-icons` → Change is valid。
