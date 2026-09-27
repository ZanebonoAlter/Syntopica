## 1. 断言校准（对照真实导出）

- [x] 1.1 跑一次 `cd backend-go && go run ./cmd/dump-sanitizer` 生成真实 `demo/seed/seed.sql`，人工确认三条安全条目在文件中的实际文本形态（`ai_call_logs`/`schema_migrations` 的 INSERT 语句格式、`ai_providers` 行 api_key 列的空值表示），据此校准断言 grep 模式。验证：`test -s demo/seed/seed.sql && stat -c%s demo/seed/seed.sql` → 文件存在且 >0 字节
- [x] 1.2 从真实导出裁剪/篡改出三个毒样本 fixture（含 `INSERT INTO ai_call_logs`、含 `INSERT INTO schema_migrations`、`ai_providers` 行 api_key 非空，各一个，几 KB 即可）存入 `scripts/deploy/tests/fixtures/`。验证：`ls scripts/deploy/tests/fixtures/` → 三个 .sql 文件存在

## 2. 同步脚本 `scripts/deploy/sync-demo-weekly.sh`

- [x] 2.1 骨架与防呆前置：`set -euo pipefail`；阶段化日志（每阶段一行状态，journal 友好）；触发时检查 load average（>4 跳过）与 `demo/seed/` 所在盘余量（<5GB 跳过），跳过记原因、退出码 0。验证：`bash -n scripts/deploy/sync-demo-weekly.sh` → 无语法错误；临时把负载阈值调至 0.1 后运行 → 输出跳过原因并以退出码 0 结束
- [x] 2.2 安全断言函数：三断言逐项 grep `demo/seed/seed.sql`，任一命中打印命中项并退出非零。验证：对 1.2 的三个毒样本分别执行断言函数 → 各自非零退出且打印对应命中项；对干净真实导出执行 → 退出码 0
- [x] 2.3 归档轮换：断言通过后将 seed 复制为 `demo/seed/seed-YYYYMMDD-HHMM.sql`（断言失败不归档——毒样本不占回滚位），只保留最近 2 份。验证：预置 3 份伪造旧归档后执行轮换 → `ls demo/seed/seed-*.sql | wc -l` 输出 2，且保留的是最新的两份
- [x] 2.4 串联主链路：防呆 → `dump-sanitizer` 导出 → 断言 → 归档 → `bash scripts/deploy/deploy-remote.sh --demo`；任一环节非零退出即终止后续环节。验证：注入断言失败（临时用毒样本替换 seed.sql 路径参数）运行 → 日志止于断言阶段、无 deploy-remote 调用、退出码非零
- [x] 2.5 确认 `.gitignore` 覆盖 `demo/seed/seed-*.sql` 归档命名（现有规则只覆盖 `seed.sql` 则补一行）。验证：`git check-ignore -v demo/seed/seed-20990101-0000.sql` → 输出匹配的 ignore 规则

## 3. systemd 用户级单元

- [x] 3.1 编写 `scripts/deploy/systemd/sync-demo-seed.service`（Type=oneshot，ExecStart 指向同步脚本）与 `sync-demo-seed.timer`（OnCalendar=周日 05:00、Persistent=true、RandomizedDelaySec=15min）。验证：`systemd-analyze verify scripts/deploy/systemd/sync-demo-seed.{service,timer}` → 无错误输出
- [x] 3.2 安装启用：`cp scripts/deploy/systemd/* ~/.config/systemd/user/` → `loginctl enable-linger $USER` → `systemctl --user daemon-reload && systemctl --user enable --now sync-demo-seed.timer`。验证：`systemctl --user list-timers | grep sync-demo-seed` → 显示下次触发时间（周日 05:00 后）

## 4. 端到端首次运行

