## Context

本 change 从“thread跨天归并”转向“event标签间相关推荐”。完整转向原因见 human-feedback.md，旧实验见 legacy-experiments.md。thread/tag/story不可互换：thread负责叙事；event标签提供事件描述；旧story仍供旧链路使用。

既有辅助标签关联在 semantic_labels + topic_tag_semantic_labels，不在 metadata.auxiliary_labels。现有标签 identity/semantic/event_keyword 向量有自己的用途，不覆盖它们。topic_tag_relations 的读端存在父子/后代展开语义，不适合塞入 related 边。

## Goals / Non-Goals

**Goals:** 从一个 event 标签直接找到同版块语义相近的事件；阈值可在线调（默认 0.86，起始 0.82 经 2026-11-10 S3 实测后上调）；历史样本验证；界面不把不可用冒充无结果。

**Non-Goals:** 统一主体身份、判断同一事实、合并标签/事件线、阶段或发酵、跨版块推断、重算thread归属、删除旧字段/表、黑名单、传递聚类。本轮不实现代码。

## Decisions

### D1. 比较事件描述，不比较叙事单元

- **结论**：输入是event标签ID和board ID；旧thread、watch、subject_key、entity ID、fingerprint_trusted均不作新关联前提。tag本身不是全局唯一事件事实。
- **落点**：Scenario「关联不依赖旧身份字段」；tasks 1.1。
- **取舍**：否决先按thread建身份再关联，因为叙事粒度、watch向量缺省不应决定事件相关性；不为本功能新增事件主实体。

### D2. 一份确定性文本，一种融合向量

- **结论**：文本为固定字段名的“名称\n…\n描述\n…\n辅助标签\n…”，字段内容trim/空白折叠，辅助标签取当前有效关联的label，规范化后去重、按Unicode码点排序。不展开aliases，不改aux身份；缺aux仍可用名称+描述。名称空白时不索引，明确invalid_source。文本不混subject_key/事件指纹/thread标题。表示版本related-event-v1进入签名；超出模型输入预算拒绝并记录失败，不默默截断产生另一个表示。
- **落点**：Scenario「文本表示稳定且辅助标签可缺失」；tasks 1.1。
- **取舍**：否决把辅助标签ID交集当硬门槛，也不套旧主谓宾min3表示，避免重新要求实体归一。

### D3. 缓存向量，不存合并或相关边

- **结论**：新增派生缓存表related_event_embeddings：tag_id、text_hash、representation_version、space_key、dimension、embedding、updated_at，主键(tag_id,representation_version,space_key)。刷新用条件upsert，仅当前文本签名可见；源文本变化后的旧缓存立即不参与查询。相关对在查询时现算，不存关系图。space_key包含实际vendor/model标识、路由模型版本与显式index generation；同维不同模型不可混算。供应商原地替换权重需显式换generation并重建，不能假装可自动感知。
- **落点**：tasks 1.1；Scenario「模型空间或文本变更后旧向量失效」。
- **取舍**：独立小表隔离现有标签去重/合并向量读端；否决复用topic_tag_relations或覆盖semantic向量。表是可丢弃索引，不是实体注册表，无别名池或身份归并历史。

### D4. 查询和索引刷新分离

- **结论**：GET只读当前索引，不调模型不写库。显式刷新CLI按board扫描当前有效event标签，调用airouter CapabilityEmbedding（operation=tagmanagement.related_event.embedding），每批最多32条、30s、并发1；按当前文本/空间签名只补缺失和失效项。初版部署及需要更新数据时显式运行，尚不新增常驻调度器；不承诺新event入库后立即可查。返回ready/partial/not_ready及候选数、已索引数，避免未刷新时假空。服务失败只影响本索引，重跑可恢复；刷新不改源标签、aux、文章或thread。输入预算按实际路由模型上限先检验，结果数量/维度/有限值/非零范数逐条检验；失败项不覆盖旧有效缓存。调用走标准审计，不逃日志、不借未获白名单的通用缓存冒充命中。
- **落点**：tasks 1.1；Scenario「索引未就绪与依赖失败」。
- **取舍**：否决每次打开popover就为全版块调模型，也不为第一版新增任务状态机。显式刷新是初版限制，需在交付说明中告诉用户。

### D5. 有限同版块查询，不做传递扩散

