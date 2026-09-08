# test-cases: split-board-upgrade-directions

> 复杂档依据：四格生成分发矩阵（direction×source×target 参数组合与校验）+ 扩充召回双路算法（相似+共现，阈值/上限/排除集边界）+ compose 确认事务分支扩展（挂载/回滚/去重）+ 存量迁移幂等。以下故事锚定 delta specs 的 Requirement；测试文件为计划落点，实施时按 tasks.md 对账。

## 故事 S1：用户选「创建版块×单标签」拿一份防重复的建版建议（锚 Requirement: board-upgrade: LLM 判断升级/跳过）

### 主链路（节拍串联）
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 生成入口选「创建版块+单标签」，days=7 触发生成 | 创建×单标签产出建版块建议 | 走聚类管线，prompt 含候选簇、co-tag 事件与全量活跃版块清单；返回建议 decision ∈ {create_new}（skip 不返回） | service（mock LLM） | `backend-go/internal/tagmanagement/service/board/semantic_board_upgrade_test.go` |
| 2 | 簇 [新能源,光伏,储能] 与既有版块不重复，LLM 判 create_new | 创建×单标签产出建版块建议 | 建议含版块名/描述/成员 aux，confidence=llm | service | `semantic_board_upgrade_test.go` |
| 3 | 簇 [AI,transformer,深度学习] 主题与既有版块「人工智能」重复 | 创建×单标签与已有版块重复被跳过 | LLM 产出 skip → 不落库不返回 | service | `semantic_board_upgrade_test.go` |
| 4 | 版块清单超 60 个 | （design D2 截断） | 清单按簇质心相似度截断 top-60，prompt 长度有界 | service | `semantic_board_upgrade_test.go` |

### 变体走查
| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 输入：候选 < RefCountThreshold（冷启动） | 聚类段空跑（不报错），不产 create 建议 | service | `semantic_board_upgrade_test.go` |
| 2 | 状态：LLM 返回 merge_into_existing（越权决策） | 越权决策被过滤丢弃（create 模式决策空间只有 create_new/skip），不落库 | service | `semantic_board_upgrade_test.go` |
| 3 | 前置：days=0 | 时间窗不过滤（沿用现有语义） | service | `semantic_board_upgrade_test.go` |

## 故事 S2：用户锁定「美债」版块扩充，拿一份指向明确的挂载建议（锚 Requirement: board-upgrade-expand: 扩充方向锁定单版块 / 扩充候选召回 / 版块画像上下文与二分类裁决）

### 主链路（节拍串联）
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 选「版块扩充+单标签+美债」触发生成 | 锁定版块生成 | 相似路（距离 ≤ 阈值）+ 共现路（与构成标签共现 ≥ 阈值）召回候选，并集去重、排除已挂载/disabled | service（testcontainer PG） | `backend-go/internal/tagmanagement/service/board/semantic_board_expand_test.go` |
| 2 | prompt 组装 | 版块画像进 prompt | 含版块描述、构成标签列表（组合标签带标记）、≤8 条近期 section 标题；候选带相似距离/共现次数证据 | service | `semantic_board_expand_test.go` |
| 3 | 候选「美债拍卖」被裁属于 B，「日本央行殖利率」被裁不属于 | 二分类裁决 | 仅「美债拍卖」产出 merge 建议，target=美债（服务端注入，LLM 输出无目标字段）；skip 不落库 | service（mock LLM） | `semantic_board_expand_test.go` |
| 4 | 确认该 merge 建议 | （锚 board-upgrade: 用户确认后执行升级建议——路径不变） | aux 写入美债 board_composition + MarkConfirmed + 缓存失效 | service | `semantic_board_upgrade_test.go`（既有用例照跑） |
| 5 | 美债版块无任何相关候选 | 召回为空 | 返回空建议列表（正常结果非错误） | service | `semantic_board_expand_test.go` |

### 变体走查
| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 召回边界：相似距离恰等于阈值 / 共现恰等于阈值 | 含边界值（≤ / ≥） | service | `semantic_board_expand_test.go` |
| 2 | 召回上限：相似路/共现路各超 40 条 | 各自截断 40，并集后按相关性排序送裁 | service | `semantic_board_expand_test.go` |
| 3 | 排除集：aux 已在目标版块构成 / status≠active | 不进候选 | testcontainer PG | `semantic_board_expand_test.go` |
| 4 | 前置：目标版块在建议生成后被禁用，用户确认 | 确认失败提示目标版块不可用，建议保持 pending | service | `semantic_board_upgrade_test.go` |
| 5 | 输入：expand 请求带 days | days 被忽略（共现走 CoTagWindowDays、相似全库） | service | `semantic_board_expand_test.go` |
| 6 | 组合路：共现对组件全不在（召回集∪构成集） | 整对被过滤不送裁 | service | `semantic_board_expand_test.go` |

