# test-cases: improve-discovery-recommendations

> 复杂档白盒账本。落点均为**拟定**，不代表已存在或已通过；`人工` 行不设测试文件。效果核对均为待执行，不得以 fixture 数字冒充实库结果。基础切片细化判据见 test-cases-foundation.md。

## 故事 S1：手动查询独立于个性化推荐（锚 Requirement: Explicit Query and Personalized Refresh）

### 主链路
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（拟定） |
| --- | --- | --- | --- | --- | --- |
| 1 | 输入非空并点找订阅源 | 手动查询完整流程 | 执行态+独立结果区「关于 ×× 的结果」，可返回为你推荐 | 组件+PG | service/discovery_run_service_test.go；人工 opencli |
| 2 | 返回为你推荐 | 同上 | 原个性化列表原样恢复，查询结果不混入 | 组件 | front features/discovery 组件测试 |
| 3 | 成功查询后看兴趣记录 | 同上 | 新增独立条目（含零结果的成功查询） | PG | service/discovery_run_service_test.go |
| 4 | 空白/超长输入提交 | 输入错误和重复提交 | 就地报错不发起请求 | 组件 | front 组件测试 |
| 5 | 执行中重复点找订阅源 | 同上 | 不发第二次请求、不重复写兴趣 | PG | service/discovery_run_service_test.go（request_key 并发） |
| 6 | 查询服务失败 | 查询失败后恢复 | 输入保留、旧结果标未更新、可重试；无种子/无新卡 | PG | 同上 |

### 变体走查
| 组 | 答案 |
| --- | --- |
| 输入 | 空串/全角空白/tab → 拒；500 rune 边界内接受、501 拒；正则元字符/引号/emoji 原样入库不拼 SQL |
| 前置 | 无任何候选 → 成功零结果仍写兴趣；已有同 request_key 运行 → 复用不重建 |
| 时间 | 兴趣 created_at=写入时刻；run started/finished 落库；时钟回拨按 0 龄处理（S3） |
| 幂等 | 相同 request_key 重试不产生第二条 run/兴趣；失败重试复用 key |
| 可用性 | 精排不可用 → 本轮失败码，不静默降级；网络超时 → 同失败路径 |

## 故事 S2：只发布精排选中项（锚 Requirement: Selected Recommendations Only）

### 主链路
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（拟定） |
| --- | --- | --- | --- | --- | --- |
| 1 | 先跑旧行为复现 | 精排选择子集（前置） | 旧代码「全候选落库」复现测试转红基准 | PG | service/discovery_run_service_test.go |
| 2 | 精排选 2/6 | 精排选择子集 | 仅 2 条发布，重复 ID 只一张卡 | PG | 同上 |
| 3 | 精排返回空集合 | 零选择不是故障 | 成功零推荐空态，无兜底发布 | PG | 同上 |
| 4 | 精排返回未知 ID/坏 JSON | 服务或协议异常 | 整轮失败、旧结果标未更新、错误可定位 | PG | 同上（mock router） |
| 5 | 配置缺失 | 同上 | 失败码 configuration，不发请求 | PG | 同上 |

### 变体走查
| 组 | 答案 |
| --- | --- |
| 输入 | 理由空串/超长 → 该候选无效或整批失败（按 design D4 严格校验）；ID 重复 → 去重留一 |
| 前置 | 粗筛零候选 → 跳过 LLM 直接成功零推荐；批次 20 上限截断 |
| 时间 | 全批成功才原子发布；单批失败整轮不发布（不出现半新半旧） |
| 幂等 | 相同 hash pending → 更新理由/时间不插入 |
| 可用性 | provider 链全挂 → errors.Join 聚合；审计日志含 configuration 失败（不依赖真实调用才可观测） |

## 故事 S3：兴趣独立记录与有界影响（锚 Requirement: Independent Interest Records and Bounded Influence）

### 主链路
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（拟定） |
| --- | --- | --- | --- | --- | --- |
| 1 | 先查体育再查软件 | 多主题查询不互相平均 | 两条独立记录，无平均合成向量 | PG | service/seed_policy_test.go |
| 2 | 查询无匹配版块 | 同上 | board_id=NULL 独立保留，不挂标签不建版块 | 纯逻辑+PG | 同上 |
| 3 | 记录超 5 条 | 有界衰减 | 仅最近 K=5 参与，其余历史可见 | 纯逻辑 | service/seed_policy_test.go |
| 4 | 版块行为成熟（N≥20） | 行为成熟后让位 | 该范围 seed 份额→0，基础路 8 条不被挤占 | 纯逻辑 | 同上 |
| 5 | 相同条件仅年龄增长 | 同上 | w 单调不增；总预算 floor(budget×max(w)) 不随条数无界增长 | 纯逻辑 | 同上 |

