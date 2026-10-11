
## retro ④ 段分类改造落点

retro ④ 段分类改造的全部落点（harness-retro.sh 为 bash 内嵌 python 渲染 + SQL 组装）：

1. soft 聚合 SQL（scripts/harness/harness-retro.sh L580-585）：`'soft', json_object('threshold',…, 'groups_total', (SELECT COUNT(*) FROM warns), 'warn_total', (SELECT IFNULL(SUM(n),0) FROM warns), 'items', (SELECT json_group_array(…) FROM (SELECT * FROM warns WHERE n >= ${WARN_THRESHOLD} ORDER BY n DESC)))`——warns CTE 按 policy+reason 分组计数（定义在 SQL 前段，实现时向上找）。分类改造：items 加 `AND reason NOT IN (白名单)`；新增 informative 分组数组（无阈值过滤）与 warn_total_corrective/informative
2. ④ 段渲染（L987-994）：`out.append('④ 软提醒失效（同 policy+reasonCode 的 warn 计数 ≥ %s）' % soft['threshold'])` + 失效组数行 + items 循环——标题加「纠正型」、名单后加情报型汇总行
3. metrics 键组装（L768-782 python dict）：`'soft.stale_groups': len(soft['items'])`（L776）、`'soft.warn_total': soft['warn_total']`（L777）——stale_groups 语义自然收窄（items 已只含纠正型），新增三键插同处
4. 白名单常量放 bash 头部常量区（L33-45 附近，HOTSPOT_DEFAULT_THR 旁），SQL 内插值展开；注释锚定：情报型=归属/状态告知不期待服从，与 harness-facts reasonCode 词汇表同步；白名单外缺省纠正型（最强监督）
5. smoke（harness-retro.smoke.sh）：A1 L330 检查 `"④ 软提醒失效"` 字符串（标题变需同步）；L557-558 `--warn-threshold 0` 拒绝断言不动；fixture 库 FB 插 policy.decision 事件的地方加两组 warn（参照既有 fixture 插入模式）
6. 文档两处：.agents/skills/harness-retro/SKILL.md L50（④ 段解读表）+ L65（指标名列表）；docs/reference/harness/pi-extensions.md 变更溯源表（L139 附近表尾加行）。L53/L56 是历史叙述不改
7. 基线比对缺键行为：实现时验证 retro-baseline.json（2026-09-27 已落旧语义）读入路径——比对遍历当前 metrics 输出 delta，旧基线无新键时的呈现按既有缺键逻辑确认，必要时对新键标「新增无基线」
8. 真库预期（2026-09-27 快照）：warn 265 条 = concurrent-mixed 239 + concurrent-dirty-tree 16 + foreign-breakage 10，全部情报型 → 失效名单 3→0；台账分布证据：45% mixed 来自 decouple-backend-domains 单 session（01a0dbcb…107 次）

<!-- pinned 2026-09-28T01:37:42Z -->
