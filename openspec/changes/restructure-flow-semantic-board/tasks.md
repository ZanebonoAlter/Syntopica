<!-- stage: implement -->

# Tasks

## 1. 搬迁前留底与对账基线

- [ ] 1.1 备份原文到 `/tmp/semantic-board.before.md`
- [ ] 1.2 记录基线：非空白行数、总行数
- [ ] 1.3 记录 20 个高频术语的 grep 计数（`MergeEmbedding`/`composite_hit`/`max_sim`/`suggestion_hash`/`hit_rate`/`L1`/`L2`/`L3`/`watched`/`radar`/`anchor`/`cooldown`/`dismiss`/`story_count`/`visibleAxis`/`rumination`/`correction`/`health`/`triage`/`auxiliary`）
- [ ] 1.4 记录 39 条红线的 `constraint-digest --brief --domains semantic-board` 输出（注入回归基线）

## 2. 块清点与归族

- [ ] 2.1 列出 17 个块的标题、起止行号、性质（rule/decision/mixed）
- [ ] 2.2 按 D1 三判据归 6 族，产出施工图（块 → 族 → 层）
- [ ] 2.3 混合块（尤其 90 行的 `升级建议生命周期`）标出拆句位置

## 3. 重排执行

- [ ] 3.1 「一、标签体系」：合并 5 块（辅助标签入库 / 组合标签 / Event 延迟 embedding / 标签合并预览 / watched tags）
- [ ] 3.2 「二、版块匹配」：SemanticBoard 匹配五级优先
- [ ] 3.3 「三、升级建议」：90 行块拆现状 + 四格矩阵决策记录
- [ ] 3.4 「四、泳道与工作面」：泳道动态 / 左右轨 / 关键事件视图 / 泳道工作面
- [ ] 3.5 「五、版块体检」：体检 / 三病分诊
- [ ] 3.6 「六、版块治理」：管理回填 / 来源构成 / 治理面板 / 相关事件
- [ ] 3.7 每族补 `#### 当前规则` / `#### 决策记录` 两层；无决策记录的写「（暂无）」
- [ ] 3.8 族首补一张 mermaid 流程图（当前链路整体形状）

## 4. 不动的部分

- [ ] 4.1 `## 业务约束与不变量` 节原样保留（节名 + 39 条 + 编号顺序）
- [ ] 4.2 `## 变更溯源` 表原样保留
- [ ] 4.3 `## 需求说明` / `## 代码入口` 基本不动（如需调整只做最小改动）

## 5. 对账

- [ ] 5.1 非空白行数前后相等
- [ ] 5.2 20 个术语 grep 计数前后一致（差异逐个说明）
- [ ] 5.3 `constraint-digest --brief --domains semantic-board` 输出与基线一致（39 条）
- [ ] 5.4 `git diff` 确认红线节无改动
- [ ] 5.5 逐族读一遍：不查别的族能否读懂

## 6. review

- [ ] 6.1 派 review
- [ ] 6.2 处置 review 发现

## 7. 归档

- [ ] 7.1 九项门禁全绿
- [ ] 7.2 `openspec archive restructure-flow-semantic-board --yes`
- [ ] 7.3 `openspec validate --specs` 全过

## 8. 测试

| 验证点 | 方法 | 期望 |
| --- | --- | --- |
| 内容不丢 | 非空白行数对账 | 前后相等 |
| 术语不丢 | 20 个术语 grep 计数 | 一致 |
| 红线不变 | constraint-digest 输出对比 | 39 条，逐条相同 |
| 红线节名未改 | grep `^## 业务约束与不变量` | 命中 1 次 |
| 无 change 名标题 | grep `^### .*(add-\|split-\|unify-\|board-health-triage)` | 零命中 |
| 六族齐全 | 链路设计节的 `^### ` 计数 | 6 个 |
| 两层齐全 | 每族都有 `#### 当前规则` 与 `#### 决策记录` | 6×2 |
| 其他 flow 未动 | `git diff --name-only` | 只有 semantic-board.md |

## 9. 文档

<!-- doc-impact: semantic-board=文档本体重排; standard=doc-authoring 如需补族组织写法 -->

- `docs/reference/flow/semantic-board.md`（本体）
- `docs/reference/standard/shared/doc-authoring.md`：补「链路设计按族组织」的写法约定（若需要）
- `docs/research/process-arch-rework/issues.md`：ISS-14 回填

## 10. 验证

- [ ] 10.1 §8 八项全过
- [ ] 10.2 ISS-14 回填「semantic-board 已落地，其余域待推广 + 推广判据」

## 11. 验收

- [ ] A 查任一族规则不需要跨族翻
- [ ] B 决策记录（坑）一条不丢
- [ ] C 红线注入行为不变（39 条）
- [ ] D 其他 9 个 flow 文档零改动
