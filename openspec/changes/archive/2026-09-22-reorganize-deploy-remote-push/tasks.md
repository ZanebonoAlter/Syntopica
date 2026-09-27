## 1. 免密前置（环境准备）

- [x] 1.1 对 10.11.12.59 配置 SSH 免密并确认远端用户名/部署路径（`ssh -o BatchMode=yes zanebono@10.11.12.59 'echo ok'` 输出 `ok`；docker 组权限已配；远端现状与处置决策已回填 design.md Open Questions——替换旧 demo 栈，路径 `~/software/Syntopica`）

## 2. 文件移动与路径修正

- [x] 2.1 `git mv` 移动 8 个文件（3 compose → `deploy/compose/`、3 Dockerfile → `deploy/docker/`、2 init → `deploy/`；`git status` 全部显示 rename；`docker-compose.pg.yml`、`.env*` 留根，根目录仅剩 pg 一个 yml）
- [x] 2.2 修 compose 内 build 指向：`deploy/compose/docker-compose.yml` 的 `dockerfile: deploy/docker/Dockerfile`（CRLF 行尾，保持原样）；`demo/docker-compose.demo.yml` 的 `dockerfile: deploy/docker/Dockerfile.demo`（验证：两个 `config` 均渲染成功，demo context 指向仓库根）
- [x] 2.3 `deploy/init.sh`/`deploy/init.ps1` 自定位仓库根 + compose 调用统一 `--project-directory` + `-f`（init.sh 顺手修了历史遗留的 CRLF 行尾——Linux 入口脚本原本带 `\r` 在 bash 下直接语法错；验证：`bash -n deploy/init.sh` 通过，旧式裸调用 grep 零命中）
- [x] 2.4 修 `deploy/docker/Dockerfile.demo`：补拷 `pnpm-workspace.yaml`（否则 three patch 被 `--no-frozen-lockfile` 静默丢弃）+ 加 `NODE_OPTIONS=--max-old-space-size=3072`（Pi 构建 OOM，与 Dockerfile 同步）（验证：随 3.6 真机构建通过）
- [x] 2.5 修 `demo/entrypoint.sh` 清场步骤：删已下线的 `narrative_boards`/`narrative_summaries` 硬引用 + 改 DO 块缺表自动跳过（否则 `ON_ERROR_STOP=1` 直接炸→容器无限重启，首次部署实测踩中）（验证：真机导入日志出现 `seed import complete` + `skip missing table` NOTICE）

## 3. 远程一键部署脚本

- [x] 3.1 新增 `scripts/deploy/deploy-remote.sh`（自检 → 本机预编译 → rsync 推送 → 远程 `up --build -d` → `/health` 轮询；`--dry-run`/首参/`DEPLOY_TARGET`/`REMOTE_PATH`/`HEALTH_TIMEOUT`；免密失效输出 `ssh-copy-id` 指引）（验证：`bash -n` 通过 + `--help` 含用法）
- [x] 3.2 新增 smoke `scripts/deploy/deploy-remote.smoke.sh`（4 断言组：语法/帮助/命令拼装含排除清单/免密失效指引，假目标 127.0.0.1 不触真实主机）（验证：`bash scripts/deploy/deploy-remote.smoke.sh` 全过）
- [x] 3.3 真机验证：实跑到 zanebono@10.11.12.59 → `~/software/Syntopica`（替换旧 demo 栈：部署前已 `docker compose down` 拆掉 syntopica-demo 两容器），远端 `/health` 返回 200（本机 `curl http://10.11.12.59:5100/health` = 200；远端 `*.tar` 保留、旧 demo 容器与旧根 compose 已移除）。过程中额外修复/处置：① Dockerfile 补拷 `pnpm-workspace.yaml`+`patches/`（pnpm 10 frozen 校验）；② 前端构建阶段加 `NODE_OPTIONS=--max-old-space-size=3072`（Pi 4 4GB 默认堆 OOM）；③ 远端 swap 100MB→2048MB（dphys-swapfile）
- [x] 3.4 本地生成 demo seed：`cd backend-go && go run ./cmd/dump-sanitizer` → `demo/seed/seed.sql`（159MB / 23.9 万行 / 30 天窗口）（验证：文件存在且非空）
- [x] 3.5 `deploy-remote.sh` 加 `--demo` 模式（本机构建镜像 → save\|gzip\|load 推送 → 远端 run.yml `DEMO_TAG` 拉起 → :5080 健康；seed 缺失前置拦截；tag 默认 latest、旧 tag 不删可回滚）+ smoke 增 demo 断言组；spec delta 补 `remote-push-deploy` Demo 需求（3 场景）（验证：smoke 5 组全过 + `openspec validate` 通过）
- [x] 3.6 真机验证（demo）：远端全量栈已 `compose down`（用户决策：10.11.12.59 只跑 demo）→ `--demo` 实跑成功 → `http://10.11.12.59:5080/health`=200、页面 200、`seed import complete`、文章/分类 API 有数据、容器 `restarts=0` 稳定、旧全量容器 0、远端保留 `v1.3.3-demo` 旧镜像可回滚（验证：7.7）
- [x] 3.7 叙事层补数 + AI 记录清空（2026-09-22 用户验收追加）：dump-sanitizer 白名单补 12 张叙事工坊依赖表（全量无窗口，`topic_enrichment_result.session_id` 清空、3 个 vector 列 NULL 投影）；ai_providers/ai_routes/ai_settings/ai_route_providers 改 `Where: "FALSE"`（记录清空、表结构保留）；修 feeds 窗口 bug（created_at=订阅时间，30 天窗口把 24 订阅滤到 1，改全量）；entrypoint 清场清单同步 12 表（验证：真机 demo 库 12 表行数全对齐 + ai_* 四表=0 + feeds=24）
- [x] 3.8 本地误操作与恢复（已闭合）：验证 entrypoint SQL 时 `psql -c` 误真执行 TRUNCATE CASCADE，清了本地 categories/feeds/articles/article_topic_tags/reading_behaviors/user_preferences/firecrawl_jobs/tag_jobs → 从 `backups/pg-key-20260922-040001.dump` 恢复 5 表（pg_restore `-t` 不带 schema 前缀）、reading_behaviors 从 15:15 seed 回填 511 行（session 已哈希、38 孤儿行弃）、firecrawl_jobs/tag_jobs 可再生队列弃；教训已记 docs/experience（验证：本地 /health 200 + articles=22783）
- [x] 3.9 泳道动态列级缺口修复（2026-09-22 用户验收追加：远程泳道动态主卡区空）：根因=`dump-sanitizer` 的 `daily_report_sections` 列白名单漏泳道归属/追踪列，远程 `persistent_topic_id` 全 NULL → 读路径按「沉寂不展示」过滤后 `lanes: []`（候选栏正常，迷惑性强）。补 7 列：`persistent_topic_id`/`lane_tier`/`watch_id`/`topic_status_at_report`/`topic_match_distance`/`topic_match_confidence`/`quality_breakdown`（该列无物理 FK，不破坏 seed 拓扑序）→ 重生成 seed → `deploy-remote.sh --demo` 重推（远端旧 latest 被 load 重命名前仍可回滚，v1.3.3-demo 保留）（验证：7.8/7.9）

