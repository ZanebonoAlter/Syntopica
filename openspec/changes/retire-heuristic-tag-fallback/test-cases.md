# Test Cases — retire-heuristic-tag-fallback

> 复杂档强制制品。测试单元 = 一个 Requirement 的用户故事，由本文件串成完整故事；spec Scenario 只是断言片段。
> **变更契约（⓪）**：本 change 删除了「提取失败 → heuristic 兜底打标」这一行为契约，并把它换成「提取失败 → 任务失败 → 队列重试」。旧测试断言的正是被删的行为，**跑绿 ≠ 对**——旧用例须逐条处置（见 §继承与调整）。

## 0. 旧资产继承与调整（⓪ 改契约了）

| 旧 Scenario | 处置 | 旧测试 | 动作 |
| --- | --- | --- | --- |
| tagging-domain「LLM 调用失败降级 heuristic」 | **作废**（行为被删） | `extractor_test.go:404-422`（重试耗尽 → `Source=="heuristic"`） | 改为断言：返回错误 + 不产出任何标签候选 |
| tagging-domain「LLM 调用成功但零标签」 | **保留** | `extractor_test.go` 零候选用例 | 保留并加断言：任务视为完成、不触发失败重试 |
| auxiliary-label「单次调用整体失败回退 heuristic」 | **作废**（行为被删） | `extractor_test.go:145-158`（parse err → `Source=="heuristic"`） | 改为断言：返回错误 + `result == nil` |
| auxiliary-label「keyword 数组为空但 event/person 有产出」 | **保留但改条款** | `extractor_enhanced_test.go` 零候选用例（断言 `Source=="llm"`） | `ExtractionResult.Source` 字段删除 → 改为断言零标签 + 不触发降级 |
| tagging-domain「泛词标签黑名单拦截」 | **保留**（仅路径清单措辞） | `article_tagger_test.go` 泛词拦截用例 | 保留不动 |
| 失败兜底用例内的 `link.Source == "heuristic"` 断言 | **改** | `article_tagger_test.go:158`（该断言在**失败兜底**用例内，不是 reuse 用例） | 改为：提取失败 → 任务返错 + 文章无任何标签行 |
| **跨 feed reuse 路径**（真·reuse） | **保留，未改** | `article_tagger_reuse_test.go`（8 例，断言 `tagSourceReuse`） | 本 change 不动；真库实测 `article_topic_tags.source` 仍为 `llm=16802` / `reuse=11` |

**反查命令**：`bash scripts/harness/test-assets.sh tagging-domain` / `auxiliary-label`。

## 1. 主链路表（节拍串成完整故事）

| 节拍 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | LLM 提取调用因连接拒绝/超时失败（重试耗尽） | auxiliary-label「单次调用整体失败回退 heuristic」（改） | 提取层返回错误，**不产出任何标签候选**（`result == nil` 或 `Tags` 为空） | 纯逻辑单测 | `extractor_test.go` |
| 2 | 编排层拿到提取层错误 | tagging-domain「编排层不兜底规则词并把错误上抛」（新） | 编排返回错误；**不调用任何规则词兜底**；文章无任何 `article_topic_tags` 行；无 `source='heuristic'` 的 `topic_tags` | 纯逻辑单测 | `article_tagger_test.go` |
| 3 | 打标任务因提取失败被标记失败 | tagging-domain「失败任务进入队列重试」（新） | 任务以退避策略回到待处理态并被重新领取；达上限后停在 `failed` 态，失败原因可读 | 组件/集成单测 | `tag_queue.go` + `tag_job_queue.go`（既有机制，需守护用例） |
| 4 | 提取成功但零候选 | tagging-domain「提取成功但零标签仍是合法结论」（新） | 文章不写任何标签行；任务视为完成，**不触发失败重试** | 纯逻辑单测 | `extractor_test.go` / `extractor_enhanced_test.go` |
| 5 | 规则词词表整体退役 | — | `extractor_heuristic.go` 不存在；**`internal/tagmanagement/` 域内生产代码无 `"heuristic"` 字面量**（守护范围=该域，理由：全部写入点均在此域内，已实测无例外；若未来域外出现写入点，需把守护上提到 `internal/`） | 静态守护 | `TestNoHeuristicFallbackPathRemains`（已实测 /tmp 探针会红） |
| 6 | 存量清理 | — | 真库 `source='heuristic'` 归零；**已被 LLM 认领的标签保留挂载**（`Coding`/`AI Agent` 仍在 board 1980，但关联文章数从 722/85 塌到 1/16——挂载行本身不解、不解即随时可被匹配作业重挂） | 真库量化核对 | tasks 12-15 |
| 7 | 批量重打 | — | 受影响文章拿到真 event 标签、能进日报 | 真库量化核对 | tasks 16-17 |

## 2. 变体走查（五组固定清单）

### 2.1 输入

| 变体 | 答案 |
| --- | --- |
| 空串 / 纯空白 | 输入摘要为空 → LLM 返回双空数组 → 零候选（合法结论），**不触发**失败重试 |
| 纯分隔符 | 同上（提取层零候选） |
| 单 token | 正常提取路径 |
| 大小写 | 无影响（提取层不做大小写归一） |
| 特殊字符 | 无影响 |
| 超长 | 无影响（摘要截断在既有 800 runes 处，本 change 不涉及） |

### 2.2 前置

| 变体 | 答案 |
| --- | --- |
| 空集 | 零候选 → 合法结论 |
| 单元素 | 正常路径 |
| 重复 | 去重在既有 `mergeExtractedTags` 完成，本 change 不涉及 |
| 越界引用 | 不适用（无跨实体引用） |
| 部分满足 | 不适用 |

