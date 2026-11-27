<!-- stage: implement -->

# Tasks

## 1. 基线

- [x] 1.1 备份原文到 `/tmp/daily-report.before.md`
- [x] 1.2 记录非空白行数与 20 个术语 grep 计数
- [x] 1.3 记录 `constraint-digest --brief --domains daily-report` 输出（73 条红线基线）

## 2. 块清点与归族

- [x] 2.1 列出 14 块标题、起止行号、性质（rule/decision/mixed）
- [x] 2.2 按 3 族归位（生成与匹配 / 对外契约与阅读 / 运维与补档）
- [x] 2.3 混合块标出拆句位置（尤其「日报生成管线」37 行）

## 3. 重排执行

- [x] 3.1 族一「生成与匹配」：生成管线 / 锚定机制 / 事件线归属 / 今日重点
- [x] 3.2 族二「对外契约与阅读」：阅读台 / 线索推荐 / 话题盯盘 / Section 可视化
- [x] 3.3 族三「运维与补档」：backfill 端点 / 缺口补填 / 泳道结算 / 关键事件注记 / 候选读取
- [x] 3.4 每族补 `#### 当前规则` / `#### 决策记录`
- [x] 3.5 去掉「后端：」「前端：」前缀与 change 名主体
- [x] 3.6 图随块搬入，不新增

## 4. 不动的部分

- [x] 4.1 `## 业务约束与不变量` 73 条逐字保留
- [x] 4.2 `## 代码入口` / `## 变更溯源` / `## 需求说明` 保留

## 5. 对账

- [x] 5.1 正文行零丢失（缺失行全部可归因为标题层级或有意拆分）
- [x] 5.2 20 个术语 grep 计数一致
- [x] 5.3 constraint-digest 输出逐字相同
- [x] 5.4 四节逐字相同

## 6. review

- [ ] 6.1 派 review
- [x] 6.2 处置 review 发现

## 7. 归档

- [ ] 7.1 九项门禁全绿
- [ ] 7.2 `openspec archive restructure-flow-daily-report --yes`
- [ ] 7.3 `openspec validate --specs` 全过

## 8. 测试

| 验证点 | 方法 | 期望 |
| --- | --- | --- |
| 内容不丢 | 正文行对账 | 缺失行全部可归因 |
| 术语不丢 | 20 术语 grep | 一致 |
| 红线不变 | constraint-digest 对比 | 73 条逐字相同 |
| 红线节名 | grep `^## 业务约束与不变量` | 1 次 |
| 无技术层前缀 | grep `^#{3,6} .*后端：\|前端：` | 0 |
| 无 change 名标题 | grep `^#{3,6} .*(add-\|retire-\|widen-\|rework-\|unify-)` | 0 |
| 3 族 | 链路设计节 `^### ` | 3 |
| 两层 | `^#### 当前规则\|^#### 决策记录` | 6 |
| 其他 flow 未动 | git diff --name-only | 只有 daily-report.md |

## 9. 文档

<!-- doc-impact: daily-report=文档本体重排; standard=flow-restructure spec 泛化为多文档约定 -->

- `docs/reference/flow/daily-report.md`（本体）
- `openspec/specs/flow-restructure/spec.md`（经 delta MODIFIED 泛化）
- `docs/research/process-arch-rework/issues.md`：ISS-14 补记

## 10. 验证

- [x] 10.1 §8 九项全过
- [x] 10.2 ISS-14 更新推广进度（2/4 达标域已改）

## 11. 验收

- [x] A 查任一族规则不跨族
- [x] B 决策记录不丢
- [x] C 73 条红线注入不变
- [x] D 其他 8 个 flow 文档零改动
