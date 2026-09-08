# test-cases: add-composite-labels

> 复杂档依据：匹配规则优先级算法（composite_hit > direct_hit > 间接三规则）+ 建议生命周期状态机（pending/watch/dismissed/confirmed + 冷却期）+ 去重两级算法（L1 集合 + L2 embedding）。以下每个故事锚定一个 Requirement；测试文件为计划落点，实施时按 tasks.md 对账。

## 故事 S1：用户/建议产线创建一个有指向性的组合标签（锚 Requirement: composite-label: 组合标签数据模型 + semantic-label-model: composite_components 组件引用表）

### 主链路（节拍串联）
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 确认 compose 建议创建「美债收益率」 | 升级建议确认创建组合标签（composite-label） | semantic_labels 行（label_type="composite", source="upgrade_suggest", status="active"）+ 2 条 composite_components（position=1「美国国债」、position=2「收益率」） | testcontainer PG | `backend-go/internal/tagmanagement/service/auxlabel/composite_label_service_test.go` |
| 2 | 治理面板手动创建「中国CPI」选组件 [中国, CPI] | 手动创建组合标签 | label_type="composite", source="manual", status="active" + 组件引用 | handler | `backend-go/internal/tagmanagement/handler/semantic_board_handler_test.go`（或新增 composite handler test） |
| 3 | 提交 6 个组件 | 组件数量约束 | 拒绝创建，返回明确错误，无落库 | service | `composite_label_service_test.go` |
| 4 | 组件引用了 board/composite 类型标签 | 组件必须指向辅助标签 | 拒绝创建，返回明确错误 | service | `composite_label_service_test.go` |
| 5 | 组件引用了 disabled 的 aux | 组件必须指向辅助标签（active 语义，tasks 2.1） | 拒绝创建 | service | `composite_label_service_test.go` |
| 6 | 删除组合标签行 | 组合标签删除时组件级联（semantic-label-model） | composite_components 级联删除 | testcontainer PG | `backend-go/internal/platform/database/composite_components_migration_test.go` |
| 7 | 表结构/主键校验 | （模型契约，无 Scenario） | TableName=gorm 复数、复合主键 (composite_id, component_label_id) tag 正确 | 单元（无 DB） | `backend-go/internal/models/semantic_label_test.go`（照 TopicTagBoardLabel 模式） |

### 变体走查
| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 输入：组合名空串/纯空白/超长/特殊字符（emoji、正则元字符） | 空白拒绝；超长按 semantic_labels 既有 label 长度约束拒绝/截断（对齐 aux 创建契约）；特殊字符允许（label 是自由文本） | service | `composite_label_service_test.go` |
| 2 | 输入：组件 ID 列表含重复（[A, A]）/ 乱序传入 | 重复组件去重后按数量校验（2 个 A=1 组件，拒绝）；position 按传入顺序重排 1..n | service | `composite_label_service_test.go` |
| 3 | 前置：组件 ID 不存在 / 跨类型引用（board、composite 套 composite） | 拒绝，错误信息指明违规组件 | service | `composite_label_service_test.go` |
| 4 | 前置：组件数量边界 1/2/5/6 | 1 与 6 拒绝；2 与 5 接受 | service | `composite_label_service_test.go` |
| 5 | 幂等：同一创建请求重复提交（并发双开） | 第二次走 L1/L2 去重命中既有组合（ref_count 不双计，见 S3） | service | `composite_label_service_test.go` |
| 6 | label_type 列写入 "composite" | 无 CHECK 约束阻拦，自然兼容（task 1.3 验证） | testcontainer PG | `composite_components_migration_test.go` |

## 故事 S2：组合 embedding 必须由 LLM 对组合短语生成（锚 Requirement: composite-label: 组合标签 embedding 由 LLM 对组合短语生成）

