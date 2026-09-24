<!-- complexity: simple -->
<!-- ui-impact: none -->
<!-- constraint-domains: 无（纯部署/运维脚本 change，不涉业务域代码） -->

## Why

演示树莓派 4（`10.11.12.59:5080`）跑的是脱敏 seed 快照，`demo/README.md` 自己要求「定期重新导出 seed 以保证内容新鲜」，但现状是三步全手动（dump-sanitizer 导出 → deploy-remote.sh --demo 构建推送 → 演示机重导），实际从没人定期做，演示内容逐渐陈旧。且手动流程里「人肉检查 seed 无敏感数据」是一道隐形安全关卡——一旦无人值守自动化，这道关卡必须变成机械断言，宁可演示数据旧一周，也不能把带 api_key 的 dump 推出去。

## What Changes

1. **每周定时同步 wrapper**：新增 `scripts/deploy/sync-demo-weekly.sh`，串联既有链路 `dump-sanitizer`（从本机真库脱敏导出）→ 安全断言 → `deploy-remote.sh --demo`（构建含新 seed 的镜像 → save/load 推送 → 拉起 → health）。不修改 deploy-remote.sh 本身，只做调用方。
2. **安全断言机械化**：导出后、推送前对 `demo/seed/seed.sql` 做 grep 断言——出现 `INSERT INTO ai_call_logs`、`INSERT INTO schema_migrations`、`ai_providers` 行 api_key 非空，三者任一命中即中止全流程，不推送任何文件。断言规则来自 `demo/seed/README.md` 的 Security review 条目。
3. **seed 归档轮换**：`demo/seed/` 下按日期归档历史 seed，只保留最近 2 份作为回滚备用（gitignore 范围内，不占库）；真库在手随时可重导，归档仅是快速回滚手段。
4. **用户级 systemd timer**：每周一次凌晨触发（避开每日 04:00 备份窗口），`Persistent=true` 错过补跑，日志只进 journal，不集成项目内通知渠道——纯个人维护演示环境。
5. **文档**：`docs/reference/deployment.md` 远程推送部署节补「定时同步」小节。

## Capabilities

### New Capabilities

- `demo-seed-sync`: 演示环境 seed 数据的定时同步行为契约——安全断言门禁（断言不过不得推送）、seed 归档保留策略（最近 2 份）、失败中止语义（任一环节失败即停止，不做半程推送）。

### Modified Capabilities

（无——不触碰任何既有 spec 的行为要求）

## Impact

- **新增**：`scripts/deploy/sync-demo-weekly.sh`（编排 + 断言 + 归档逻辑）、`~/.config/systemd/user/` 下 service + timer 单元各一（单元文件入库存档于 `scripts/deploy/systemd/`，安装为手动 `cp` + `enable`）。
- **不改**：后端/前端零代码改动；`deploy-remote.sh` / `dump-sanitizer` / `demo/entrypoint.sh` 均为被调用方，逻辑不动。
- **文件系统**：`demo/seed/` 新增归档文件布局（`seed-YYYYMMDD.sql` × ≤2）；演示机无新落盘文件（seed 仍打进镜像）。
- **运维面**：本机（开发树莓派）新增一个用户级定时任务；演示机完全被动，无需任何改动。