## 故事 S3：组合标签分两路——创建即组合、扩充则组合并挂载（锚 Requirement: board-upgrade: co-tag 高频共现对产出组合标签建议 / compose 建议确认执行 / 前端渲染 compose 建议）

### 主链路（节拍串联）
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 创建方向生成（source=composite） | 高频共现对产出 compose 建议 | 共现 ≥10 且组件 ref 达标 → LLM {compose|skip}，compose 建议无 target，hash 幂等落库 | service | `semantic_board_compose_test.go`（既有用例改断言） |
| 2 | 扩充方向生成（source=composite+美债） | 扩充方向的组合建议携带目标 | 相关性过滤后送裁，compose 建议 target_board_id=美债 | service | `semantic_board_compose_test.go` |
| 3 | 确认创建方向 compose 建议 | 确认创建组合标签 | 建组合（含去重）→ confirmed，不挂载 | service（testcontainer PG） | `semantic_board_upgrade_test.go` |
| 4 | 确认扩充方向 compose 建议 | 确认扩充方向的组合建议 | 同事务建组合 + 写美债 board_composition + confirmed + 缓存失效 | testcontainer PG | `semantic_board_upgrade_test.go` |
| 5 | 前端看扩充 compose 卡片 | 扩充组合建议展示目标版块 | 卡片带「将挂载到：美债」徽标 | 组件 | `front/app/features/tags/components/UpgradeSuggestionPanel.test.ts`（新增/改） |

### 变体走查
| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 挂载写入失败（board_composition 约束冲突） | 整体回滚：组合不落库，建议保持 pending，错误返回 | service | `semantic_board_upgrade_test.go` |
| 2 | 确认时 L1/L2 去重命中既有组合 | 复用（ref_count++），挂载照常，confirmed+复用提示 | service | `semantic_board_upgrade_test.go`（既有用例扩断言） |
| 3 | 同 hash 重生成 / dismiss 后冷却期内重生成 | skipped / cooldown_blocked（机制不变，mode 值换 direction:source 后 hash 空间隔离） | service | `semantic_board_compose_test.go` |
| 4 | LLM 坏 JSON / 超时 | **语义随四格拆分调整**：手动单入口（create×composite）诚实报错（不静默空列表误导用户）；定时任务段失败仅记日志继续兄弟段（job 层降级，红线 10/16 语境适配）。均不产半成品 | service | `semantic_board_compose_test.go`（TestGenerateSuggestionsComposeLLMFailureDegrades 改断言 Error） |

## 故事 S4：退役走查——watch 消失、高置信合并只剩存量、单例簇沉默（锚 Requirement: board-upgrade: 升级建议生成路径单一化 / 定时生成仅创建方向）

### 主链路（节拍串联）
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 聚类产出单例簇后生成 | 单例簇不产建议 | 不产任何建议（无 watch，无观察池） | service | `semantic_board_upgrade_test.go` |
| 2 | 双签名高置信簇生成 | 单例簇不产建议（同 REQ：无自动合成） | 不自动合成 merge——全部建议都经 LLM | service | `semantic_board_upgrade_test.go` |
| 3 | 部署后跑迁移 | 存量 watch 建议被清理 | decision='watch' 行删除；二次执行 no-op（幂等） | testcontainer PG | `backend-go/internal/platform/database/` 新迁移测试 |
| 4 | 查看存量高置信 merge 建议 | 存量高置信合并建议保留可确认 | 展示正常，confirm/dismiss 路径可用 | service | `semantic_board_upgrade_test.go`（既有 confirm merge 用例照跑） |
| 5 | 定时任务触发 | 定时任务跑创建方向 | 依次跑 {create,aux} + {create,composite}；不产任何扩充建议；无 watch GC 段 | service/scheduler | `backend-go/internal/admin/scheduler/job_board_upgrade_suggest_test.go` |
| 6 | 定时任务第一段失败 | （红线 10 语义） | 仅记日志继续第二段，返回 nil error 不阻塞兄弟 job | scheduler | `job_board_upgrade_suggest_test.go` |