### 主链路（节拍串联）
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 创建组合标签，embedder 正常 | 创建时生成 embedding | 调用 embedder 输入=`label + ". " + description` 模式（对齐 board embedding 生成），向量写入 embedding 字段；merge_embedding 不使用（保持 NULL） | service | `composite_label_service_test.go` |
| 2 | 创建时 embedder 失败 | embedder 失败则创建失败 | 事务回滚：semantic_labels 行与 composite_components 均不落库，错误返回 | service | `composite_label_service_test.go` |
| 3 | 代码走查：无组件向量加权/平均路径 | SHALL NOT 合成（红线） | 单测断言 embedder 收到的输入是组合短语文本（非组件向量拼接请求）；评审确认无合成代码分支 | 单元 + 人工 | `composite_label_service_test.go` + 人工：代码走查留痕 |

### 变体走查
| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 输入：label 有、description 空 | 仍调用 embedder（label 单独成短语），不因 description 空而跳过或合成 | service | `composite_label_service_test.go` |
| 2 | 可用性：embedder 超时/限流 | 同失败路径：整体回滚、建议保持 pending（S9 联动）、handler 返回结构化错误 | service | `composite_label_service_test.go` |

## 故事 S3：新组合先去重再落库，组合空间不碎片化（锚 Requirement: composite-label: 组合标签去重 canonical 化）

### 主链路（节拍串联）
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 新建「美债收益率」组件 canonical 集 {美国国债, 收益率}，已存在同集合组合「美国国债收益率」 | L1 canonical 集合相等复用 | 不新建，ref_count++，返回既有组合 | testcontainer PG | `composite_label_service_test.go` |
| 2 | 新建「美债利率」组件集 {美国国债, 利率} 与既有 {美国国债, 收益率} 不同，embedding cosine=0.96 | L2 embedding 相似追加 alias | 不新建，addAlias「美债利率」+ ref_count++，不改 label 不重算 embedding | testcontainer PG | `composite_label_service_test.go` |
| 3 | 新组合集合不同且 cosine < 0.95 | 均未命中新建 | 新建 label_type="composite" 行 | testcontainer PG | `composite_label_service_test.go` |
| 4 | 组件经 aux canonical 归一后集合相等（组件是同义簇别名） | L1（组件先归一到 aux canonical ID） | 归一后按 L1 命中复用 | testcontainer PG | `composite_label_service_test.go` |

### 变体走查
| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 边界：cosine = 0.9499 / 0.95 / 0.9501 | 0.95 命中 L2（含边界），0.9499 不命中 | service | `composite_label_service_test.go` |
| 2 | 边界：`composite_dedupe_sim` ai_settings 改 0.90 | 新阈值生效（可配验证） | service | `composite_label_service_test.go` |
| 3 | 前置：既有组合已 disabled（embedding=NULL） | L2 无法比较（无向量）→ 不与 disabled 组合合并；L1 仍按集合可命中？（见判据：disabled 不参与匹配但行存在——L1 命中时 ref_count++ 但不自动启用，返回复用提示） | service | `composite_label_service_test.go`（判据主线程定） |
| 4 | 幂等：L2 alias 重复追加同名 alias | aliases 集合去重，ref_count 不重复 ++ | service | `composite_label_service_test.go` |
| 5 | 红线：去重路径不调 merge_embedding | SHALL NOT（spec 红线） | 单测断言去重全程无 merge_embedding 读写 | service | `composite_label_service_test.go` |

## 故事 S4：组合标签可挂载、可禁用，禁用即弃向量（锚 Requirement: composite-label: 组合标签挂载复用关联表 + 禁用即弃向量 + 治理操作）

### 主链路（节拍串联）
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | tag「美债收益率创两周新高」关联组合标签 | tag 挂载组合标签 | topic_tag_semantic_labels 记录创建，组合 ref_count++ | testcontainer PG | `composite_label_service_test.go`（复用 attach 路径） |
| 2 | 组合标签加入版块「美债观察」composition | 版块 composition 挂载组合标签 | board_composition 记录指向组合标签 | testcontainer PG | `composite_label_service_test.go` |
| 3 | 禁用组合标签「美债收益率」 | 禁用组合标签 | 同事务 embedding=NULL，composite_components 与 aliases 保留，后续匹配不使用 | service | `composite_label_service_test.go` |
| 4 | 重新启用 | （启用重算路径，tasks 2.3） | embedding 重算恢复，匹配重新可用 | service | `composite_label_service_test.go` |
| 5 | 治理面板查列表 | 列表展示组件 | 每条展示 label、组件序列（按 position）、ref_count、status；status 过滤生效 | handler + 组件 | `semantic_board_handler_test.go` + `front/app/features/tags/components/CompositeLabelPool.test.ts`（新增，Vitest） |