### 2.3 时间窗口

| 变体 | 答案 |
| --- | --- |
| 边界两端 | 队列退避 `failureBackoff`：attempt=1 → 1 分钟；上限 30 分钟。**重试窗口不跨天约束**（既有机制） |
| 空窗口 | 不适用 |
| 跨窗口 | 不适用 |
| 归一 | 不适用 |

### 2.4 幂等

| 变体 | 答案 |
| --- | --- |
| 重复执行 | 清理脚本用 `source='heuristic'` 谓词，重复执行影响 0 行（幂等） |
| 部分失败重试 | 清理在**单事务**内；事务回滚则全部不生效 |
| 并发 | 清理期间停后端（无并发写入）；重打走既有队列（并发安全） |

### 2.5 可用性（UI 必检前三）

| 变体 | 答案 |
| --- | --- |
| 误输入反馈 | 不适用（无新输入面） |
| 空态 | 队列面板 `failed` 任务列表为空即空态（既有渲染） |
| 错误态 | **本 change 的关键可见面**：`TagQueuePanel.vue:282` 渲染 `task.last_error`（红色 + title），失败原因（提取错误原文）可读。既有渲染路径，无需新增 |
| 加载态 | 不适用 |
| 超长文本 | `last_error` 有 `max-w-[200px] truncate` + title 提示（既有） |
| 重复提交 | 不适用 |

## 3. 效果核对（依赖断言外因素 → 真库量化核对）

| 核对项 | 触发原因 | 方法 | 量化结果 | 结论 |
| --- | --- | --- | --- | --- |
| 污染面清零 | 代码删除不作用于存量 | 真库只读查询 | `article_topic_tags` heuristic 行数 / `topic_tags` heuristic 标签数 | 待 tasks 15 填 |
| 版块泛词**文章数**塌缩 | 28 个 heuristic 标签曾挂 9 个到 board 1980 | 真库只读查询标签关联文章数 | 已实测：`Coding` 722→**1** 篇、`AI Agent` 85→**16** 篇（余下为 LLM 真实产出）；board 1980 挂载标签 Top20 全是真标签（英伟达 47 / 人工智能 41 / codex 40 …），`Coding` 已跌出前 20 | 症状基本消除；**挂载行未解**（见下） |
| 日报文章数恢复 | 重打拿到 event 标签 | 重打后查日报 | 11-11/11-12 日报文章数 vs 修复前（30/11/4） | 待 tasks 17 填（阻塞于解析器修复） |
| 挂载行是否需要解挂 | `Coding`(id=27) aux 仅 1 条 → `min(2,1)=1`、max_sim 0.8811≥0.8 → **匹配作业可随时把它重新挂回**；`AI Agent`(id=185089) 有 16 篇 LLM 真实文章，属正常 AI 主题词 | 真库只读核对 | 二者挂载行、aux、embedding 全部存活 | **本 change 不解挂**（超出「删兜底」范围）。`Coding` 若要彻底消失需另开 change 做标签退役（清 aux/embedding + 解挂 + 置 merged 或删除）；`AI Agent` 建议保留 |
| 重打成功率 | 本地 4B 模型可能仍 parse_error | 重打过程统计 | 失败率 | 待 tasks 16 填 |

> **判据**：若重打失败率仍高，说明模型侧问题未清（指纹已摘除），此时零标签是「合法零结果」而非污染，但日报仍缺文章——**须如实报告，不得掩盖**。

## 4. 白盒附加（复杂档分支表）

| 函数 | 分支 | 边界值 | 期望 |
| --- | --- | --- | --- |
| `TagExtractor.ExtractTags` | 提取成功且候选 > 0 | 候选 1 / 5 | 正常返回，`Tags` 非空 |
| 同上 | 提取成功且候选 = 0 | 0 | 返回 `Tags=[]`、**err=nil**（合法零结果） |
| 同上 | 提取失败 | 重试 3 次全失败 | 返回 `err != nil`、`result=nil`（**不再兜底**） |
| `tagArticle` | aggregate 成功 | 有 section 且有产出 | 走 aggregate 分支，`source=llm` |
| 同上 | aggregate 失败 → 回落 mono | mono 成功 | 走 mono 分支，`source=llm`（**既有编排，不变**） |
| 同上 | aggregate 失败 + mono 失败 | 两次都失败 | 返回错误 → 任务失败重试 |
| `tagArticle` | 跨 feed reuse | sibling 有合法标签 | 复用成功，`source=reuse`（**不变**） |
| 同上 | reuse 命中全黑名单 | sibling 全是泛词 | reuse 返回 false → 落回 AI 提取（**不变**） |
| `MarkFailed` | attempt < max | attempt=1 / 4 | `status=pending`、`available_at=now+backoff` |
| 同上 | attempt >= max | attempt=5 | `status=failed`（终态） |
| `filterGenericLabels` | 黑名单命中 / 未命中 | 「新闻」/「GLM Coding Plan」 | 命中丢弃；未命中保留（**不变**） |

**不适用划除**：`min(2,N)` 放宽准入、方向校验、AI 路由降级观测（`DegradationPanel`）、`downgraded` 标记（已由 `2026-11-12-retire-downgraded-mount` 退役）——均不在本 change 范围。

## 5. 验收措辞（三形态之一）

- `go test -short ./internal/tagmanagement/...` → 期望全绿
- `golangci-lint run ./...` → 期望 0 issues
- 文件存在：`internal/tagmanagement/service/core/extractor_heuristic.go` → 期望**不存在**
- 人工：真库核对 `source='heuristic'` 归零与 board 1980 泛词挂载消失（tasks 15）
- 人工：重打后日报文章数恢复核对（tasks 17）