- [x] 4.1 手动触发一次全链路并逐项核对。人工：`systemctl --user start sync-demo-seed.service` 后，`journalctl --user -u sync-demo-seed.service -n 50` 看到「导出→断言→归档→部署→health」各阶段完成日志；`curl --noproxy '*' http://10.11.12.59:5080/health` 返回 200；`ls demo/seed/seed-*.sql` 出现当日归档；演示机页面数据为最新导出内容

## 5. 测试

- 纯 shell 运维脚本 change，无产品代码：断言函数与归档轮换的行为验证已内嵌于 2.2（毒样本三连）、2.3（伪造归档轮换）、2.4（断言失败熔断）的任务验收，均为「命令+期望」形态，不另立测试文件。

## 6. 文档

<!-- doc-impact: deployment -->

- [x] 6.1 `docs/reference/deployment.md` 「远程推送部署」节补「定时同步」小节：安装/卸载步骤、`journalctl --user -u sync-demo-seed` 查日志、调触发时刻改 OnCalendar、防呆跳过语义。验证：`grep -n 'sync-demo-seed' docs/reference/deployment.md` → 至少 3 处命中（安装/日志/调时刻）
- [x] 6.2 `demo/README.md` 「定期重新导出 seed」相关措辞指向新定时机制（手动一次性刷新仍保留为备选路径）。验证：`grep -n 'sync-demo-weekly\|定时' demo/README.md` → 至少 1 处命中

## 7. 验证

- [x] 7.1 `openspec validate auto-sync-demo-seed --strict` → 期望：无违例输出（退出码 0）
- [x] 7.2 `bash -n scripts/deploy/sync-demo-weekly.sh && bash -n scripts/deploy/deploy-remote.sh` → 期望：两脚本均无语法错误
- [x] 7.3 `systemd-analyze verify ~/.config/systemd/user/sync-demo-seed.{service,timer}` → 期望：无错误输出
- [x] 7.4 `systemctl --user list-timers --no-pager | grep sync-demo-seed` → 期望：一行含下次触发时间（周日）
- [x] 7.5 毒样本断言回归：对四个 fixture 逐个执行断言逻辑（rsshub-host 需 `RSSHUB_REWRITE='rss.example.internal:1200=public'`）→ 期望：各非零退出且分别打印 `ai_call_logs` / `schema_migrations` / `api_key` / `rsshub-host` 命中项
- [x] 7.6 `journalctl -t sync-demo-weekly.sh --since "-7 days" | grep -c '阶段'`（等效阶段标记；`--user` 视角受本机 journald 用户分文件缺失影响，见 deployment.md 查日志节）→ 期望：≥5（导出/断言/归档/部署/health 各阶段至少一条记录）

### Scenario 对账

| Scenario | 测试文件 |
| --- | --- |
| 正常周期触发 | 人工：systemctl --user start 全链路后 journal 阶段日志加 health 200，4.1 已核 |
| 错过窗口后补跑 | scripts/deploy/systemd/sync-demo-seed.timer |
| 资源不足时跳过本次 | scripts/deploy/sync-demo-weekly.sh |
| seed 含调用日志 | scripts/deploy/tests/fixtures/poison-ai-call-logs.sql |
| api_key 未清空 | scripts/deploy/tests/fixtures/poison-ai-providers.sql |
| 改写源 host 残留（2026-09-24 泄露事故后新增） | scripts/deploy/tests/fixtures/poison-rsshub-host.sql |
| 断言全部通过 | 人工：assert_seed_safe 对 demo/seed/seed.sql 退出 0（gitignore 文件，命令加期望） |
| 轮换删除最旧归档 | scripts/deploy/sync-demo-weekly.sh |
| 归档可用于回滚 | 人工：归档副本替换 seed.sql 后重跑部署链路 |
| 导出失败即停 | 人工：阶段失败即 die 的串联语义已在 2.4 注入验证 |
| 部署失败保留旧版 | 人工：deploy-remote.sh 既有自检失败非零退出加旧 tag 不删 |