### 白盒分支与边界值
- `w = 2^(-ageDays/halfLife) × (1-min(N/20,1))`：age=0 → w=1-min；age→window → 参与资格出局；N=0/19/20/21；halfLife=7d。
- 归属：max sim≥0.78 且领先第二 ≥0.03 才挂版块；单候选版块仅验绝对阈值；sim=1-distance。
- 分配：最大余数法、稳定 ID 破同分；未用份额不转赠其他 seed。
- 变体：同秒两查询以 ID 排序稳定；窗口两端（第 29/30/31 天）当天算参与以 now<t 窗口判据为准；时区统一 UTC。

## 故事 S4：版块与行为双路召回（锚 Requirement: Independent Board and Behavior Recall）

### 主链路
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（拟定） |
| --- | --- | --- | --- | --- | --- |
| 1 | 日本新闻近期行为偏财经 | 近期阅读不能挤掉版块方向 | 基础路 8 条保留进精排 + 行为路候选并存 | PG | service/discovery_run_service_test.go |
| 2 | 同候选两路命中 | 单路缺失与重复候选 | 单卡展示双来源徽标；单路缺失不伪造 | PG | 同上 |
| 3 | 换 embedding 模型后刷新 | 模型不兼容 | 本轮阻断报错，不混算不发布 | PG | 同上 |

### 白盒分支与边界值
- 资格过滤在截 top-8 **之前**：gone/broken、disabled、已订阅、冷却/排除、accepted 路由剔除；unknown 保留。
- 配额：基础 8/行为 8/seed 预算（S3）；批次 ≤20；全局桶独立批次不冒充版块。
- 变体：全版块无 embedding → 仅行为+全局路；两路候选完全重叠 → 去重后不虚占名额（基础路保底仍成立）。

## 故事 S5：推荐生命周期与排除（锚 Requirement: Recommendation Lifecycle and Exclusion）

### 主链路
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（拟定） |
| --- | --- | --- | --- | --- | --- |
| 1 | 旧 pending 再次入选 | 刷新与过期 | 更新理由/时间不追加；未入选者到期退出默认列表标自动过期 | PG | service/discovery_run_service_test.go |
| 2 | 暂时不看某源 | 冷却边界及长期排除 | 卡退出+冷却到期时间；30 天内刷新/查询都不再出 | PG | 同上 |
| 3 | 长期排除后刷新 | 同上 | 跨 source 阻断；目录 enabled 开关不能解除 | PG | 同上 |
| 4 | 恢复长期排除 | 恢复不是订阅 | 仅恢复资格，不建订阅不立即出卡 | PG+组件 | 同上 + front 组件测试 |
| 5 | 刷新失败 | 刷新与过期 | 不伪造刷新时间；自然到期卡仍进历史 | PG | 同上 |

### 白盒分支与边界值
- 到期判据 `now >= expires_at`；冷却 `now < snoozed_until` 阻断；两端各走一步。
- TTL=14d/冷却=30d 可配置校验（1–365）。
- 变体：排除发生在旧 run 发布前 → 发布前复查拦截；自动过期 ≠ 拒绝（不写 dismissed_at）。

## 故事 S6：展示与订阅边界（锚 Requirement: Recommendation Display and Subscription Boundaries）

### 主链路
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（拟定） |
| --- | --- | --- | --- | --- | --- |
| 1 | 订阅原生 RSS | 原生与参数订阅 | 确认地址→安全验证→事务建源；失败不标已订阅 | PG | service/accept_service_test.go |
| 2 | 订阅填参 RSSHub | 同上 | 字典优先/options 次之/输入兜底；最终 URL≤500 字符 | PG | 同上 |
| 3 | 重复 accept 同地址 | 同上 | 仅一个 Feed；无 RSSHub 文档链接（原生） | PG+组件 | 同上 + front 组件测试 |
| 4 | 未检查候选展示 | 多来源和缺失检查信息 | 未验证明示、双来源徽标、无伪造百分比 | 组件 | front 组件测试 |

## 故事 S7：旧数据可恢复迁移（锚 Requirement: Recoverable Legacy Discovery Migration）