### 变体走查
| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 前置：空库无组合标签 | 列表返回空态（不报错） | handler/组件 | handler test + Vitest 空态 |
| 2 | 幂等：同 tag 重复挂载同组合 | ref_count 只 ++ 一次（对齐 aux attach 既有纪律） | service | `composite_label_service_test.go` |
| 3 | 可用性：禁用操作失败（DB 错误） | 前端显示结构化错误，状态不变 | 组件 | `CompositeLabelPool.test.ts` |

## 故事 S5：tag→board 匹配按新优先级判定（锚 Requirement: tag-to-board-matching: Tag 通过辅助标签匹配 Board [MODIFIED]）

### 主链路（节拍串联）
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | tag 关联组合「美债收益率」，board「美债观察」composition 挂载同组合 | 组合标签直接命中 | match_reason="composite_hit"，score=1.0，direction_mismatch=false | service | `backend-go/internal/tagmanagement/service/board/semantic_board_matching_test.go` |
| 2 | tag aux {AI, 机器学习} 与 board #100 composition 交集=2 ≥ min_overlap，方向 cosine ≥ threshold | 直接命中 board 构成标签 | match_reason="direct_hit"，score=direct_hit_score_factor（默认 0.7），direction_mismatch=false | service | `semantic_board_matching_test.go` |
| 3 | 同上但方向 cosine=0.55 < threshold | 降级 direct_hit 方向校验不通过 | 仍挂载，direct_hit，score=0.7，direction_mismatch=true | service | `semantic_board_matching_test.go` |
| 4 | tag 同时组合命中（board A）且单标签重叠（board A、B） | 组合命中与单标签重叠同时存在 | board A 只记 composite_hit（score=1.0），不记 direct_hit；board B 正常走单标签规则 | service | `semantic_board_matching_test.go` |
| 5 | 系统无任何组合标签 | 无组合标签时行为 | 走降级单标签 direct_hit + 间接三规则，仅分数与 direction_mismatch 行为与旧版不同 | service | `semantic_board_matching_test.go`（回归） |
| 6 | direct_hit_min_overlap=1，交集=1 | direct_hit_min_overlap=1 向后兼容 | 交集判定同旧版；以降级语义挂载（score=0.7 + 方向校验） | service | `semantic_board_matching_test.go` |
| 7 | 交集=1 < min_overlap=2，无组合命中 | 交集数不足退回相似度匹配 | 不以 direct_hit 匹配，退回 hit_rate/max_sim/weighted | service | `semantic_board_matching_test.go`（回归） |

### 变体走查
| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 边界：交集数 = min_overlap-1 / min_overlap / min_overlap+1（min_overlap=2） | 1 不触发 direct_hit；2、3 触发降级 direct_hit | service | `semantic_board_matching_test.go` |
| 2 | 边界：方向 cosine = threshold-ε / threshold / threshold+ε | threshold 含边界判定与间接规则一致（≥ 通过） | service | `semantic_board_matching_test.go` |
| 3 | 边界：`direct_hit_score_factor` ai_settings 改 0.5 / 1.0 | 可配生效；1.0 时恢复旧分数但方向校验仍在 | service | `semantic_board_matching_test.go` |
| 4 | 前置：组合标签存在但 disabled（embedding=NULL） | 不参与 composite_hit 判定（禁用即弃向量语义延伸到匹配输入） | service | `semantic_board_matching_test.go` |
| 5 | 前置：tag 关联多个组合、board 挂多个组合，部分交集 | 任一交集即 composite_hit（score 恒 1.0，不叠加） | service | `semantic_board_matching_test.go` |
| 6 | 排序：同 tag 候选中 composite_hit(1.0) vs 高分 weighted(0.85) | composite_hit 排前；挂载上限 3 截断语义不变 | service | `semantic_board_matching_test.go` |

## 故事 S6：匹配输入加载组合标签并走缓存（锚 Requirement: tag-to-board-matching: 匹配输入加载组合标签 [ADDED]）

