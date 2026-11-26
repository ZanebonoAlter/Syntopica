# 测试用例 — fix-board-health-triage-degrade

> 完整故事：一轮体检从「模型漏判几条」到「用户仍能看到评分与分诊依据」的全链路，再覆盖另外两条触发路径与预算保护。
> 主链路落点全在后端（ui-impact: none），效果验证依赖真库一轮实测。

## 0. 契约变更反查（⓪ 五问句第一问）

`bash scripts/harness/test-assets.sh board-health-checkup` 反查结果（本 change MODIFIED 两个 Requirement）。

| 旧 Scenario / 旧测试 | 处置 | 旧测试 | 动作 |
| --- | --- | --- | --- |
| `LLM 综合评分与降级` / `llm_test.go: TestValidateTriage`「漏覆盖（少一条）」 | 契约反转：漏覆盖不再非法，改为补 insufficient | `backend-go/internal/tagmanagement/service/boardhealth/llm_test.go` | **改写**：该用例从 `require.Error` 改成「返回补齐后的清单，长度 == labels，缺失项 category=insufficient」 |
| 同上「多余条目（多一条）」 | 保留（未知 aux_id 仍致命） | 同上 | 保留断言，改名「多余条目（未知 aux id）仍整轮降级」 |
| 同上「非法类别 / 未知 aux / 重复 aux / 缺证据 / rewrite 无 suggestion」 | 保留（仍致命） | 同上 | 沿用，仅随函数签名（`normalizeTriage`）调整调用 |
| `LLM 综合评分与降级` / `run_test.go: TestCheckupBoard*`「超时→score/comment/triage 同降级」 | 保留（调用失败仍是整轮降级） | `run_test.go:60-72`、`repo_pg_test.go:540-544` | 沿用，不改断言 |
| `体检自动化与扩充手动边界` / `job_daily_report_test.go`「体检在日报后自动执行」 | 保留 + 扩展（回填也触发、预算改 30 分钟） | `backend-go/internal/admin/scheduler/job_daily_report_test.go` | 沿用原用例，新增回填触发与预算断言 |
| `体检自动化与扩充手动边界` / 其它（不改匹配数据、扩充建议手动） | 保留 | `run_test.go` | 沿用 |

## 1. 主链路（串节拍）

| # | 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | 采集 | 板块 1974（47 个构成标签）跑完统计与信号 | （前置） | metrics.Labels 长度 47，信号齐备 | PG 测试 | `boardhealth/repo_pg_test.go` |
| 2 | 裁决 | 桩 LLM 返回 45 条合法分诊项 + score=78 + comment | LLM 产出评分与评语 | 结果被接受，不报错 | 单测 | `boardhealth/llm_test.go` |
| 3 | 规范化 | `normalizeTriage(45 条, 47 labels)` | 模型漏判部分标签仍保留整轮裁决 | 返回 47 条；缺失 2 条 category=insufficient，evidence 含「未覆盖」；已有 45 条原样保留 | 单测 | `boardhealth/llm_test.go` |
| 4 | 落库 | `CheckupBoard` 用该桩跑完整管道 | 模型漏判部分标签仍保留整轮裁决 | 快照 llm_score=78、comment 非空、`jsonb_array_length(metrics->'triage')`==47、缺失项 insufficient | PG 测试 | `boardhealth/repo_pg_test.go` |
| 5 | 补齐项展示口径 | 取快照里的补齐项 | 补齐项不被猜成可执行建议 | category 恒为 insufficient，suggestion 为空 | 单测（结构断言） | `boardhealth/llm_test.go` |
| 6 | 日志 | 捕获 logging 输出 | 降级与补齐都有日志 | 每板一行含 labels/covered/filled/degraded；整轮降级行含 reason | 单测（日志捕获） | `boardhealth/run_test.go` |

## 2. 变体走查

### 输入（分诊清单形态）

| 变体 | 答案 | 落点 |
| --- | --- | --- |
| 空清单 + 非空 labels（覆盖率 0%） | 低于下限 → 整轮降级，reason 含覆盖率 | `llm_test.go` |
| 空清单 + 空 labels（板无构成标签） | 合法，triage 落库为 null（沿用现状） | `run_test.go:60` 沿用 |
| 恰好等于下限（50%） | 视作「不完整但可补齐」边界：`>= 0.5` 通过，`< 0.5` 降级 —— 两边界各一条用例 | `llm_test.go` |
| 漏 1 条（覆盖率 96%） | 补齐 1 条 insufficient | `llm_test.go` |
| 全量覆盖 | 原样通过，filled=0 | `llm_test.go` 既有用例 |
| 未知 aux_id / 重复 aux_id | 整轮降级 | `llm_test.go` 既有用例 |
| category 非法（`quarantine`） | 整轮降级 | `llm_test.go` 既有用例 |
| evidence 全空白（含全角空格） | 整轮降级 | `llm_test.go` 既有用例扩展 |
| rewrite 缺 suggestion | 整轮降级 | `llm_test.go` 既有用例 |

### 前置