## 4. 引用修复与文档

- [x] 4.1 README.md：初始化脚本/Compose 命令/目录树改新路径（验证：旧路径 grep 仅剩新旧对照表行）
- [x] 4.2 AGENTS.md + `docs/reference/standard/`：核查无需改动（两处只引根目录 `docker-compose.pg.yml`，留根不变）
- [x] 4.3 `docs/reference/deployment.md`：部署方式表/命令约定/init 流程/拓扑/快速部署/回滚全部改新路径；新增「远程推送部署（免密一键）」一节（免密配置 + 一键命令 + 5 形态选型表 + 新旧命令对照）（验证：旧路径 grep 零残留）
- [x] 4.4 全仓 grep 收口：README/AGENTS/docs/reference/scripts/deploy/demo 旧路径零残留（v1.x 历史存档与 openspec changes 历史文档为事实存档不改写；`demo/entrypoint.sh` 注释为通用陈述非路径指引）（验证：6.2）
- [x] 4.5 demo 接入文档：deployment.md 远程推送节补 `--demo` 用法与回滚说明，选型表 demo 行改「远程展示用 --demo」；spec delta 已补 Demo 需求（验证：openspec validate 通过）

## 5. 测试

- [x] 5.1 运行 `bash scripts/deploy/deploy-remote.smoke.sh`，期望 5 断言组全部通过、退出码 0

## 6. 文档

<!-- doc-impact: deployment, standard -->
<!-- doc-impact-excuse: flow=归属集合混入并发 change board-signal-reports 的 front/app/features 文件（误报）；本 change 不触及任何业务 flow 代码 -->
- **无 flow 影响**：本 change 为部署编排/路径重整，不触及任何 `flow/*.md` 业务域（运行时行为不变），按 §12.2 豁免变更溯源链接
- [x] 6.1 部署文档更新：deployment.md（含新「远程推送部署」节）、README、architecture/overview、development、configuration 全部改毕且互相一致；无幽灵指引
- [x] 6.2 旧路径 grep 一致性校验：`grep -rn --include='*.md' --include='*.sh' --include='*.yml' -E 'bash init\.sh|-f docker-compose\.(firecrawl|rsshub)\.yml|docker compose up' README.md AGENTS.md docs/ scripts/ demo/ | grep -vE ':[0-9]+:\| '` → 期望零输出（排除规则：markdown 表格行仅存于 deployment.md 新旧对照表；`docs/v1.x/` 历史存档不改写）
- [x] 6.3 新路径正向校验：`grep -c 'deploy/compose/docker-compose.yml' docs/reference/deployment.md` = 11 ≥ 1；`deploy/init.sh`、`scripts/deploy/deploy-remote.sh` 均存在
- [x] 6.4 `docs/reference/standard/backend/testing.md` §🛑 补 2026-09-22 psql `TRUNCATE…CASCADE` 同类事故硬约束（任务 3.8，standard 域随此补声明）

