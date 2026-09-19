<!-- complexity: simple -->
<!-- ui-impact: none -->
<!-- constraint-domains: semantic-board -->

## Why

mono 打标路径对超长正文（少数派、阮一峰等周刊/合集类）一律掐头 4000 runes，后半部分完全不可见——文集型文章每个栏目是一个独立主题，砍尾巴等于漏主题，标签召回质量受损。实测近 14 天超 12k 字符的待打标正文中 95% 带 markdown `##` 标题（栏目结构天然存在），具备按段均匀采样的条件。

## What Changes

- `buildArticleSummary` 的超限处理从"掐头 4000 runes"改为"分段采样分配 4000 runes 预算"：
  - 预算前先剥 markdown 噪声（`![图片](url)` 删除、`[文字](url)` 保留文字），噪声不再占用预算；
  - 按标题（`#`~`######`）切段后段数 > 3 的文集型：**标题全保 + 正文预算均分到段、段内句界截断**（保底 120 字/段；段数过多时尾部段落退化为标题清单）；
  - 段数 ≤ 3 的叙事型：头中尾采样（约 2000/1000/1000）；
  - 未超预算的短文（约 86% 的 RSS 摘要类文章）行为完全不变；
  - 总预算仍为 4000 runes，双分支 extractor、aggregate 路径、入队与持久化逻辑均不动。
- 新增独立的采样切分器（宽松标题切分 + 空行兜底），不复用也不改动 aggregate 路径的 `splitSections`（职责不同：那是"每栏目一次调用"的服务，这是"拼预算"的服务）。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `tagging-domain`: "单主题打标输入与上限参数"需求的截断行为变更——超限输入从"取前 4000 runes"改为"预算 4000 runes 内分段采样（文集型均匀采样 / 叙事型头中尾采样），预算前剥离 markdown 图片与链接噪声"。

## Impact

- 代码：`backend-go/internal/tagmanagement/service/core/`（`article_tagger.go` 的 `buildArticleSummary` 重构 + 新增采样切分器文件及其单测）；不影响 API、数据库 schema、前端。
- 可观测：`ai_call_logs` 中 `tagmanagement.extractor_enhanced` 的 prompt 长度 p90 预期从 ~10.5k 字符降至 ~6-7k（标题与系统提示为固定开销）；少数派/阮一峰等长文 feed 的标签多样性预期提升。
- 部署后影响：无需用户手动操作；存量文章不自动重打标（如需对比可对个别 feed 手动 retag）。
