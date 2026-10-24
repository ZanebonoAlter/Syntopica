# Review：fix-board-health-llm-misread

> 步骤 4 review 意见建账。审查人：change-review-glm（zai-coding-cn/glm-5.3）子线程，2026-10-22；范围：制品 + llm.go/llm_test.go diff + 红线 22 边界 + DB 快照实测复核。

### R-1 2026-10-22【High】刚果疫情 90→60 留痕定性错误（H1：降档理由违反 insufficient 不扣分锚）
- **状态**：已修复
- tasks 4.4 原留痕把「评语以『所有标签样本不足』为由降档」记为「挤掉虚高水分」——事后合理化。已三处处置：①4.4 留痕改诚实定性（insufficient 误读残留，本次三处校准未覆盖此型）；②prompt 补强 insufficient 锚（「全部标签 insufficient 的板块按剩余真实指标定档，证据缺失本身不得进入评语或作为降档理由」）；③llm_test.go 该锚补 needle。待下轮体检复验刚果回升。

### R-2 2026-10-22【Medium】验证锚空转：-run TestPrompt 匹配不到 TestSystemPromptAnchors（M1）
- **状态**：已修复
- tasks 2.1 锚命令改为 `-run TestSystemPromptAnchors`（原锚形式绿但 no tests to run）。

### R-3 2026-10-22【Medium】科技巨头「不高于 65+10」容差与「不放松」意图自相矛盾（M2）
- **状态**：已修复
- 4.4 留痕改为显式豁免论证：同统计跨 prompt 版本升档（65→72）的合理性写明（ghost 全部 mismatch 行已被隔离机制中和，不进标签级统计构成侧），不再藏在 +10 容差里；锚措辞同步删除。

### R-4 2026-10-22【Low】3.1 理由句事实错误（L1）
- **状态**：已修复
- 改为「评分判据在红线 22 无对应条目（22 只锁统计口径与降级语义）」。

### R-5 2026-10-22【Low】「数学上必有约 1/3」过强（L2）
- **状态**：已修复
- spec/prompt/proposal 三处统一弱化为「P25 相对线的固有性质决定相当比例标签天然在线下」。

### R-6 2026-10-22【Low】测试 needle 脆性（L3）
- **状态**：已修复
- 空格敏感锚（`cosine_mean < 0.6`/`0.6 才视为不健康`）收敛为稳定 token（`cosine_mean`+`0.6`+`不健康` 分立断言、`双条件`/`相对位次` 语义核）；阈值变动断裂属预期（spec 同步信号）。

### R-7 2026-10-22【Low】proposal 与落码数值/强度词微差（L4）
- **状态**：已修复
- proposal 统一 0.05-0.10；spec「畸高」改「主导」与 prompt 一致。

### R-8 2026-10-22【Low】未跟踪文件提交易漏（L5）
- **状态**：已采纳
- llm_test.go 与 change 目录均为 untracked——归档提交时显式 `git add`（与 fix-board-direction-gap-exempt review R-3 的 hunk 拆分提交策略合并执行）。