| 变体 | 答案 | 落点 |
| --- | --- | --- |
| 无活跃板块 | 整轮零快照、零 LLM 调用，只记日志 | `run_test.go`（既有 seam 可注入空列表） |
| 该板块已有 6 小时内**带评分**快照（新鲜度守卫） | 跳过该板，零 LLM 调用、不写快照，记 skip 计数 | `boardhealth/run_test.go` |
| 该板块 6 小时内只有**降级快照**（`llm_score` 空） | 不算新鲜，照常真跑一轮（降级不挡重生，review M-1） | `boardhealth/run_test.go`（`TestRunCheckupGuardIgnoresDegradedSnapshot`） |
| 主轮尾步遇到 6 小时内带评分快照 | 零守卫：照常跑（每夜必须有当日依据） | `boardhealth/run_test.go`（`TestRunCheckupZeroGuardSkipsNothing`） |
| 该板块最近快照恰好 5h59m / 6h01m | `<6h` 跳过、`>=6h` 执行（边界两条） | `boardhealth/run_test.go` |
| 指定 BoardIDs 只含 1 个板 | 只处理该板 | `boardhealth/run_test.go` |
| 指定 BoardIDs 含非 board / disabled id | 与全量路径同口径：不产出快照、不报错 | `boardhealth/run_test.go` |

### 时间窗口 / 超时

| 变体 | 答案 | 落点 |
| --- | --- | --- |
| 单板超时 6 分钟 | 该板记失败日志（含 elapsed），继续下一板 | `run_test.go`（注入慢 seam + 短超时覆盖常量；真实 `checkupBoard` 的超时路径靠 ctx 取消让 `db.Create` 失败→不写快照，由该 seam 用例间接覆盖） |
| 整轮预算耗尽 | 未跑到的板不写快照，整轮正常返回 | `job_daily_report_test.go`（seam 断言 ctx deadline ≈30min） |

### 幂等 / 重复触发

| 变体 | 答案 | 落点 |
| --- | --- | --- |
| 主轮尾步 + 回填尾步同一晚连续触发 | 回填那次全部被新鲜度守卫跳过（零 LLM 调用）；回填触发本身异步，不拖长 job | `job_daily_report_test.go`（`require.Eventually`）+ `boardhealth/run_test.go` |
| 手动生成连续点两次 | 第二次在体检入口被守卫跳过，HTTP 立即返回 | 守卫在体检入口实现，handler 层只断言「seam 被异步调用 + scope 正确」（`topicgraph/handler/daily_report_handler_test.go`）；守卫跳过本身由 `boardhealth/run_test.go` 覆盖（handler 不重复实现守卫） |
| 体检 goroutine 内 panic | 被 recover，记日志，不影响日报 job / HTTP 响应 | 既有 seam 测试沿用 |

### 可用性（UI 不适用）

ui-impact: none —— 前端无改动，降级态与 insufficient 展示分支已存在，故本节划除留痕（不做视觉验收）。

## 3. 效果核对（真库量化）

| 项 | 内容 |
| --- | --- |
| 触发原因 | 11-03 实测 6 板因漏判整轮作废（47/50/54/58/96/98 标签板），修完必须证明这些板能产出依据 |
| 方法 | 部署后跑一轮真体检：`cd backend-go && go run cmd/admin/main.go board-checkup`（**写快照，需用户先确认**），随后只读 SQL：`SELECT semantic_board_id, llm_score, jsonb_array_length(COALESCE(metrics->'triage','[]'::jsonb)) FROM board_health_snapshots WHERE checked_at > now() - interval '1 hour' ORDER BY id;` |
| 量化结果（期望） | 12/12 板块 `llm_score` 非空；每个板 `triage 长度 == labels 长度`；补齐项数量 = 模型漏判数（对照日志 filled） |
| 结论口径 | 若仍有板降级：读该板日志 reason —— 覆盖率 <50% 说明模型输出崩（非本 change 能修），非法项说明真异常，两者都要在归档报告里如实记录 |

## 4. 白盒附加（分支表）

`normalizeTriage(triage, labels)` 判定树：

```
输入: triage 条目 n, labels 集合 L (m 个)
1. m == 0:
     n == 0 → (空, nil)
     n > 0  → error(unknown aux id)           // 空板却给建议
2. m > 0:
   2a. 遍历条目：category ∉ {remove,rewrite,keep,insufficient} → error
   2b. aux_id ∉ L                            → error
   2c. aux_id 重复                            → error
   2d. trim(evidence) == ""                  → error
   2e. category == rewrite && trim(suggestion)=="" → error
   2f. coverage = len(unique valid) / m
       coverage < 0.5  → error(coverage below floor)
       coverage >= 0.5 → 补齐缺失 L\seen 为 insufficient，返回 (filled, nil)
```

边界值：`coverage` 恰为 0.5（`>=` 通过）、`m=1` 且 `n=0`（0% → 降级）、`n > m`（多出的必是未知 aux → 降级）、evidence 全角空格（trim 后为空 → 降级）。

不适用：并发安全（体检单轮串行，不声称线程安全）；LLM 提示词内容（本 change 不改提示词，既有 `TestBuildScorePrompt*` 不动）。
