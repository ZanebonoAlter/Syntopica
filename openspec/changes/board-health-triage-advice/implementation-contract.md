# 实施接缝合同（v2 获批后）

用户已明确回复「这个可以」，ui-design 为 major/approved。早批实现使用 `zai-coding-cn/glm-5.3:max`；该提供方额度耗尽后，用户最新明确指定续做使用 `minimax-cn/MiniMax-M3.1-Flash:max`（用户再次确认provider；reload后已通过无工具连通核验，commandcode通道已因401停止，不作为替代），从检查点续做，不重做已完成部分，主仓库单写者、按 seam 串行交接，不覆盖其他 change 脏改。

## 1. HTTP 接缝（前后端共享）

沿用 `{success, data, error/message}`。HTTP wire沿既有Go API保留数字ID；「ID在API层转string」指前端API边界/normalizer产出字符串ID视图，模板不得直接消费数字wire ID或snake_case。无需全局修改既有AuxiliaryLabel.id数字DTO；本change新增归一视图独立采用string。

- `GET /api/auxiliary-labels/:id/description-repair-context`：纯读。data 含 `auxiliary_label:{id,label,description,status,version}`、`direct_boards:[{id,label,status}]`、`active_tag_count`、`latest_task`（空则 null）。version 为服务端不透明版本令牌；不让前端拼版本。非 auxiliary/不存在返回明确 4xx；disabled 可展示不可执行状态。
- `POST /api/auxiliary-labels/:id/description-repairs`：body=`{description,expected_version,idempotency_key}`，创建并返回持久任务，202。客户端为一次确认生成唯一键，超时重发复用同键；相同键不同 payload 必须 409，不能默默返回旧任务当成功。未结束同标签任务防并发覆盖。
- `GET /api/auxiliary-labels/:id/description-repairs/:job_id`：纯读持久任务；校验 task 属于该辅助标签。
- `POST /api/auxiliary-labels/:id/description-repairs/:job_id/retry`：无修改描述/版本的参数；body可空或空JSON对象，非法JSON/非空参数对象为400。恢复失败/失租中断任务并返回202+同一任务；运行中或已完成时为202+当前任务的幂等no-op，不重排、不启动另一个worker、不改计数或时间。conflict/unavailable不可原任务重试，返回409并要求重新核对。已提交描述只续匹配，不改expected_version/new_description、不重嵌/重存。

任务 data 至少含 `id,auxiliary_label_id,status,stage,old_description,new_description,description_committed,total,processed,failed,not_applicable,remaining,error,created_at,updated_at`；需时可扩 `completed_at/retryable` 等而不删语义。`not_applicable` 是处理时判定已失效/删除并持久记录的不再适用项数，不是匹配成功；`remaining` 是尚待处理项数。两者由服务端提供；旧任务的not_applicable按0，remaining由后端按同一持久计数计算。历史/测试wire缺remaining时前端保持未知或不展示，不伪造0或完成态；不全局改旧DTO。

status 词表：`pending|embedding|saving|matching|completed|embedding_failed|save_failed|matching_failed|conflict|unavailable`。stage=`embedding|saving|matching`（pending 可为空）。中断恢复可继续使用既有 stage + 可重试状态，不把旧持久阶段误报 completed。total 是创建时冻结的范围数量，不因后续失效而缩小。processed 指**匹配引擎实际成功处理数量**（合法零命中也算成功执行），failed 指仍处失败态的数量，not_applicable 单列处理时已失效/删除的终态项。`remaining=total-processed-failed-not_applicable`，四类之和必须等于total；各计数由服务端单源提供，前端不把跳过/失败凑作成功。只有 remaining=0、failed=0 且所有项为 succeeded/not_applicable 时才可 completed；部分失败仍 matching_failed。此澄清落实既有「失效者不再适用、不永久卡死」边界，不改变匹配算法或增加自动写入方向。

## 2. 后端正确性边界