### 变体走查
| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 状态：数据库残留 semantic_board_upgrade_watch_gc_days 配置行 | 无副作用（代码不再读取，行留存无害） | 人工 | 走查留痕 |
| 2 | 组合：decision 枚举值 'watch' 仍存在 | 枚举保留（存量行 DTO 兼容），仅生成侧不产生 | 单元 | `semantic_board_upgrade_test.go` |

## 故事 S5：suggest API 只认四格参数（锚 Requirement: board-upgrade: suggestUpgrades API）

### 主链路（节拍串联）
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | `?direction=create&source=aux&days=7` | 创建方向调用 | 200，建议 decision ∈ {create_new} | handler | `backend-go/internal/tagmanagement/handler/board_upgrade_handler_test.go` |
| 2 | `?direction=expand&source=composite&target_board_id=42` | 扩充方向调用 | 200，compose 建议带 target=42 | handler | `board_upgrade_handler_test.go` |
| 3 | `?direction=expand&source=aux`（缺 target） | 扩充缺目标版块被拒 | 400 参数错误，不启动生成 | handler | `board_upgrade_handler_test.go` |
| 4 | `?mode=discover_new` | 旧 mode 参数被移除 | 400（旧参数拒绝） | handler | `board_upgrade_handler_test.go` |
| 5 | `POST /upgrade/candidates` | （REMOVED Requirement） | 404/405（端点删除） | handler | `board_upgrade_handler_test.go` |

### 变体走查
| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | `direction=create` 且带 target_board_id | 400（create 拒绝携带 target） | handler | `board_upgrade_handler_test.go` |
| 2 | target_board_id 指向 disabled/不存在版块 | 400 | handler | `board_upgrade_handler_test.go` |
| 3 | 非法组合 direction=expand&source=之外值（如 source=foo） | 400 | handler | `board_upgrade_handler_test.go` |

## 故事 S6：前端生成入口两步选择 + 版块锁定（锚 Requirement: board-upgrade: 生成入口模式选择）

### 主链路（节拍串联）
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 打开面板 | 旧内存探索区移除 | 无候选列表/簇列表/内存建议区；仅模式入口 + 持久化列表 | 组件 | `UpgradeSuggestionPanel.test.ts` |
| 2 | 选「创建版块→单标签」点生成 | 选择创建版块方向 | 触发 suggest(direction=create, source=aux, days=…)，生成中按钮 loading 且选择器禁用 | 组件 | `UpgradeSuggestionPanel.test.ts` |
| 3 | 切「版块扩充」 | 扩充方向必须选定版块 | 出现版块单选下拉（仅活跃版块）；未选定时生成按钮禁用；选定「美债」后可生成 | 组件 | `UpgradeSuggestionPanel.test.ts` |
| 4 | filter tabs | （UI contract） | 无 watch tab；全部/新建/合并/组合 四项 | 组件 | `UpgradeSuggestionPanel.test.ts` |
| 5 | 建议：扩充建议卡片 | （UI contract） | 卡片展示锁定版块徽标；无「合并到...」行内下拉 | opencli | 人工：opencli 主链路留证 |

### 变体走查
| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 状态：生成 API 失败（400/500） | 入口区行内错误提示，列表保持原状 | 组件 | `UpgradeSuggestionPanel.test.ts` |
| 2 | 状态：生成成功但 0 建议 | 空态区分「未生成过」（引导）与「本轮无建议」（含扩充覆盖提示文案） | 组件 | `UpgradeSuggestionPanel.test.ts` |
| 3 | 天数下拉：扩充方向下操作 | 禁用并提示「仅创建方向生效」 | 组件 | `UpgradeSuggestionPanel.test.ts` |

## 效果核对

- **真实库 opencli 主链路（2026-09-05）**：选「版块扩充×单标签×中东地缘政治与美伊关系（ID 1974）」生成 → 新入库 4 条 expand:aux 建议，target 全部恒等 1974（服务端注入断言通过）；merge 卡片展示「→ 中东地缘政治与美伊关系」徽标、确认按钮文案「合并进「中东地缘政治与美伊关系」」、无「合并到...」改目标下拉；面板无横向溢出（截图 `/tmp/usd_shots/panel_1266x769.png`，minor 档当前视口留证）。
- **存量对账**：迁移 20260905_0002 执行后 pending watch 行 = 0；存量 discover_new 150 行保留可确认；mode 分布 {discover_new: 150, expand:aux: 4}。
- **参数校验**：旧 mode 参数 → 400、expand 缺 target → 400、create 完整参数（days=7）→ 200（powershell 实测）。

