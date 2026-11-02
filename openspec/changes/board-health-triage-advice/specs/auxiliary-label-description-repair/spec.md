## ADDED Requirements

### Requirement: 共享描述修复须人工确认

系统 SHALL 提供从体检问题项进入的辅助标签描述修复能力：展示实时原描述、建议文案、可编辑新描述、直接引用版块及全局共享影响。描述须 trim 后非空、不仅重复 label、不超过 500 字符。只有用户明确确认才可创建修复任务；只查看建议/打开页面/运行体检 MUST NOT 创建任务。disabled、非 auxiliary 或不存在的标签 MUST 被拒绝。

#### Scenario: 核对并确认共享影响
- **WHEN** 用户确认将「上海」描述从商业场所表述改为城市定义
- **THEN** 系统 SHALL 在提交前声明是共享标签、使用它的版块均受影响、匹配更新覆盖全部活跃话题标签，用户取消时零写入

#### Scenario: 无效描述与禁用标签
- **WHEN** 用户提交空白、超过 500 字符、仅重复标签名的描述，或标签已 disabled/非 auxiliary
- **THEN** 系统 SHALL 返回明确错误且不修改描述、向量或匹配，不将任务显示为成功

### Requirement: 描述与 storage 向量原子更新

系统 SHALL 使用最终确认的新描述生成 storage embedding（label + ': ' + description），生成成功后在事务中校验来源版本、类型与状态并原子保存 description+embedding。MUST NOT 清空旧向量后复用 NULL 回填；MUST NOT 修改 label、aliases、merge_embedding、组合标签向量。生成/保存失败时旧描述和旧向量均保持。并发修改冲突 MUST 明确拒绝并引导重新核对，不能覆盖较新描述。

#### Scenario: 生成失败保持旧数据
- **WHEN** 修复任务生成新向量失败
- **THEN** 原描述与原 storage/merge 向量 SHALL 保持，任务记录失败原因并可重试

#### Scenario: 修改说明不改变标签身份
- **WHEN** 新描述与 storage 向量提交成功
- **THEN** label/aliases/merge_embedding SHALL 原值不变，description 与 storage 向量均为对应的新版本，不出现半更新

#### Scenario: 来源版本已变化
- **WHEN** 用户核对后、事务保存前标签已被其他操作修改或禁用
- **THEN** 系统 SHALL 拒绝覆盖，记录冲突或不可用状态；界面保留草稿并要求加载最新版本重新核对

### Requirement: 修复任务持久且幂等

任务 SHALL 持久记录请求幂等键、标签及来源版本、用户确认的新描述、执行阶段、保存版本、匹配范围与进度、失败原因。相同幂等键重试 MUST 返回同一任务；同标签未结束任务不能并发覆盖。页面关闭不取消已确认任务，worker SHALL 绑定应用生命周期而非提交请求的 context。服务重启后须可查询并恢复失租的未结束阶段；明确失败态 SHALL 等待用户重试，不自动无限重跑。不得凭内存 job 或组件本地状态宣称可恢复。

#### Scenario: 双击与请求重试不重复执行
- **WHEN** 同一提交因双击/超时使用相同幂等键重发
- **THEN** 系统 SHALL 返回同一任务，不重复重嵌、不重复提交描述

#### Scenario: 离开或服务重启后再进入
- **WHEN** 用户关闭页面或服务重启后查询已确认的任务
- **THEN** 系统 SHALL 返回持久阶段与结果；已保存的描述不再次覆盖，未完成匹配可继续或明确重试

#### Scenario: 运行中与完成后的重试幂等
- **WHEN** 用户重复重试已运行或已完成的任务
- **THEN** 系统 SHALL 返回202及同一任务当前状态，不再排队、不启动重复worker、不修改范围/计数/时间；conflict/unavailable则明确409要求重新核对，不借retry修改原描述请求或来源版本

#### Scenario: 失败等待明确重试
- **WHEN** 已确认任务进入embedding_failed、save_failed或matching_failed
- **THEN** 后台 SHALL 不自动无限重跑；用户retry只重新排入同一任务，已提交描述只续匹配，旧失败清空后展示新尝试

### Requirement: 修复后更新匹配并独立重试

描述与新向量提交后 SHALL 失效 board 匹配缓存，并用既有匹配引擎更新任务范围内全部活跃 topic_tags 的版块匹配，包含当前未归属任何版块的标签；任务范围在执行起点固定并可恢复，后续新标签沿既有实时匹配路径处理。MUST NOT 仅枚举现有匹配行或只处理当前版块，MUST NOT 改匹配算法或重嵌全部文章。任务 SHALL 返回冻结总数 total、真实成功 processed、失败 failed、不再适用 not_applicable 与未完成 remaining；所有计数由服务端提供，四类之和 SHALL 等于 total。成功指实际调用匹配引擎且正常返回（包括合法零命中）；未调用、报错或失效跳过 MUST NOT 冒充成功。冻结后被删除、禁用或合并的范围项 SHALL 持久记录为不再适用，total 保持不变，不永久卡死任务。部分失败不得标全完成。匹配失败后仅重试失败或未完成匹配，不重嵌/再次保存描述。

#### Scenario: 未归属标签也进入更新范围
- **WHEN** 修复时某活跃 topic_tag 在匹配表没有任何行
- **THEN** 该标签 SHALL 仍被纳入本次匹配范围并由既有匹配引擎处理

#### Scenario: 匹配失败只重试匹配
- **WHEN** 描述和向量已保存，但部分 tag 匹配失败
- **THEN** 任务 SHALL 标部分完成并返回计数；用户重试只继续未完成匹配，embedding 调用次数不再增加

#### Scenario: 冻结后标签不再适用
- **WHEN** 冻结范围中的标签在处理前已删除、禁用或合并
- **THEN** 系统 SHALL 不调用该项匹配，持久记录不再适用，not_applicable 增加而 processed/failed 不增加，total 不缩小；其余适用项仍正常执行

#### Scenario: 成功执行可合法零命中
- **WHEN** 既有匹配引擎对适用标签成功执行并返回空命中结果
- **THEN** 系统 SHALL 按成功执行计入 processed，不伪造版块命中，不将正常空结果误报失败

### Requirement: 更新完成与效果验证分离

界面 SHALL 展示生成向量、原子保存、匹配更新三个真实阶段及可执行恢复动作。仅在冻结范围所有仍适用项均匹配成功、其余项均有明确不再适用记录且无失败/未完成项后才显示「更新完成，效果待下一轮体检验证」，并引导处理下一项。旧体检快照、既有日报正文、历史评分 MUST 保持，不因修复被覆盖；不自动触发额外体检，不把复制、已排队、描述已保存或部分完成当成效果已改善。

#### Scenario: 结果不伪造健康改善
- **WHEN** 修复任务成功结束
- **THEN** 用户 SHALL 看到匹配更新完成与下一步处理入口，同时知道效果待下一轮体检验证；旧评分和旧快照不变
