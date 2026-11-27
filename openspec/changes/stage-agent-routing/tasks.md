# Tasks

## 1. 六档定义

- [x] 1.1 `stage-explore.md`（只读 / gpt-6.1-terra / high）
- [x] 1.2 `stage-propose.md`（读+写制品 / gpt-6.1-terra / high）
- [x] 1.3 `stage-design.md`（读+写+bash / glm-5.4 / max）
- [x] 1.4 `stage-implement.md`（全工具 / deepseek-v4.1-flash / max）
- [x] 1.5 `stage-verify.md`（只读+bash / gpt-6.1-terra / max）
- [x] 1.6 `stage-archive.md`（读+bash / glm-5.4 / high）
- [x] 1.7 六个 frontmatter 均含 `name` / `description` / `model` / `thinking` / `tools`（缺 name 会静默不注册）

## 2. 路由脚本

- [x] 2.1 `scripts/harness/stage-agent.sh <change>` 读 stage.md 输出档名
- [x] 2.2 `--explain` 附理由、`--cheap` 给降本建议
- [x] 2.3 fail-open：无 stage.md / marker 缺失 / 取值非法 → `general-purpose` + exit 0
- [x] 2.4 只读文件头 20 行，避免正文提到 stage 字样被误读

## 3. 文档

- [x] 3.1 `AGENTS.md` 子线程段补六档表 + 脚本用法
- [x] 3.2 `docs/reference/开发执行规范.md` §主线程纪律 补阶段→档映射

## 4. review

- [x] 4.1 派 review（change-review 代做：stage-verify 当时无 write，见 review R-2）
- [x] 4.2 处置 10 条发现（3 High 全修，R-1 白名单 / R-2 verify 落盘 / R-3 cheap 语义）

## 5. 归档

- [x] 5.1 九项门禁全绿
- [ ] 5.2 执行归档命令
- [ ] 5.3 `openspec validate --specs` 全过

## 6. 测试

| 验证点 | 方法 | 期望 |
| --- | --- | --- |
| 六档注册 | `discoverAgents(cwd,'both','commandcode')` | 6 个 stage-* 均出现且 model 正确 |
| 六阶段路由 | 临时 change 逐个写 marker 跑脚本 | 输出 `stage-<阶段>` |
| 边界兜底 | 空 stage.md / `bogus` | `general-purpose` + exit 0 |
| cheap 模式 | `--cheap` on verify | `general-purpose` + 理由 |
| 跨供应商实派 | 派 `stage-verify` 问模型 | 报 `gpt-6.1-terra`（与主会话 commandcode 不同家） |
| 权限收窄 | 读 stage-explore 的 tools | 无 write/edit/bash（smoke 断言） |
| 冒烟 | `bash scripts/harness/stage-agent.smoke.sh` | 38/38 通过 |

## 7. 文档

<!-- doc-impact: harness=AGENTS.md 与开发执行规范 的子线程档位段改写 -->

- `AGENTS.md`（3.1）
- `docs/reference/开发执行规范.md`（3.2）

## 8. 验证

- [x] 8.1 §6 六项全过（跨供应商实派已确认：环境变量 `PI_MODEL=gpt-6.1-terra`）
- [x] 8.2 旧档回归：`change-review` 本轮成功派发并写盘（stage-verify 修复前由它代做审查）

## 9. 验收

- [x] A 换阶段只改 stage.md 一行
- [x] B 跨供应商天然成立（六档横跨 codex / zai / commandcode 三家，实派确认 `PI_MODEL=gpt-6.1-terra`）
- [x] C explore 真无写权限；verify 有 write（仅 review.md）+ bash（纪律），档位正文如实说明
- [x] D legacy change 行为不变（无 stage.md → general-purpose，smoke 断言）
