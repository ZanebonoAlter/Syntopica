# Design: long-form-sampled-tagging

## Context

现状：`buildArticleSummary`（`backend-go/internal/tagmanagement/service/core/article_tagger.go:347`）按 AIContentSummary → FirecrawlContent → Content → Description 选正文后，超 4000 runes 一律掐头。双分支 extractor（event/person + keyword）共享该输入；aggregate 文章走独立的按栏目调用路径，不经此函数。

实测约束（2026-09-19 探索，数据源 `ai_call_logs` / `articles` 近 3-14 天）：

- 86% 文章仅有 RSS 短摘要（不超预算，行为不变区）；重灾区为拿 FirecrawlContent 直接打标的长文（~72 篇/天，99% 超限）与超长整理稿 mono（~17 篇/天）。
- 超长正文中 95%（409/431）带 markdown `##` 标题，文集结构天然存在。
- 正文含大量 `![](url)` 图片与行内链接，长文里 URL 可占 20-30% 长度，对标签提取是纯噪声。

## Goals / Non-Goals

**Goals:**

- 文集型长文的每个栏目在 4000 runes 预算内都可见（标题全保 + 正文均分），叙事型长文保留头/中/尾弧线。
- 噪声（图片/链接 URL）在计量预算前剥离。
- 短文路径（≤ 预算）行为逐字节不变，零回归面。

**Non-Goals:**

- 不动双分支 extractor 的调用次数与 prompt 模板（"双分支合并"是独立 change）。
- 不动 aggregate 路径的 `splitSections` 与按栏目调用逻辑。
- 不改 4000 预算总量、不改标签上限 6、不动队列/并发调度。
- 不做 map-reduce 全量分段多次调用（成本原因，未来可选）。

## Decisions

### D1: 采样全部收敛在 `buildArticleSummary`，新写独立切分器

采样发生在 `input.Summary` 生成处，双分支与 aggregate 自动继承，调用方零改动。切分器新写（`sampling_splitter.go`，估 ~80 行），不改动 aggregate 的 `splitSections`——后者带 dropIntro/mergeShort/splitLong/capCount 后处理，为"每栏目一次调用"服务；本切分器只要"宽松标题切 + 空行兜底 + 拼预算"。

- 备选：扩展现有 `splitSections`。否决：两套后处理语义会互相污染（aggregate 不想要保底/清单逻辑，采样不想要导读过滤）。

### D2: 预算算法（一次静态分配 + 可选滚动回收）

```
stripNoise(body)                        # 删 ![…](…) ；[…](…) → …
if runes ≤ 4000: 原样返回               # 86% 短文零变化
sections = splitByHeadings(body)        # 认 #~###### 标题行；无标题时按空行段聚合
if len(sections) ≤ 3:                   # 叙事型
    return 头2000 + "……" + 中1000 + "……" + 尾1000
perSection = max((4000 − Σ标题) / N, 120)
if perSection 被保底顶到超预算:          # N ≳ 20
    前 K 段按 120 采样；其余标题归一行 "其他栏目：A、B、C…"
for each section: 标题 + 句界截断(正文, perSection) + 截断尾加 "……"
```

- 句界：从切点回退找 `。！？\n`（窗口 50 runes），找不到硬切。
- 段短于预算全保，余额滚动给后续段（两遍循环；第一版可只做静态均分，差异可接受）。
- 备选：前重后轻加权（编辑排版上重要栏目在前）。否决于第一版：均匀更简单可预测，权重靠实测数据说话再补。

### D3: 切分粒度——`#`~`######` 全认 + 代码块豁免留作已知限制

firecrawl 正文标题层级不统一（h1/h3 混用），只认 `##` 会漏切。代码块 fence 内的 `#` 注释行可能误判为标题——打标输入带代码场景少，第一版接受，遇到再跳 fence（记入 Risks）。

### D4: 省略标记用裸 `……`，不改 extractor 系统提示

模型对省略号语境理解足够；改系统提示会波及所有文章（含短文），扩大回归面。

## Risks / Trade-offs

- [标题误判（代码块内 `#`、正文中伪标题行）切出过多碎段] → 保底+清单退化路径本身就是兜底：碎段越多，正文采样越少但标题仍在，不会比现状（纯掐头）差。
- [句界回退在无标点的列表/表格类正文失效] → 回退窗口内无句界允许硬切，行为退化为现状级，可接受。
- [采样后 prompt 变短，个别原来"后半碰巧有强标签"的文章标签变化] → 这是本 change 的目的本身（看得全）；上线后对少数派/阮一峰 feed 抽查一周对比。
- [中文 runes 与 markdown 半角符号混排导致预算估算偏差] → 预算容差 5%（spec 已约定），算法只在 4000 量级保持，不追求精确到字符。

## Migration Plan

1. 合并部署即生效（新文章打标走采样；存量不自动重打标）。
2. 回滚：revert 单 commit 即可，无数据迁移、无状态。
3. 验证：上线后观察 `ai_call_logs` 中 `tagmanagement.extractor_enhanced` prompt p90（预期 10.5k → ~6-7k 字符）与少数派 feed 标签多样性。

## Open Questions

（无——段数阈值 3、保底 120、头中尾 2:1:1 均为可本地调参常量，不改变 spec 行为轮廓。）
