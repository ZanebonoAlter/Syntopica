# Design: fix-board-ghost-false-positive

## Context

方向校验现状（见 proposal Why）：绝对门 0.6 切在 qwen3-embedding「具体事件 × 宽泛类目」天然 cosine 分布带（0.50-0.65）上，全库 23.5% 挂载被标 mismatch，其中 55% 为全库最优挂载（冤案），且 mismatch 挂载被排除出日报、计入体检 ghost 病征。垃圾与冤案同分布带，绝对阈值不可校准（用户实测 0.5 放进大批垃圾）。判定逻辑唯一实现：`backend-go/internal/tagmanagement/service/board/semantic_board_matching.go` 的 `directionMismatch` 闭包（绝对门，L386-395）+ `applyDirectionRelativeGate`（相对门后置遍历，L485-529）；匹配循环已全量加载 `boardEmbeddings`（全部 active 版块 embedding map）。

真库验证数据（2026-10-16 会话，作为判定结构依据）：

| 切分 | 行数 | 抽样成色 |
| --- | --- | --- |
| 全库最优 mismatch（豁免组） | 671 (55%) | 10/10 + 低分段 11/11 正确归属（Cursor→开发者工具 0.731、高市早苗→日本新闻 0.5826、Tokenization→生成式AI 0.515） |
| 非最优 mismatch（继续拦组） | 543 (45%) | 15/15 真串味（均有 0.76-0.87 明显真家在别处） |
| 全库最优挂载 cos 分段 | <0.45: 0 行，0.45-0.50: 0 行，0.50-0.55: 11 行（全对） | 地板线当前无操作量 |

## Goals / Non-Goals

**Goals**
- mismatch 判定从「双门一刀切」改为「composite_hit 豁免 → 全库最优豁免 → 绝对门 → 相对门」四层次序
- 全库最优参照系 = 全部 active 版块（含本次匹配候选外），纯内存计算，不引入新 IO
- 存量 direction_mismatch 全量重算（复用 rematch-all）
- 消费方（日报过滤 / boardhealth ghost / 前端展示 / match detail API）零改动

**Non-Goals**
- 口径②「多数暴政」（多元版块少数子主题构成标签必然垫底，如靖国神社）——体检评分语义问题，留给后续 change
- 体检可解释性下钻（幽灵明细、判定理由展示）——UI change 另议
- 绝对门/相对门阈值本身的调整（0.6 / 0.15 保持不动，只收窄作用域）
- 日报 ExclusiveGap 跨版块排他逻辑与方向校验的统一（哲学同源但本次不动）

## Decisions

### D1: 全库最优参照系用「全部 active 版块」而非「同轮匹配候选」

相对门（fix-digest-tag-leak）用同轮候选做参照是合理的：它抓的是「同轮多挂中明显偏弱的」。但豁免判定若用同轮候选，弱关联单挂垃圾（真家在候选外，如巴拿马地震 × 美国新闻 > × 日本新闻，本次只匹配上日本新闻）会被误豁免——候选内它是最优（唯一）。真库 543 行继续拦组的形态（真家 0.76-0.87 多为候选外版块）证明参照系必须扩大到全部 active 版块。两参照系各司其职：豁免层管「归属正确性」，相对门管「同轮串味」。

实现上零成本：`boardEmbeddings` map 本就全量加载，豁免判定只需对 map 内全部版块算一遍 tag identity cosine 取最大值。

*备选否决*：① 用同轮候选做参照——单挂垃圾误豁免（上述）；② 降绝对门到 0.5——用户实测放进大批垃圾，分布带重叠不可校准。

### D2: 豁免优先级高于双门，判定次序重排为四层

现实现里绝对门是 per-board 即时闭包（匹配循环内调用）、相对门是循环后后置遍历。新结构需要在绝对门判定前知道「全库最优集合」，因此把全库最优计算提前到循环前（tag identity embedding 就绪后对 boardEmbeddings 全量算一遍，得 bestCos_global 与最优版块集合），绝对门闭包改为查集合：挂载版块 ∈ 最优集合 → 免标。相对门后置遍历保持不变，但同样跳过最优集合内的挂载（最优者 gap 语义上恒通过，跳过是显式化而非行为变化）。

*备选否决*：把豁免做成消费方各自过滤（体检/日报各自排除「全库最优」行）——flag 落库列仍是脏数据，新增消费方会再次踩坑，且日报查询需要 join 全库 embedding 计算不可行。

### D3: 地板线做成配置（默认 0=关闭）而非硬编码

真库全库最优挂载最低 cos 0.515 且成色正确，硬编码任何 >0.5 的地板都会产生新误伤面。配置 key `semantic_board_match_direction_floor` 默认 0，与现有 13 个匹配参数同管道（LoadConfig ai_settings 读取）。防御目标是未来「哪都不像却硬挂」形态（embedding 模型更换后可能出现）。

### D4: 存量重算复用 rematch-all，不写迁移脚本

direction_mismatch 是匹配过程的派生列，rematch-all（`semantic_board_backfill.go` all 模式）重算挂载时自然重写该列。顺带修复发现的存量陈旧 flag（Cursor 0.731 全库最优却标 mismatch——embedding 重生成后 flag 未重算的历史遗留）。上线后手动触发一次，验证口径：mismatch 总数 1214 → 预期 ~543 ±（陈旧 flag 修正带来的少量额外豁免）。

*备选否决*：SQL 迁移脚本按 embedding 现算回填——把判定逻辑复制进 SQL 两处维护，漂移风险大于收益。

### D5: tag identity embedding 缺失语义保持「跳过=不标」

现有语义（缺 embedding 跳过门、mismatch=false）不变。豁免层同理：无法算 cosine 就无法判最优，维持 false。避免「证据缺失」被误读为「方向不符」——与 boardhealth 对 insufficient 的处理哲学一致。

## Risks / Trade-offs

- [豁免组混入人眼可见的边缘内容（如欧元兑日元→日本新闻类外汇宏观）] → 属于「全库最优且用户未配置拦截」的合法形态，交给既有治理手段（版块构成调整 / 人工摘除挂载），不回退判定结构；上线后观察体检评分与日报内容一周期。
- [active 版块数增长后每 tag 全库 cosine 计算量增长] → 纯内存 dot product，当前 14 个版块 × 5158 挂载 tag 量级可忽略；版块数上百时再考虑缓存 top-k，本次不做。
- [并列最优的取值边界（浮点相等）] → 用与 bestCos_global 的差值 < 1e-9 判等（与真库验证 SQL 口径一致），避免浮点严格相等漏判。
- [rematch-all 重算期间新匹配并发写入] → 现有 rematch-all 已有逐 tag 事务语义，无新增并发面。
- [体检评分突变（ghost 病征消失→分数回升）引发用户困惑] → 属预期行为变化，归档汇报中明确「部署后影响」；评语对「贴合度分化」的点名（靖国神社）可能仍在——那是口径②问题，本次不解决。

## Migration Plan

1. 合并代码（判定四层化 + floor 配置）→ 部署后端。
2. 手动触发 rematch-all 全量重算存量 flag（用户操作，见部署影响汇报）。
3. 观察一周期：体检评分趋势、日报内容回归情况（日本新闻应重新出现高市早苗/央行类事件）。
4. 回滚策略：还原代码部署即可；若需还原 flag，再跑一次 rematch-all（判定回到双门语义）。

## Open Questions

（无——并列最优、embedding 缺失、地板线默认值均已在 D1-D5 拍定）