### 主链路（节拍串联）
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | tag 关联组合且 board composition 挂同组合，执行匹配 | 加载组合标签参与匹配 | 识别交集产出 composite_hit | service | `semantic_board_matching_test.go` |
| 2 | 匹配两次（cache 生效） | 组合标签纳入缓存 | 第二次命中 board match cache，组合数据不重复查库 | service | `backend-go/internal/tagmanagement/service/board/semantic_board_cache_test.go`（如无则落 matching test） |
| 3 | board composition 组合挂载变更后再匹配 | 组合标签纳入缓存（失效） | cache 失效，下次匹配重载组合数据 | service | 同上 |

### 变体走查
| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 前置：组合存在但 embedding 为 NULL（disabled 或历史缺失） | 加载时跳过该组合（无向量不参与判定），不 panic | service | `semantic_board_matching_test.go` |
| 2 | 幂等：缓存失效后重建，数据与直查一致 | 一致性断言 | service | cache test |

## 故事 S7：存量归类按新规则重算（锚 Requirement: tag-to-board-matching: 存量匹配重算 [ADDED]）

### 主链路（节拍串联）
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 存量 tag 原以 direct_hit score=1.0 挂 board X（单标签重叠、方向不符），跑 mode="all" backfill | 全量重算后 direct_hit 降级生效 | 记录重写为 score=direct_hit_score_factor、direction_mismatch=true | testcontainer PG | `backend-go/internal/tagmanagement/service/board/semantic_board_backfill_test.go` |
| 2 | backfill 中单 tag 失败 | （复用现有回填机制 Scenario） | 不阻塞批次（既有行为回归） | testcontainer PG | `semantic_board_backfill_test.go`（回归） |

### 变体走查
| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 前置：存量含组合命中候选 | 重算后该记录变为 composite_hit score=1.0 | testcontainer PG | backfill test |
| 2 | 幂等：连续跑两次 mode="all" | 结果稳定（重写幂等，无重复挂载） | testcontainer PG | backfill test |

## 故事 S8：co-tag 高频共现对产出 compose 建议（锚 Requirement: board-upgrade: co-tag 高频共现对产出组合标签建议）

### 主链路（节拍串联）
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 「美国国债」×「收益率」30 天窗口共现 15 ≥ 10，ref_count 达标，LLM 裁决值得组合 | 高频共现对产出 compose 建议 | decision="compose" 建议落库 pending（组合名「美债收益率」、组件引用、描述） | service | `backend-go/internal/tagmanagement/service/board/semantic_board_upgrade_test.go` |
| 2 | 「日本」×「市场」共现 12，LLM 裁决无明确指向语义 | LLM 裁决无意义组合被过滤 | 产出 skip，不落库 | service | `semantic_board_upgrade_test.go` |
| 3 | 下一轮生成同 hash 建议 | 同 hash 建议幂等 | 跳过不重复入库 | service | `semantic_board_upgrade_test.go` |
| 4 | dismissed 后冷却期内同 hash 再生成 | dismissed 冷却期内拦截 | cooldown_blocked，期满后可重生 | service | `semantic_board_upgrade_test.go` |

### 变体走查
| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 边界：共现次数 = 9 / 10 / 11（min_cooccurrence=10） | 10 含边界触发候选；9 不触发 | service | `semantic_board_upgrade_test.go` |
| 2 | 边界：组件 ref_count = 阈值-1 / 阈值 | 达标才入选（含边界） | service | `semantic_board_upgrade_test.go` |
| 3 | 时间窗口：共现事件恰在窗口边界/跨窗口 | 按 CoTagWindowDays 既有窗口语义裁剪（30 天），边界当天计入 | service | `semantic_board_upgrade_test.go`（复用 loadCoTagEventContext 语义） |
| 4 | 输入：候选对超 topN 上限 | 截断至上限（防 O(n²) 膨胀），稳定排序 | service | `semantic_board_upgrade_test.go` |
| 5 | 前置：空共现数据 / 组件已 disabled | 无候选产出；disabled aux 不入候选 | service | `semantic_board_upgrade_test.go` |
| 6 | 可用性：LLM 返回坏 JSON / 超时 | 本轮建议生成降级跳过（不崩、不产半成品），错误留痕 | service | `semantic_board_upgrade_test.go` |
| 7 | 三元组组合（3 组件共现） | 频次达标同样可产出 compose（组件 2-5 约束内） | service | `semantic_board_upgrade_test.go` |

