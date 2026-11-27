# Tasks

## 1. 索引瘦身

- [x] 1.1 备份原文件（`/tmp/constraints-index.before.md`，6032 B）
- [x] 1.2 重写索引：只留执行规范表（压缩 8→5 行）、域表（10 个域）、流程规范指针
- [x] 1.3 头部写明「这是路由表不是约束正文」+ 机制说明指向 AGENTS.md 与 §13
- [x] 1.4 补漏掉的域 `research-data-sources`（有「业务约束与不变量」节却没进清单）

## 2. 手册搬进开发执行规范

- [x] 2.1 §13 标题改为「change 头部声明与图表达（stage / 域 / diagrams marker）」
- [x] 2.2 三种头部声明对照表（写在哪 / 作用 / 缺失后果）
- [x] 2.3 `stage.md` 非 schema 制品说明 + 载体样例路径
- [x] 2.4 域声明两条规则（两处并集、stage.md 只增不减）+ 域名定义
- [x] 2.5 取红线命令 + `--brief` 原样粘的提醒
- [x] 2.6 `stage.ts` 的 `ROUTING_DOC_CATALOG` 节名同步

## 3. 顺带修

- [x] 3.1 4 个 main spec 补 `<!-- domain: harness -->`（check-standards 警告清零）

## 4. review

- [x] 4.1 派 `change-review` 审（实测确认：旧版每条信息都有归宿、路由链路真接通）
- [x] 4.2 处置 7 条（含 R-2 三份副本归一、R-6 修了个既存 catalog 漂移）

## 5. 归档

- [x] 5.1 九项门禁全绿
- [ ] 5.2 执行归档命令
- [ ] 5.3 `openspec validate --specs` 全过

## 6. 测试

| 验证点 | 方法 | 期望 |
| --- | --- | --- |
| 体积 | `wc -c docs/reference/constraints-index.md` | ≤ 3000 B（原 6032） |
| 域清单完整 | 对比 `flow/` 下含「业务约束与不变量」的文档 | 10 个域全覆盖，无死链 |
| 手册已移走 | `grep '<!-- stage:' constraints-index.md` | 无格式说明（只允许一处指向 §13 的指针） |
| §13 有格式 | `grep '两处并集\\|只增不减' 开发执行规范.md` | 命中 |
| 路由节名对齐 | `grep '13\\. change' .pi/extensions/lib/stage.ts` 与文档标题比对 | 完全一致 |
| smoke 不破 | `bash .pi/extensions/tests/run-smoke.sh`（这个才构建并跑 constraint-injection；`run-harness-smoke.sh` 不覆盖它） | 1078 绿 / 1 红（台账项） |
| catalog 无漂移 | 逐条比对 `path` 存在 + `section` 命中 | 0 漂移（顺手修了 test-design.md 的既存漂移） |
| 标准检查 | `bash scripts/harness/check-standards.sh` | 0 失败 0 警告 |

## 7. 文档

<!-- doc-impact: harness=索引瘦身与头部声明格式迁移 -->

- `docs/reference/constraints-index.md`（1.x）
- `docs/reference/开发执行规范.md` §13（2.x）
- `.pi/extensions/lib/stage.ts`（2.6）

## 8. 验证

- [x] 8.1 §6 七项全过（体积 2718 B、域 10/10、`run-smoke.sh` 1078 绿 / 1 红台账项、标准 360/0/0）
- [x] 8.2 实际注入验证：注入块仍含 `### constraints-index.md`，内容为新版

## 9. 验收

- [x] A 索引只回答「去哪找」：保留单行格式骨架，操作说明全移走
- [x] B 实测接通：`explore`/`propose` 的 required 含 `开发执行规范.md`，catalog 节名逐字对齐
- [x] C 不再重复：AGENTS.md 讲机制、§13 讲格式、索引只给路标（§0.6 的两段也压成指针）