## 继承与调整（问句⓪：MODIFIED/REMOVED 契约回归走查）

| 旧 Scenario | 处置 | 旧测试文件 | 动作 |
| --- | --- | --- | --- |
| LLM 判断创建新 board / LLM 判断跳过（board-upgrade） | 改语义（决策空间四格化） | `semantic_board_upgrade_test.go` | 改断言（prompt 含版块清单、skip 不落库） |
| LLM 不再产出 merge_into_existing（board-upgrade） | 改语义（原 discover 语境的禁令由四格参数取代） | `semantic_board_upgrade_test.go` | 改断言（create 模式越权 merge 被滤） |
| API default mode backward compatible / API expand mode（board-upgrade） | 废止（mode 参数移除） | `board_upgrade_handler_test.go` | 删除留痕，替换为 S5 新参数用例 |
| Candidates in discover/expand mode（board-upgrade: getUpgradeCandidates） | 废止（API 删除） | `board_upgrade_handler_test.go` | 删除留痕 |
| 建议卡片展示相似板块（board-upgrade: Board Affinity） | 废止（affinity 展示删） | 前端组件测试 | 删除留痕 |
| 用户手动合并到已有板块 / 无相似板块时不展示合并选项（board-upgrade: 人工合并下拉） | 废止（行内下拉删） | `UpgradeSuggestionPanel.test.ts` | 删除留痕 |
| 高频共现对产出 compose 建议 / LLM 裁决无意义组合被过滤（board-upgrade） | 改语义（加方向维度） | `semantic_board_compose_test.go` | 改断言（两路 prompt/target） |
| 同 hash 幂等 / dismissed 冷却期拦截（board-upgrade） | 继承（机制不变） | `semantic_board_compose_test.go` | 照跑 + hash mode 值断言更新 |
| 确认创建组合标签 / 创建失败回滚 / 命中去重（board-upgrade: compose 确认） | 改语义（挂载分支） | `semantic_board_upgrade_test.go` | 改/扩断言（S3 步 3-4、变体 1-2） |
| compose 建议卡片 / 决策过滤包含组合（board-upgrade: 前端渲染） | 改语义（target 徽标、tabs 无 watch） | `UpgradeSuggestionPanel.test.ts` | 改断言 |
| board-upgrade-expand 全部 11 Scenario（5 Requirements 整体 REMOVED） | 废止 | （test-assets 重建无历史映射——当初未落测试） | 无旧测试需删；新能力由 S2 覆盖 |
| 确认创建新 SemanticBoard / embedder 失败 / 确认合并到已有 / Confirm 缓存失效 / 手动回填 / 冷启动（board-upgrade 未改 REQ） | 继承 | `semantic_board_upgrade_test.go` 等 | 照跑（回归网） |

## 白盒附加（复杂档）

**分支表：四格分发（GenerateSuggestions 入口）**

| direction | source | target | days | 分支 |
| --- | --- | --- | --- | --- |
| create | aux | 0 | 生效 | cluster 管线（瘦身版） |
| create | composite | 0 | 生效 | compose 管线（无 target） |
| expand | aux | N | 忽略 | 召回+画像二分类（merge） |
| expand | composite | N | 忽略 | 召回过滤+画像二分类（compose+target） |
| （任一非法组合/非法值） | | | | handler 400（S5 变体 3） |

**边界值清单：扩充召回**

- 相似距离：阈值-ε（召回）/ 阈值（召回，≤）/ 阈值+ε（不召回）；空 embedding aux（跳过，不参与相似路）
- 共现次数：阈值-1（不召回）/ 阈值（召回，≥）
- 上限：相似路恰 40 / 41（截断）、共现路恰 40 / 41（截断）、并集 >40（不设二次截断，上限按路计）
- 排除集：已挂载 / disabled / （组合路）组件全无关
- 版块清单（create×aux）：恰 60 / 61（截断）

**不适用划除留痕**

- 变体「并发双开同一生成请求」：幂等由 suggestion_hash 兜底（继承机制，无新并发面），不另设并发用例——沿用 add-composite-labels S1 变体 5 结论。
- 性能压测（全量版块清单 token 上限）：截断 top-60 硬上界已消除无界风险，不做压测。