### 效果核对
- 触发原因：compose 建议质量依赖共现统计覆盖与 LLM 裁决质量，mock 绿灯不能证明建议有价值。
- 核对方法：真实库生成一轮 compose 建议，统计候选对数量、LLM 通过率、确认后组合标签数（tasks 8.2）。
- 量化结果：实施验收时记录。
- 结论：待 apply 实测填写。

## 故事 S9：用户确认 compose 建议原子地创建组合标签（锚 Requirement: board-upgrade: compose 建议确认执行）

### 主链路（节拍串联）
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 用户确认 compose 建议「美债收益率」 | 确认创建组合标签 | 同事务：组合标签 + 组件引用 + embedding + ref_count 初始值落库，建议 → confirmed | testcontainer PG | `backend-go/internal/tagmanagement/service/board/board_upgrade_suggestion_persist_test.go` 或 `semantic_board_upgrade_test.go` |
| 2 | 确认执行时 embedder 失败 | 创建失败回滚 | 整体回滚：组合不落库，建议保持 pending，错误返回 | testcontainer PG | 同上 |
| 3 | 确认时 L1/L2 命中既有组合 | 确认时命中既有组合去重 | 不新建，复用（ref_count++），建议仍 confirmed，结果提示复用 | testcontainer PG | 同上 |
| 4 | ComputeSuggestionHash compose 签名 | （hash 幂等契约） | 同组件有序序列 → 同 hash；不同顺序/不同组件 → 不同 hash | 单元（无 DB） | `semantic_board_upgrade_test.go` 或独立 unit test |

### 变体走查
| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 状态：非 pending 建议（watch/dismissed/confirmed）确认 | 409 或按现有 suggestion 状态机拒绝，状态不变 | handler/service | handler test |
| 2 | 幂等：双击确认（并发双 confirm） | 单一终态，组合标签只创建一个（事务 + hash 幂等） | testcontainer PG | persist test |
| 3 | 部分失败：建议已标 confirmed 但组合创建失败 | 不可能态——同事务保证；测试断言事务回滚后建议仍 pending | testcontainer PG | persist test |

## 故事 S10：用户在前端看到组合标签体系（锚 Requirement: board-upgrade: 前端渲染 compose 建议 + composite-label: 治理操作）

### 主链路（节拍串联）
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 打开升级建议面板，列表含 compose 建议 | compose 建议卡片 | 展示组合名、组件序列、共现证据（频次+窗口+代表事件），确认/dismiss 操作可用 | 组件 | `front/app/features/tags/components/UpgradeSuggestionPanel.test.ts`（扩展） |
| 2 | 切换决策过滤 tab 到「组合」 | 决策过滤包含组合 | 仅展示 decision="compose" | 组件 | `UpgradeSuggestionPanel.test.ts` |
| 3 | 治理面板打开组合标签管理页 | （S4 步 5） | 列表 label/组件序列/ref_count/status；手动创建组件选择器限 aux active 2-5 个；禁用/启用 | 组件 | `front/app/features/tags/components/CompositeLabelPool.test.ts` + `CompositeLabelEditDialog.test.ts`（新增，Vitest） |
| 4 | 匹配详情查看 composite_hit 记录 | （匹配详情展示，tasks 6.3） | 显示组合名 + 组件序列 | 组件 | `front/app/features/tags/components/`（匹配详情组件测试，按现有 MatchDetail 组件扩展） |

### 变体走查
| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 可用性：误输入——组件选 1 个或 6 个提交 | 前端即时校验拦截（2-5），已选不丢 | 组件 | `CompositeLabelEditDialog.test.ts` |
| 2 | 可用性：空态（无 compose 建议/无组合标签） | 明确空态文案，不残留旧数据 | 组件 | `UpgradeSuggestionPanel.test.ts` / `CompositeLabelPool.test.ts` |
| 3 | 可用性：错误态（API 失败）/加载态 | 结构化错误提示/骨架或 spinner；确认按钮防重复提交 | 组件 | 同上 |
| 4 | 可用性：超长组合名/组件序列 | 安全截断 + title 提示，不破版 | 组件 | `CompositeLabelPool.test.ts` |
| 5 | 手动创建命中去重 | 界面提示复用既有组合（非报错），展示既有组合信息 | 组件 | `CompositeLabelEditDialog.test.ts` |