- active auxiliary 验证；trim 后 1..500 Unicode 字符且不只重复 label。纯分隔符在当前三规则闭集下合法，不擅自加黑名单。
- version 校验覆盖 label/description/status 与更新时间等服务端语义，生成后事务内再次锁行校验。冲突响应保留用户草稿，由用户加载最新版本重新核对。
- 事务外生成最终 label+': '+description 的 storage embedding；事务内描述、storage 向量与任务「已提交/下一阶段」原子提交。label/aliases/merge_embedding 保持原样；失败不先清空向量，不借 NULL 回填绕道。
- 任务及固定 tag 范围/逐项进度持久化。创建时冻结全部 active topic_tags ID（包括无匹配者）；不得仅枚举匹配表或当前版块。新进入的 tag 沿既有实时匹配，不强行并入旧任务；范围内后来失效/删除者需明确记录不再适用而非任务永久卡死。
- 任务重启可查询，具安全恢复/明确重试，不凭组件或内存 map 伪装恢复；后台仅自动领取pending与失租未结束阶段，embedding_failed/save_failed/matching_failed等待用户明确retry，不自动无限重试失败任务或消耗AI额度。worker绑定应用生命周期而非提交HTTP的context，关页面不取消已确认任务；并发 claim/lease/幂等保护必须有 PG 用例。保存后故障恢复禁再重嵌/重存。匹配执行前和重试时失效无 TTL 的 board cache，复用既有 MatchTopicTag；不改匹配规则。
- 没有实际成功的匹配不得计成功；部分失败留可重试。完成表示范围处理结束，不表示 LLM 评估健康，不重写旧快照/日报，不自动追加体检。
- 新服务不得塞进 boardhealth，避免 boardhealth→board 循环；建议独立 `service/auxrepair` 与 repository/model 文件。handler 只绑参数/错误/响应，不直接写 SQL；外部 facade 按现有 bridge/wire 纪律接通。AI embed 走已有 airouter 和 operation 日志链。

## 3. 三病信号与前端接缝

metrics.labels 增 `magnet/neighbors/desc_flags`，metrics.triage=`[{aux_id,category,evidence,suggestion}]`；category 四值 `remove|rewrite|keep|insufficient`。缺字段/非法分类/非法aux引用/重复项等不得作为可执行错误建议透出；在测试中明确整个裁决降级或合法性处理，不能伪造证据。描述/邻居缺失需显式不足。

构成清单混有组合标签（composite）：`metrics.labels[]` 逐条带 `label_type`（auxiliary / composite），消费方据此把**组合标签的 rewrite 项标为不可执行**（描述修复链只覆盖辅助标签，组合标签没有修复入口）——证据照常展示、不隐藏、不当它健康；**remove 项不受影响**（走既有构成 DELETE 链，组合标签同样可摘除）。类型未知（历史快照缺字段）按不可执行处理，不臆造类型。

磁铁使用同轮全部挂载与失败子集，gap=all_mean-failed_mean，阈值 >=；不足不硬算。主要流向是失败 tag 在**其他有效挂载**中的 top1，不能用 direction_mismatch 失败的其他挂载当可靠流向，未知则显式未知。多归属不是摘除的充分依据。

前端 triage 仅归一不复制统计公式。问题索引只展示需核对项；keep/insufficient 与统计按需展开。进入任务后表单/证据/共享说明收为只读折叠摘要，三阶段/恢复/下一项首屏可达；完成态无可编辑 textarea。轮询按 task ID，卸载清理timer，终态停止，超时/网络失败保留已知任务而不是重新提交；重新进入从 context.latest_task 恢复。结果未知的确认请求必须保留原幂等键与冻结payload；单次context查询为空或只返回别的幂等键任务，不能证明原请求未被接受，不能自动解冻换新键。只有确认同键任务或明确最终拒绝才可解除该请求冻结；超时类响应仍视结果未知。冲突后刷新context仍须保留用户草稿，卸载后所有迟到响应不得重新启动轮询。

## 4. 交接与测试

每 seam 交接写明文件、导出/API/JSON 实际形态、最小验证命令及未决事项；需要改变本接缝时先报告父线程，不静默削弱规范。产品 tests 不接受原型38/42条模拟断言替代。只跑影响包/文件，PG 测试用 testcontainer，不碰真库有副作用 SQL；前端 maxWorkers=2，CPU 测试/构建不并行。