- **结论**：GET /api/semantic-boards/:boardId/tags/:tagId/related-events。源与候选须为当前可见、非merged的event标签，且存在有效入板关联（沿用归档可见且非mismatch的语义）；同版块候选按tag.created_at筛选[as_of-30×24h,as_of]，as_of取请求开始时服务端UTC时间，非事件发生日期。源可早于窗口但仍须在版块有效。排自身、distinct tag ID。过滤后在当前空间计算完整候选的cosine，score>=threshold，再score降序、id升序取10；不在阈值前按无关topK截断。初版用精确pgvector距离，不声称近似索引召回完整性。候选为0且源有效缓存就绪可ready空；有候选而源无有效向量为not_ready；源就绪且仅部分候选就绪为partial（即使已索引结果为空），均报覆盖数。
- **落点**：Scenario「同版块窗口与稳定排序」；tasks 1.2。
- **取舍**：否决跨版块和无限图扩展，先控制成本与语义；不把A相关B/B相关C推成A相关C。top10只是每个源的显示截断，不承诺截断后的列表相互对称。

### D6. 配置是相似度，不是身份置信度

- **结论**：ai_settings.related_event_similarity_threshold默认0.86（2026-11-10 S3 实测后由 0.82 上调），有效闭区间[0,1]、有限数；缺项回退默认，坏存量配置告警并回退，PUT非法值整体拒绝不写。与旧距离0.5/min3/主体0.92及板挂载阈值独立；每次请求读最新设置并返回实际threshold，调高/调低不需重新embed。保留既有matching-config GET/PUT及其白名单/验证/汇总模式，更新前端类型和表单。
- **落点**：backend-go/internal/tagmanagement/handler/board_crud_handler.go；front/app/features/tags/components/MatchingConfigDialog.vue；tasks 2.1。
- **取舍**：拒绝把subject_key当否决器：同义键漂移仍要求实体归一，且“相关”不要求同一主体。拒绝声称该阈值已由C1验证。

### D7. 最小已有UI入口

- **结论**：在ArchiveMatchPopover的event tag模式内展开相关列表；沿用已有文章预览查看来源，不改日报布局，不创建新的事件阶段面板。payload包含tag_id/label/description/similarity及可选source_article{id,title}，来源从该候选标签在当前版块有效来源文章中取pub_date降序、id降序首条；仅ID阅读照既有归档可读策略，来源缺失如实为空。新请求竞态按board+tag+请求序号隔离，关闭失效；配置与查询错误局部反馈。
- **落点**：front/app/features/tags/components/console/ArchiveMatchPopover.vue；ui-design.md；tasks 2.2。
- **取舍**：否决新页面/新主面板；复用已有标签卡只是增加一个局部阅读动作，UI minor。

### D8. 用历史检验相关性，不验证合并率

- **结论**：AI版块1980的历史有效event标签用同一文本表示在内存/临时文件计算候选，源库只读；embedding通过标准路由会产生正常调用审计，故“只读”指不写业务身份/缓存/关系，不隐瞒审计副作用。输出带版本/模型/范围的直接样本对、分数、辅助标签、阈值与覆盖。人工判断“是否值得一起看”，非“同一件事”。详见test-cases.md S5，结果待实测。
- **落点**：Scenario「历史样本验证相关而非同一事件」；tasks 3.1。
- **取舍**：否决等1–2周影子期及旧thread重算；65/65旧线距离和31旧样本对均不能证明本能力有效。

### D9. 保留旧功能，撤回旧目标

- **结论**：不改旧story字段/指纹校验/阶段/热度；D13向量与CLI、实体off保留，停止以本change授权继续身份回放。旧spec delta撤回，不删除现行主规范。是否清理旧实现另议，不自动迁移历史身份。
- **落点**：legacy-experiments.md；Scenario「相关连接不改身份也不传递」。
- **取舍**：否决为了新设计回滚脏树或清理真库；沉没实验不再决定新模型。

## Risks / Trade-offs

- 相似不保证有用，尤其同领域泛内容；起始 0.82 经 2026-11-10 S3 实测偏宽松（分层样本明确相关 68%，≥0.86 带 12/12），默认值已按用户拍板上调为 0.86，仍按样本持续观察。相关误推可调阈值重算，不形成持久合并黑洞。
- 初版显式刷索引，有更新延迟；partial/not_ready必须显示，不能掩盖。自动增量维护不在本期。
- 同版块/近30天/top10会漏跨版块或旧事件，这是明确范围，不是事件消失。
- 精确检索成本随板内event数增长；试验记录候选数、延迟，再决定优化，不预设复杂向量召回体系。
- 权重原地替换不可自动察觉，运维需更新generation；同维不等于同空间。

## Migration / Rollback

新建仅派生缓存；部署执行方先跑AI版块试验、再刷新索引，GET不在用户请求中偷偷批量回填。旧表/正文/映射零迁移；关新入口和服务即可停止消费，已有业务数据无需回滚。当前仅制品，无部署动作。

## Open Questions

无阻塞产品决策。显式刷新频率与后续自动增量维护作为后续优化，不暗示本期已自动化；来源样本与UI验收须实现后记录。