### 主链路
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（拟定） |
| --- | --- | --- | --- | --- | --- |
| 1 | 带脏 seed 升级 | 错挂种子的升级 | 旧 seed 标 legacy inactive，不编造原查询；订阅/文章计数不变 | 隔离 PG | platform/database 迁移测试 |
| 2 | 迁移重跑 | 同上 | 幂等无副本 | 隔离 PG | 同上 |
| 3 | 回滚 v2 开关 | 同上 | 排除投影到旧资格或禁用旧发布，不绕过用户选择 | 隔离 PG | 同上 |

## 故事 S8：参数与文档契约继承（锚 MODIFIED: Recommendation API Carries Param Options / Official Documentation Link）

### 主链路
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（拟定） |
| --- | --- | --- | --- | --- | --- |
| 1 | 请求推荐列表 | 响应包含 param_options | RSSHub route 带 param_options（空字典=空集）；原生候选带实际地址无参数表 | PG | service/recommendation_param_options_test.go（改断言） |
| 2 | 打开 RSSHub 填参表单 | 表单提供文档链接 | 官方文档按钮按 doc_base 生成 | 组件 | front 组件测试 |
| 3 | 改 doc_base | doc_base 可配置 | 新链接生效 | 纯逻辑 | platform/aisettings/config_store_test.go（补） |
| 4 | 配置获取失败/原生候选 | 配置获取失败及原生来源 | RSSHub 兜底默认 base；原生无 RSSHub 链接 | 组件 | front 组件测试 |

### 继承与调整（问句⓪）
| 旧 Scenario | 处置 | 旧测试文件 | 动作 |
| --- | --- | --- | --- |
| 响应包含 param_options | 改语义（原生分流） | backend-go/internal/admin/service/recommendation_param_options_test.go | 改断言：保留原断言 + 原生候选负向 |
| 表单提供文档链接 | 继承+补兜底 | front/app/stores/discovery.test.ts 及组件测试 | 照跑 + 补配置失败兜底 |
| doc_base 可配置 | 继承 | backend-go/internal/platform/aisettings/config_store_test.go | 照跑 + 补默认值断言 |
| 字典命中/未命中/来源可追溯（3 条） | 继承 | service/route_param_option_service_test.go | 照跑（回归网） |
| 参数渲染 4 条（下拉/输入/目录/兼容） | 继承 | front 组件测试 | 照跑（回归网） |
| LLM 不产出参数值 | 继承 | service 相关测试 | 照跑（回归网） |

## 故事 S9：候选库不是订阅清单（锚 Requirement: Candidate Catalog Is Not a Subscription List）

### 主链路
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（拟定） |
| --- | --- | --- | --- | --- | --- |
| 1 | 保存有效候选 | 手动入库再订阅 | 条目出现+「尚未订阅」；无 Feed 副作用 | PG | service/candidate_catalog_service_test.go |
| 2 | 非法/重复输入 | 输入校验及同源重复 | 字段错误不落库；重复给已有条目入口 | 纯逻辑+PG | service/candidate_identity_test.go + 同上 |
| 3 | 关闭已订阅候选推荐开关 | 停用与订阅独立 | 推荐资格退出、订阅保留；失败回滚原值 | PG+组件 | service 测试 + front 组件测试 |

细化边界见 test-cases-foundation.md（URL 规范化、rune 限长、人工覆盖回退）。

## 故事 S10：上游同步与人工隔离（锚 Requirement: Upstream and Manual Metadata Isolation）

### 主链路
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（拟定） |
| --- | --- | --- | --- | --- | --- |
| 1 | 人工改介绍后触发同步 | 人工说明经同步保留 | 上游原始资料更新、人工介绍不动；参数按上游 | PG | service/catalog_sync_service_test.go（扩展） |
| 2 | 上游确认路由消失 | 上游删除或同步失败 | 标下架保留历史不删订阅；同步失败不得把全部误标 gone | PG | 同上 |

### 白盒分支
- 同步 fetch 失败 → 报错不返回假成功；content_hash 纳入 url/example 变化。
- 变体：无人工覆盖 → 全用上游；清空人工 → 回退上游；人工开关不参与 diff。

## 故事 S11：目录导入导出（锚 Requirement: Portable Catalog Import and Export）

### 主链路
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（拟定） |
| --- | --- | --- | --- | --- | --- |
| 1 | 预览混合配置后确认 | 导入后仍未订阅 | 新增入库、重复复用、逐项结果；订阅数不变、无网络探测 | PG | service/catalog_import_export_test.go |
| 2 | 部分写入失败后重试 | 混合结果与重试 | 只处理未成功项、无重复记录；冲突默认跳过 | PG | 同上 |
| 3 | 导入 v99 版本 | 不兼容版本 | 拒绝且零写入 | 纯逻辑 | 同上 |
| 4 | 确认期间目录被改 | 混合结果与重试 | revision/指纹变化 → 要求重新预览 | PG | 同上 |