## 故事 S11：全链——建议到归类闭环（跨 Requirement 集成故事：S8→S9→S5/S7）

### 主链路（节拍串联）
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 真实库触发建议生成 → 面板确认 compose → 治理面板见组合标签 → 版块挂载组合 → 重算 → 匹配详情见 composite_hit | S8 步 1 + S9 步 1 + S5 步 1 + S7 步 1 | 端到端走通：确认后组合标签存在且可挂载；重算后目标 tag 以 composite_hit 挂载正确版块 | opencli 端到端 | 人工：opencli 驱动真实 Chrome 完成「生成建议→确认→挂载→重算→查看匹配详情」主链路（ui-verify skill） |
| 2 | 抽样 3 个组合标签验证误归类文章不再 direct_hit 直进 | （tasks 8.2 效果核对） | 原 A 类误归类 tag 不再以旧 direct_hit score=1.0 直进错误版块 | 人工 | 人工：SQL + 界面抽样留痕（结果记入本文件效果核对节） |

### 效果核对（2026-09-03 真实库实测）

**方法**：真实库（1 万+ tags）跑一轮 compose 建议生成 → 确认 5 个组合 → 挂载 3 个版块 → `mode="all"` 全量重算（10451 tags，~1.5 分钟，0 failed）→ SQL 全表统计 + 抽样。

**量化结果**：

| 指标 | 数值 |
| --- | --- |
| generate 一轮 | 47s；inserted 42（含 compose 9）/ skipped 20 / cooldown 0 |
| compose 候选对 | 12 对（共现 10-26 次） |
| LLM 通过率 | 9/12 = 75%（拒掉的 3 个为「伊朗×特朗普」「OpenAI×Anthropic」类同域并列无指向组合，裁决合理） |
| 建议确认 | 3/3 成功创建组合（美联储加息/英伟达黄仁勋/英伟达HuggingFace），L2 无误伤 |
| 重算后 composite_hit | **44 行 / 37 tags**，score 全 1.0，direction_mismatch 全 false（免校验生效） |
| 重算后 direct_hit | **342 行全部 score=0.700**（旧 1.0 降级 ✓），35 行方向不符标记 |
| 间接规则 | max_sim 2104 / weighted 1241 / hit_rate 248（量级稳定，无意外扰动） |

**抽样 3 个组合**（误归类验证）：
1. **Hugging Face Hub**（挂「生成式AI与大模型厂商」）：12 个 tag 命中——「英伟达129亿美元收购Hugging Face」「黄仁勋发文称英伟达是 Hugging Face 的完美归宿」等收购事件簇精确归类（此前靠中性 aux「Hugging Face」弱挂）。
2. **美联储加息**（挂「美国新闻」）：「美联储主席沃什在杰克逊霍尔年会放鹰暗示加息」「特朗普回应沃什鹰派发言」等 10+ 个放鹰事件簇 tag 归位。
3. **英伟达黄仁勋**（挂「全球科技巨头动态」）：含「英伟达2027财年第二财季营收超预期」——tag 挂齐「英伟达+黄仁勋」组件即推导命中（无显式关联）。

误归类抽查：tag「沃什鹰派表态引发市场对9月加息概率重估」全表仅 1 行挂载（composite_hit@美国新闻，1.0），**零 direct_hit 误挂其它版块**——同 board 组合与单标签重叠只记 composite_hit 契约生效。

**结论**：组合标签堵中性标签误归类的效果在真实库成立——LLM 裁决过滤质量高（75% 通过率且拒绝项合理）、推导式命中把事件簇精确归位、direct_hit 降级与方向校验契约全部落地。

