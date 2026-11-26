<!-- stage: implement -->

# Tasks

## 1. 根因（先看数据）

- [x] 1.1 查 `session.rollup`：主线程 984 次工具调用 / $44.96，其中 gpt-6.2-sol $38.63（86%）
- [x] 1.2 子线程 25 次派发（多数是 review），MiniMax 合计 $0.015 → 单价差 2600 倍
- [x] 1.3 定性：与刚修掉的 `ui-stage` marker 同病——「让执行者自己判断要不要做」的规范必然被跳过

## 2. 实现

- [x] 2.1 `.pi/extensions/router-discipline.ts`：read >200 行 / 同文件连续 edit ≥3 时 steer 一次
- [x] 2.2 每会话每类信号各一次（低噪声）；换文件重置 streak
- [x] 2.3 全程 fail-open（异常不影响工具执行）
- [x] 2.4 记账 `policy=router-discipline` / `action=warn`
- [x] 2.5 `scripts/harness/router-discipline.sh`：`--excuse` / `--status` / `--clear`
- [x] 2.6 豁免：理由非空才生效（空理由 exit 2）；读到后静音本会话 + 记 `action=bypass` / `reasonCode=router-excused` / `detail=<理由>`

## 3. 验证

- [x] 3.1 探针：无豁免时 read 大文件触发 1 条、连续 3 次 edit 触发 1 条
- [x] 3.2 探针：写豁免文件后同样动作 0 条提醒
- [x] 3.3 `--excuse ""` 报错 exit 2
- [x] 3.4 `--status` / `--clear` 正常
- [x] 3.5 tsc 无新增类型错误
- [x] 3.6 该扩展尚无 smoke 文件——本 change 只做探针验证，smoke 另开（已登记）

## 4. 文档

- [x] 4.1 `开发执行规范.md` §主线程纪律：补五类活的分工表 + 软提醒 + 豁免命令
- [x] 4.2 `check-standards` 350 通过 / 0 失败

## 5. review

- [x] 5.1 派 review
- [x] 5.2 处置 review 发现

## 6. 归档

- [x] 6.1 八项门禁全绿
- [x] 6.2 `openspec archive add-router-discipline-nudge --yes`
- [x] 6.3 `openspec validate --specs` 全过

## 7. 测试

| 验证点 | 方法 | 期望 | 实测 |
| --- | --- | --- | --- |
| 读大文件提醒 | 探针调 tool_call(read 无 limit) | 1 条 | ✓ |
| 连续 edit 提醒 | 探针连续 3 次同 path | 1 条 | ✓ |
| 换文件重置 | 探针 edit(a)×2 → edit(b)×1 | 不触发 | ✓ |
| 豁免静音 | 写豁免文件后重跑 | 0 条 | ✓ |
| 空理由拒绝 | `--excuse ""` | exit 2 | ✓ |
| 不阻断 | 所有探针的 tool 调用 | 全部正常执行 | ✓ |

## 8. 文档

<!-- doc-impact: dev -->

- `docs/reference/开发执行规范.md` §主线程纪律（4.1）
- `docs/research/process-arch-rework/issues.md`：ISS-06 回填（见 9.2）

## 9. 验证

- [x] 9.1 探针六项全过（见 §7 表）
- [x] 9.2 ISS-06 回填「已落地 + 判据」到 ledger

## 10. 验收

- [x] A 两个信号可观测、阈值可调、每会话各一次
- [x] B 豁免必须带理由、进事实库、per-session 不永久
- [x] C 零阻断：任何提醒都不改变工具执行结果
- [x] D 五类活的分工表进规范