### 白盒分支与边界值
- 文件 ≤2MiB、≤1000 条；名称 200/URL 500/说明 4000 rune；未知 kind 拒绝。
- 幂等：stable_key 唯一冲突 → OnConflict 复用；重跑全量预览结果稳定。

## 故事 S12：私有源与导出安全（锚 Requirement: Private Source and Export Safety）

### 主链路
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（拟定） |
| --- | --- | --- | --- | --- | --- |
| 1 | 导出含私有/凭据条目 | 默认安全导出 | 默认排除+计数说明；文件无凭据无画像 | 纯逻辑+PG | service/catalog_import_export_test.go |
| 2 | 导入内网地址条目 | 导入私网地址 | 标 private_pending；自动任务不探测 | PG | 同上 |
| 3 | 公开源检查被重定向到内网 | 重定向改变访问范围 | 逐跳校验阻断+脱敏错误 | 集成 | platform/safefetch_test.go |

### 白盒分支
- 私网/loopback/link-local/multicast/169.254.169.254 全拒；授权仅放行指定 host:port；DNS 二次解析防 rebinding；代理不绕过。
- 变体：URL userinfo 拒；带 query 保守排除出默认导出；错误信息不含完整内网 URL。

## 故事 S13：有效介绍与增量向量（锚 Requirement: Effective Descriptions and Incremental Embeddings）

### 主链路
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（拟定） |
| --- | --- | --- | --- | --- | --- |
| 1 | 人工介绍变化 vs 未变条目 | 说明变化才重建 | 前者重嵌、后者复用；清洗不删正文 | 纯逻辑+PG | service/candidate_embedding_test.go |
| 2 | 重嵌失败/模型切换 | 更新失败与模型切换 | 不覆盖完整旧向量；不兼容不混检索；条目保留 | PG | 同上 |

### 白盒分支
- 文本拼装：名称/网站各 80、分类等合计 80、说明 257（四段合计≤500 rune，对齐 embedding 512 token 上限，E2E 后收紧）；Markdown/提示块清洗；指纹含生成器版本（当前 v2）。
- 旧 revision 结果写入前复查，不覆盖新资料。

## 故事 S14：可用性检查反映真实端点（锚 Requirement: Availability Checks Reflect Actual Endpoints）

### 主链路
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（拟定） |
| --- | --- | --- | --- | --- | --- |
| 1 | 检查需参数路由（无实例） | 需参数但无实例 | 状态 requires_parameters，不发请求 | 纯逻辑 | service/availability_test.go |
| 2 | 超时 / 200 但非 RSS | 短暂故障和内容错误 | 分别记网络错误/内容校验失败；单次不判死 | 集成 | platform/safefetch_test.go + availability_test.go |
| 3 | 连续 3 次失败跨 24h | 同上 | 升级 broken；410 直接 broken；429 按 Retry-After 推迟 | 纯逻辑 | service/availability_test.go |
| 4 | 失效源修复复查但推荐已关 | 修复后复查 | 可用性转 ok+时间；推荐开关与订阅不动 | 纯逻辑+PG | 同上 |

### 白盒分支与边界值
- 状态机：unknown→ok/broken/requires_parameters；consecutive=0/2/3；跨度 23h59m/24h；Retry-After 有界上限 7d。
- 变体：端点（参数实例）变化 → 旧检查作废重新 unknown；同模板多实例互不影响。

## 故事 S15：三页签状态与视口（锚 Requirement: Catalog States and Approved Navigation）

### 主链路
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（拟定） |
| --- | --- | --- | --- | --- | --- |
| 1 | 空库/筛选无结果/请求失败 | 空库与筛选无结果 | 三态区分：新增导入入口 / 清筛选 / 重试不冒称空库 | 组件 | front features/discovery 组件测试 |
| 2 | 双视口+窄屏长文本 | 长文本与双视口 | 无横向溢出、contained≤1120px、dialog≤92vw | 人工 | 人工：opencli + 截图（任务 8.4） |

## 效果核对（触发问句④，待执行）
- 触发：双路召回是否实际改善方向性。方法：日本新闻/AI/开发工具三版块只读对照（任务 6.3），记录基线 vs 双路候选数、重复率、基础路保留率、精排零结果率。结论以实库输出为准。

## 层与运行说明
- 纯逻辑=Go 内存单测；PG=testcontainer 隔离库（禁止碰开发库）；组件=Vitest；人工=opencli/截图。SQL 断言只在隔离 PG；前端编译类命令走 Windows cmd。
