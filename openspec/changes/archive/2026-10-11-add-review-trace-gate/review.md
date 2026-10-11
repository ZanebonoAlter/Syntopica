# Review — add-review-trace-gate

> review 意见建账（add-review-trace-gate 引入的格式契约，本文件即首个实例）：每条一
> 个条目（`### R-<N> <日期>【High|Medium|Low】<标题>` + 状态行，正文在状态行之后），
> 随 change 归档永久保存；处置词五值封闭：待处理/已修复/已采纳/已驳回/误报丢弃。

## 意见清单

### R-1 2026-10-11【High】count_entries 首版「物理次行」判定在空行分隔条目形态下漏计全部合法条目

- **状态**：已修复（smoke 红 → 绿）

fixture（标题与状态行间空行，human-feedback.md 同构形态）下 review-trace.smoke 六例
两红——建账通过误判空壳、双痕迹漏注。解析器改为「标题行后首个非空行为状态行」，
空行跳过等待。

### R-2 2026-10-11【Medium】spec-gate.smoke 7i/7j 真跑脚本的 REAL_RT 路径少升一级，waived fixture 被误阻断

- **状态**：已修复（真跑 fixture 复核 + debug 复现定位）

`__dirname/../../scripts/...` 落在 `.pi/` 下（脚本不存在 → bash exit 127 → review
项恒红），7j「waived 放行」断言误红且 reason 无「零痕迹」文案；改 `../../../` 三级
到仓库根后 7i/7j 各归其位（target 断言曾误过暴露路径问题）。

### R-3 2026-10-11【Low】7i/7j fixture proposal 误带 ui-impact 声明，UI 验收检查串扰 review 单变量判定

- **状态**：已修复

fixture 带 `<!-- ui-impact: none -->` 但无 .openspec.yaml 合同 → missingArchiveUiEvidence
按声明缺失计失败，review-trace 之外多一项红。去掉声明走 legacy 零检查，review-trace
成为唯一判定变量。

### R-4 2026-10-11【Low】lifecycle review 覆盖段行首对齐与既有五段不齐

- **状态**：误报丢弃（三行自身已对齐 16 显示宽，非问题）

「review 类派发」（13 显示宽）与「痕迹形态」（8 宽）等行标签按 16 宽基准对齐
（痕迹形态+8sp / review 类派发+3sp / 修复类派发+6sp）；渲染列宽本无硬约束，
wide() 亦未用于段内标签。