**过程发现并修复的链路缺口（本核对的最大价值）**：
1. `POST /semantic-boards/:id/composition` 校验写死 `label_type='auxiliary'` 拒绝组合挂载 → 放宽 aux/composite + 补测试；
2. composition 挂载/移除路径从不失效匹配缓存（板级缓存无 TTL）→ 两处补 `InvalidateMatchCache()`；
3. **tag↔组合关联零写入**（确认创建只建本体，提取端 Q1 推迟，显式关联永远空 → composite_hit 空转）→ 改推导式语义：tag 挂齐 active 组合全部组件 aux 即视为挂该组合（显式 ∪ 推导），「确认→重算」闭环成立。

## 故事 S12：用户创建组合时被推荐与提示引导（锚 Requirement: composite-label: 组件候选推荐排序 + 已选组件提示现有组合，design D7 用户 review 补充）

### 主链路（节拍串联）
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 打开创建对话框，后端返回组件候选 | 组件候选推荐排序 | 版块挂载数优先 → ref_count → label；每个候选带挂载版块名；默认 top 50 | handler/service | `composite_label_handler_test.go` + `composite_label_service_test.go` |
| 2 | 前端默认展示推荐列表，chip 带「挂 N 版块」信号 | 组件候选推荐排序 | 推荐列表渲染，挂载信号可见；搜索时降级全量模糊 | 组件 | `CompositeLabelEditDialog.test.ts` |
| 3 | 选中「美国国债」后，下方展示含该组件的现有组合 | 已选组件提示现有组合 | 相关组合（名称+组件链）优先展示；组件集与现有组合完全一致时提示「将复用」 | 组件 | `CompositeLabelEditDialog.test.ts` |

### 变体走查
| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 前置：无任何版块挂载（board_count 全 0） | 纯 ref_count 降序，无挂载徽标，不报错 | service | `composite_label_service_test.go` |
| 2 | 前置：挂载了 disabled 版块的 aux | 不计入 board_count（只统计 active 版块） | service | `composite_label_service_test.go` |
| 3 | 可用性：component-options 接口失败 | 对话框降级回 aux 全量列表（现有能力），不阻断创建 | 组件 | `CompositeLabelEditDialog.test.ts` |
| 4 | 可用性：搜索关键词无命中 | 空态文案，已选组件不丢 | 组件 | `CompositeLabelEditDialog.test.ts` |

## 继承与调整（问句⓪：MODIFIED Requirements 回归走查）

`bash scripts/test-assets.sh tag-to-board-matching` / `semantic-label-model` 反查结果处置：

| 旧 Scenario | 处置 | 旧测试文件 | 动作 |
| --- | --- | --- | --- |
| tag-to-board-matching / 直接命中 board 构成标签 | 改语义（score 1.0→factor 0.7 + 强制方向校验） | `service/board/semantic_board_matching_test.go:TestSemanticBoardMatchingDirectHit` | 改断言：score 断言换 direct_hit_score_factor、补 direction_mismatch 两态断言 |
| tag-to-board-matching / direct_hit_min_overlap=1 向后兼容 | 改语义（交集判定不变，分数与方向校验新增降级语义） | `semantic_board_matching_test.go`（upsertMatchSetting direct_hit_min_overlap=1 相关用例） | 照跑交集判定 + 增补降级分数/方向断言 |
| tag-to-board-matching / 交集数不足退回相似度匹配 | 继承（行为不变） | `semantic_board_matching_test.go` | 照跑（回归网） |
| tag-to-board-matching / 间接匹配三规则（7 个 Scenario） | 继承（行为不变） | `semantic_board_matching_test.go` + `_unit_test.go` | 照跑（回归网） |
| tag-to-board-matching / 匹配参数用户可调、多 Board 归属、方向校验数据、缓存、回填批处理 | 继承（行为不变；缓存/回填语义扩展由 S6/S7 新断言补充） | matching/backfill/cache 既有测试 | 照跑 + 新增扩展断言 |
| semantic-label-model / 辅助标签写入、板块创建、SemanticBoard 全局共享 | 继承（行为不变；label_type 枚举扩值） | `service/auxlabel/auxiliary_label_service_test.go`（L3 创建等） | 照跑（回归网） |
| semantic-label-model / 新 Scenario：组合标签写入 | 新增 | `composite_label_service_test.go` | 新断言（见 S1/S2） |

## 白盒附加