## 7. 验证

- [x] 7.1 `docker compose --project-directory . -f deploy/compose/docker-compose.yml config` → 渲染成功，postgres 卷落在根 `data/`
- [x] 7.2 `docker compose -f demo/docker-compose.demo.yml config` → 渲染成功（context=仓库根，dockerfile=`deploy/docker/Dockerfile.demo`）
- [x] 7.3 `bash -n deploy/init.sh && bash -n scripts/deploy/deploy-remote.sh` → 均退出码 0
- [x] 7.4 `bash scripts/deploy/deploy-remote.smoke.sh` → 退出码 0、断言全过
- [x] 7.5 6.2 的旧路径 grep → 零输出（按 6.2 排除规则）
- [x] 7.6 `bash scripts/deploy/deploy-remote.sh --dry-run` → 退出码 0，输出含 rsync 排除清单与远程 compose `--project-directory` 命令
- [x] 7.7 demo 真机：`curl http://10.11.12.59:5080/health` → 200；`curl …/api/articles?limit=1` 含真实文章数据；远端 `docker inspect -f '{{.RestartCount}}' syntopica-demo` → `0`；`docker logs syntopica-demo | grep 'seed import complete'` → 命中
- [x] 7.8 泳道动态真机：`GET /api/semantic-boards/{1974,3030,2272}/lane-dynamics?days=14` → lanes=5/3/4 非空，均含 snapshot 与 12-15 天 timeline；1974 的 lane topic_id 集合与本地完全一致 `[2,5,9,1204,1205]`
- [x] 7.9 列回填校验：远端 `daily_report_sections.persistent_topic_id` 非空 1326/1353（缺口=未匹配 section，合法）、`lane_tier` 全非空、`watch_id` 46 行；容器 `RestartCount=0`、日志零 ERROR/panic

### Scenario → 测试文件映射

| Scenario | 测试文件 |
| --- | --- |
| 完整 Firecrawl 栈 | 人工：7.1 compose config 渲染成功 + 4.4 旧路径 grep 零残留 |
| 无需认证 | 人工：`deploy/compose/docker-compose.firecrawl.yml` 配置走查无认证项 |
| Redis 连接 | 人工：7.1 compose 拓扑走查（redis 服务定义保留） |
| 关闭 Firecrawl 不影响 SSR 抓取 | 人工：firecrawl 独立 compose project、与主栈隔离走查 |
| 单独管理 Firecrawl | 人工：4.1/4.5 新旧命令对照表与 deployment.md 选型表一致 |
| Firecrawl 降级为可选增强 | 人工：deployment.md 部署形态选型表 firecrawl 行走查 |
| 用户首次部署 | 人工：7.3 `bash -n deploy/init.sh` 通过 + 4.1 README 新旧命令对照 |
| init.sh 不存在时 | 人工：2.3 `deploy/init.sh` 自定位仓库根 + 缺失分支提示走查 |
| 重复运行 init.sh | 人工：2.3 三阶段幂等走查（`--project-directory` 统一调用） |
| 免密就绪时一键完成部署 | 人工：3.3 真机实跑（远端 `/health` 200、旧 demo 栈替换） |
| 免密未配置时给出可执行指引 | scripts/deploy/deploy-remote.smoke.sh |
| 远程健康检查失败时报告 | 人工：7.6 dry-run 输出 + 3.3 真机健康轮询收敛日志 |
| 目标主机 .env 不被覆盖 | scripts/deploy/deploy-remote.smoke.sh |
| 运行数据不被同步 | scripts/deploy/deploy-remote.smoke.sh |
| demo 一键部署 | 人工：3.6/7.7 真机（`:5080/health` 200、页面 200、`RestartCount=0`） |
| seed 缺失时拒绝构建 | 人工：3.5 seed 前置拦截分支走查（smoke demo 断言组辅证） |
| 远端旧镜像 tag 保留可回滚 | 人工：3.6/7.7 远端 `docker images` 保留 `v1.3.3-demo` 旧 tag |
| 目标主机无 Docker | 人工：`deploy-remote.sh` 自检段 `command -v docker` 分支走查 |
| 镜像内产物不含宿主地址 | 人工：7.1/7.2 compose config 渲染走查 |
| 构建期可覆盖 | 人工：Dockerfile ARG/ENV 覆盖路径走查（2.4/3.3①②） |
| 手工静态产物可判据 | 人工：deployment.md §本地裸跑静态托管产物约定走查（4.3） |
| 部署步骤可复现 | 人工：4.3 文档新旧命令对照 + 7.6 dry-run 实测 |
| 文档与实际部署形态一致 | 人工：4.3/6.1 文档一致性 + 6.2 旧路径 grep 零残留 |
