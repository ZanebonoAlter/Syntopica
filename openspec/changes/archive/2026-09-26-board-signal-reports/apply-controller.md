# Apply controller：派发与验收边界

## 用户授权

用户确认当前简化样例后明确要求进入opsx-apply：实现使用`zai-coding-cn/glm-5.3-flash:max`，一次性独立审查使用`zai-coding-cn/glm-5.3:high`，主线程作为controller。UI批准已记入ui-design。不得自行切供应商、降低档位或增加第二轮审查。

## 工作区与基线

- repo/cwd：`/home/zanebono/software/Syntopica`，branch=`develop`。
- 启动HEAD：`c2d6744c493484d871f1a9953a2e89b3516917e1`（不是完整改动基线；树上已有其他工作）。
- 本地保全目录：`/tmp/syntopica-board-signal-apply-xQCFH3`，包含preexisting.patch、status.txt、untracked.tgz、归属态势及doc-impact输出；禁止还原/清理这些无关改动。该目录是本机临时证据，不能当永久归档。
- `doc-impact suggest --change board-signal-reports`已跑；规划涉及flow/api/database。`context`命令退出1且明确说明已退役，由constraint-injection替代；这不是忽略业务约束，子线程仍须读flow/standard。
- concurrency报告有其他活跃change（daily-report-margin-notes）和预存datasources/frontend改动。同文件必须对当前盘面增量编辑，不得用HEAD整文件覆盖。

## 拓扑与阶段（multi-seam，全部串行）

按仓库规则主仓库直改，不建worktree；同一时刻只有一个写线程。不同阶段具有独立合同和测试，不让一个writer承包全栈。

| 阶段 | 唯一写辖区/决策 | 下一关卡 | 交接 |
| --- | --- | --- | --- |
| 0 准入/白盒 | change内白盒枚举、真实接点/源能力核定；不写业务代码 | EIA历史路径可核定、无未批准范围缩减 | runtime绑定报告，controller确认 |
| 1 数据与源 | datasources窗口/兼容；dataenrichment repository模型/查询；database迁移 | 目标源fixture、PG形状/外键/原子保存 | 精确模型/查询/工具契约 |
| 2 编排与HTTP | dataenrichment service/handler/wire、仅必要路由装配；不任意改阶段1合同 | 发现不自动研究、40轮/计算/幂等/无review、影响包测试 | API/schema与前端消费说明 |
| 3 前端 | front数据增强API/工作台/新组件；不改后端合同 | lint/typecheck/目标组件测试、原型结构与大白话表达 | UI状态及证据入口 |
| 4 集成与文档 | 仅接缝修复、故事测试/参考文档、任务映射；不另起大重构 | scoped验证、doc-impact/standards，真实验收计划 | 本change差分范围/已执行与未执行证据 |
| 5 一次性审查 | fresh-context只读；glm-5.3 high，不写源代码 | 按High/Medium/Low给出可复现意见 | runtime绑定审查报告 |
| 6 定向修复与收尾 | flash max；先请求controller裁决审查意见再修；串行验证 | controller最终验收，部署前确认无阻塞 | 最终差分与残余风险 |

每个实现阶段结束前用contact_supervisor提交改动清单/实测结果/阻塞，请controller确认再交接。阶段遇到未批准设计变更、需要降低规格、基础设施/额度/工具失败时停，不越过阻塞继续。只有controller可决定下一步，不自动兜底换模型/CLI。

## 共同硬边界（子线程必读）

1. 先读根AGENTS、相关子目录AGENTS、开发执行规范§0.6/§2、backend/frontend测试规范与本change所有相关制品及explore-findings；实现行为以当前spec/design为准，不沿用旧4/8轮或review方案。
2. 新链：人工发现只保存候选，逐条点击才研究；一候选一研究，最多40轮/40次取数+计算，可提前结束；新报告不调用judge、不读digest、不写review。大白话样例为文案基线，不输出满篇去库/转多/接口缩写。
3. 候选快照/cutoff/owner必须服务器可信；研究/报告不污染新闻；成功快照不可变；同board原子互斥/复用成功结果。数据库安全按隔离PG测试，不连业务库做测试DDL。
4. 四源host封闭、缺失非0、单位不换算、缓存时间真实；仅新research白名单扩展；旧A/B/QA/调查/关系发现工具面不变。EIA不提供的指标不编造，JODI现有month不可丢。
5. 只跑本次影响包/测试文件；树莓派CPU任务串行，前端maxWorkers=2且参数不带额外`--`；不跑全量go test ./...或全量vitest。不把无关既有失败算本change通过，需记录归因证据。
6. 不自行commit/push/archive，不重置/覆盖无关脏改，不改harness/工具配置，不启动手提dev服务，不改数据源key。进程起停只走项目脚本，浏览器用完关闭。
7. 输出路径由workflow的output绑定。报告包含文件范围、命令/退出码、未做项、残余风险；只把真实完成的tasks勾选。不得用“编译过”代替行为用例。
8. 阶段4之前不部署；最终收尾只有controller同意才执行静态部署/重启。真实AI验收仅限本change所需少量手动样本，需先报告预计调用范围，不批量自动研究。