### 分支表（匹配优先级——evaluateSemanticBoardMatches 决策树）
| # | 条件/分支 | 输入 | 期望 | 测试用例名 |
| --- | --- | --- | --- | --- |
| 1 | tag 组合 ∩ board 组合 ≠ ∅ | 双方挂载同组合 | score=1.0, reason=composite_hit, direction_mismatch=false | `TestMatchingCompositeDirectHit` |
| 2 | 组合命中且单标签重叠同 board | 组合交集 + aux 交集≥2 同 board | 仅记 composite_hit（优先级覆盖） | `TestMatchingCompositeOverridesDirectHit` |
| 3 | aux 交集 ≥ min_overlap 且方向 cosine ≥ threshold | {AI,机器学习} ∩ board=2, cosine 高 | reason=direct_hit, score=factor(0.7), direction_mismatch=false | `TestMatchingDirectHitDowngraded` |
| 4 | aux 交集 ≥ min_overlap 且 cosine < threshold | {国债,收益率} ∩ board=2, cosine=0.55 | reason=direct_hit, score=0.7, direction_mismatch=true | `TestMatchingDirectHitDirectionMismatch` |
| 5 | aux 交集 < min_overlap 且无组合命中 | 交集=1, min_overlap=2 | 不 direct_hit，退回 hit_rate/max_sim/weighted | `TestMatchingDirectHitInsufficientOverlap`（既有回归） |
| 6 | 无任何组合标签存在 | 库内 composite=0 | 与 5 相同路径 + 降级 direct_hit 语义 | `TestMatchingNoCompositeLabelsBaseline` |
| 7 | disabled 组合（embedding=NULL）参与交集 | 组合交集但组合已禁用 | 不触发 composite_hit | `TestMatchingDisabledCompositeExcluded` |
| 8 | composite_hit vs 高分 indirect 排序 | composite 1.0 vs weighted 0.85 | composite_hit 排前，上限 3 截断不变 | `TestMatchingCompositeRankingAndCap` |

### 边界值清单
| 变量 | 边界值 | 期望 | 测试用例名 |
| --- | --- | --- | --- |
| 组件数量 | 1 / 2 / 5 / 6 | 2、5 接受；1、6 拒绝 | `TestCreateCompositeLabelComponentCountBoundary` |
| composite_dedupe_sim | cosine 0.9499 / 0.95 / 0.9501 | 0.95 及以上命中 L2 | `TestCompositeDedupeL2Boundary` |
| composite_cotag_min_cooccurrence | 共现 9 / 10 / 11 | ≥10 触发候选（含边界） | `TestComposeCandidateCooccurrenceBoundary` |
| direct_hit_score_factor | 0.5 / 0.7（默认）/ 1.0 | 可配生效；1.0 恢复分数但方向校验保留 | `TestDirectHitScoreFactorConfigurable` |
| direct_hit_min_overlap × 交集数 | (2, 1) / (2, 2) / (1, 1) | (2,1) 不触发；(2,2) 触发降级；(1,1) 向后兼容触发 | `TestDirectHitOverlapBoundary`（既有扩展） |
| direction cosine | threshold-ε / threshold / threshold+ε | ≥ threshold 通过（与间接规则同含边界语义，按现网实现锁定） | `TestDirectHitDirectionThresholdBoundary` |
| 候选对 topN | 0 / 1 / 上限 / 上限+1 | 超限稳定截断 | `TestComposeCandidateTopNLimit` |

### 不适用划除（留痕）
- 纯分隔符组合名输入：组合名是自由文本 label（「美债-收益率」合法），不按分隔符解析，无纯分隔符拒绝语义；空白校验已覆盖。
- 时区/日期归一化：co-tag 窗口沿用 loadCoTagEventContext 既有 30 天窗口与时间语义，本 change 不改时间处理，仅复用。
- 大小写组合名去重：组合去重锚 L1 组件集合 + L2 embedding，不引入 label 大小写归一（避免新归一化规则与 alias 机制打架）；大小写变体走 L2 embedding 相似兜底。
- 并发组合创建线程安全：创建走单事务 + DB 唯一约束兜底，不声称应用层线程安全；并发幂等由 L1/L2 去重 + 事务保证（S3 变体 5、S9 变体 2 覆盖）。
