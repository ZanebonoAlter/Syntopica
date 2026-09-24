## Context

演示树莓派 4（`10.11.12.59:5080`）已有一键部署链路 `scripts/deploy/deploy-remote.sh --demo`（本机构建含 seed 的镜像 → docker save/load 推送 → 拉起 → 健康检查，SSH 免密零交互），seed 由 `backend-go/cmd/dump-sanitizer` 从本机真库脱敏导出（~160MB，gitignore）。缺的只是无人值守的外壳。See proposal.md - Why。

## Goals / Non-Goals

**Goals:**

- 每周自动跑通「导出 → 安全断言 → 部署」全链路，演示机代码+数据同步跟上开发机
- 把人肉安全检查（`demo/seed/README.md` Security review 三条目）固化为机械断言门禁
- seed 留最近 2 份归档作快速回滚手段

**Non-Goals:**

- 不改 `deploy-remote.sh` / `dump-sanitizer` / `demo/entrypoint.sh` 任何既有逻辑（纯调用方）
- 不做增量数据同步（不做 seed 解耦挂载方案——见 D1）
- 不集成任何外部通知渠道（journal 躺尸，个人维护环境）
- 演示机不做任何改动（纯被动接收方）

## Decisions

### D1: 定时串联现有链路（方案 A），不做 seed/镜像解耦（方案 B）

备选 B（run.yml 挂载 seed.sql + rsync 增量 + entrypoint 优先读挂载路径）传输更轻，但用户诉求是「代码+数据都跟上」——演示环境展示最新版本，每次重推镜像本来就是期望行为；且 B 要动 demo 编排三件套，改动面反而大。A 只是新增调用方，现有链路的回滚机制（旧 tag 不删、`DEMO_TAG` 回滚）原样继承。

### D2: 用户级 systemd timer（非 cron、非系统级）

- vs cron：`Persistent=true` 错过窗口（关机/重启）恢复后自动补跑——这是 spec 硬要求（`错过窗口后补跑` 场景），cron 做不到；日志天然进 journal，与「无外部通知」决策对齐。
- vs 系统级：用户级跑在用户 session 内，`deploy-remote.sh` 依赖的 SSH 免密（`~/.ssh/`）与 docker 权限直接可用，无需处理 key 路径/权限映射。
- 前置：需 `loginctl enable-linger <user>` 一次（不登录也跑用户级 timer）。单元文件入库存档于 `scripts/deploy/systemd/`，安装为手动 `cp` 到 `~/.config/systemd/user/` + `systemctl --user enable --now`。

### D3: 触发时刻 = 每周日 05:00 + 随机延迟 15 分钟内

避开每日 04:00 备份窗口（留 1 小时余量收尾），`RandomizedDelaySec=15min` 防与其他周期任务对齐。具体时刻是单元文件里的一个字段，调整无需动脚本。夜间跑使镜像构建的 CPU 满载期（Pi 4 核数分钟）无人感知。

### D4: 安全断言为 wrapper 内前置函数，fail-closed

三断言（无 `ai_call_logs` INSERT、无 `schema_migrations` INSERT、`ai_providers.api_key` 全空）逐项 grep；任一命中即打印命中项并退出非零，**不进入部署环节**。断言模式以 `dump-sanitizer` 实际输出格式为准（实现时先跑一次导出对照真实文件校准正则，见 tasks）。格式将来若漂移，断言失败方向是「中止同步」而非「放行」——fail-closed，安全方向正确。

### D5: 归档轮换 = 日期命名 + 保留 2 份

每次成功导出后复制为 `demo/seed/seed-YYYYMMDD-HHMM.sql`，`ls -t` 排序删超出 2 份的最旧者（gitignore 已覆盖 `demo/seed/` 下非 README 文件，实现时核对）。真库在手随时可重导，归档仅是省去重导的快速回滚手段，不值得多留。

### D6: 防呆前置 = 负载 + 磁盘双阈值，跳过不算失败

触发时检查 load average（>4 跳过，Pi 4 核满载线）与 `demo/seed/` 所在盘余量（<5GB 跳过，seed 160MB + 镜像 tar 数百 MB 需要余量）。跳过以退出码 0 结束并记 journal 原因——演示环境保持现版本，下周期自愈，不该触发任何告警语义。

## Risks / Trade-offs

- [Pi 夜间构建与其他任务撞车（备份延长、其他 pi 会话）] → D3 错开 1 小时 + D6 负载阈值跳过；备份窗口若实际超 1 小时，调 `OnCalendar` 即可
- [断言正则与 dump 格式漂移导致误中止（同步停摆但演示机还在跑旧版）] → fail-closed 方向安全；停摆会在下次人工查看 journal 时发现（个人环境可接受），断言函数与 README 三条目一一对应，改 dump-sanitizer 时有 README 对齐提醒
- [导出 160MB 对真库瞬时 SELECT 压力] → 只读查询、Pi postgres 配置虽小但单用户库无并发竞争；夜间低负载时段执行
- [演示机刷新期间（TRUNCATE+导入约几分钟）页面空态] → 周日 05:00 深夜无人观看，spec 已接受此窗口
- [SSH 免密失效（换 key / 演示机重装）] → `deploy-remote.sh` 既有自检会在部署环节非零退出，journal 可见，不做半程推送——失败语义已有保障

## Migration Plan

部署（一次性手动）：`cp scripts/deploy/systemd/* ~/.config/systemd/user/` → `loginctl enable-linger $USER` → `systemctl --user daemon-reload && systemctl --user enable --now sync-demo-seed.timer`。

回滚/卸载：`systemctl --user disable --now sync-demo-seed.timer` + 删单元文件；链路本身无状态，卸载即完全恢复原状。

## Open Questions

（无——触发时刻默认周日 05:00 可后续调单元文件；断言正则实现时对照实际导出校准，均不改变 spec 与任务结构）
