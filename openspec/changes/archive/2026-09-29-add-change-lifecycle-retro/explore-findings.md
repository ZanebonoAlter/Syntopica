
## human-feedback 计量维度扩充设计（用户已拍板）

背景：ai-provider-model-overhaul 首创 human-feedback.md（FB-1~10 + 处置记录），FB-4 转挂本 change 消费：harness 计量新增人类反馈维度。当前 add-change-lifecycle-retro 的 design/tasks 对此零覆盖，扩充点即此。用户已拍板核心设计（2026-09-29 会话）：

【已拍板决策】
1. 数据源=文件：retro 解析 change 目录下 human-feedback.md（未归档读 openspec/changes/<name>/、已归档读 openspec/changes/archive/ 日期前缀剥离，与 --lifecycle 已有 proposal 头解析同构）。不进 events.db——文件是人写给人的，不要求 DB 友好格式；且 events.db 有 TTL 清扫（rollup 90 天），文件随 change 归档永久保存。
2. 状态词锁死五值：待处理/处理中/已消化/已转挂/已放弃。状态只用于 add 初始化 + 归档对账 + 体检卡展示，不算指标（用户明确砍掉消化率/消化时滞等流程效率指标）。
3. 归因词表五类（封闭枚举，参考 doc-impact.sh DOMAIN_WHITELIST 模式）：frontend（前端设计/交互/视觉）/ backend（后端业务/参数/数据）/ test-gap（测试没覆盖）/ fact-check（探索或搬运没核实就写了）/ process（流程/harness 本身）。根因优先归 fact-check（如 FB-8 既是没核实也是测试盲区，归 fact-check）。归因高发层直接映射规范整理去向（frontend→standard/frontend/、test-gap→test-design.md、fact-check→探索纪律）。存量 FB-1~10 实测归类：frontend 5（FB-3/5/6/7/9）/ fact-check 3（FB-1/2/8）/ backend 1（FB-10）/ process 1（FB-4）/ test-gap 0。
4. 条目格式（标签放条目标题行，一行正则可解析）：`### FB-N 日期【产品|归因词】标题` + 状态行。存量 10 条手工补标，不留推断口径。
5. 脚本三件套 scripts/harness/feedback.sh（照 doc-impact.sh 模式：menu 打印选项=选择题不是回忆题；add 自动编号套模板；verify 归档对账——有待处理/处理中条目提醒处置，不许静默丢）。
6. 体检卡新增第五段「人类反馈段」：条目数（产品/流程）、归因分布、未处置数。有文件才显示。

【关键实证】
- 体检卡盲区：--lifecycle ai-provider-model-overhaul 输出 UI 段「无 ui-design-gate block 记录」，而 human-feedback.md 记录两轮原型返工+approval 重置——事件侧观测不到人类真实不满，human-feedback 是旁路数据源。
- 存量格式漂移（制度化不能照搬）：状态词六种写法混用（待处理/已处理/已拍板/已修/已消化/已转挂）+ 条目状态与处置记录表双源不同步（FB-1 条目仍「待处理」但处置表写已完成）。
- doc-impact.sh 可复用模式：menu 选择题、白名单枚举、suggest/verify 对账、HTML 注释标签惯例。

【扩充范围】（complete 未归档 change 走增量，ai-provider-model-overhaul FB 先例）
- proposal.md What Changes 增补；design.md 新增决策（数据源/格式契约/词表）；tasks.md 增量节；specs/harness-retro-loop delta 增 scenario
- 文档任务：doc-authoring.md + 开发执行规范 §0.6 把 human-feedback.md 纳入 change 可选制品清单（FB-4 后续动作①）；harness-retro/harness-facts SKILL.md 同步
- 采集完整性起步两档：规范清单 + feedback.sh 工具化，不做 extension 自动化（先观察漏采程度）

<!-- pinned 2026-09-29T13:47:35Z -->

## ctx_execute_file 项目根误判根因（context-mode workspace bug）

现象：ctx_execute_file 相对路径被拼到 pi-web 安装目录（ENOENT）；绝对路径被拒「outside project root (…/node_modules/@agegr/pi-web)」。会话实际 cwd 应为 /home/zanebono/software/Syntopica。

【根因】pi-web 以 next-server（cwd=自身 npm 安装目录）进程内加载 context-mode pi 扩展（settings.json extensions 含 npm:context-mode，v1.0.169），spawn server.bundle.mjs 时未传 cwd、env 无 workspace 变量；context-mode 的 resolveProjectDir() 级联（PI_WORKSPACE_DIR→PI_PROJECT_DIR→CONTEXT_MODE_PROJECT_DIR→PWD→process.cwd()）全落空回退 process.cwd()，项目根被钉死为安装目录。resolveProjectPath() 产生相对路径 ENOENT；checkProjectBoundary()（issue #852 containment）产生绝对路径拒绝。

【上游 bug 实锤】build/adapters/detect.js:176-184 注册 PI_WORKSPACE_DIR，注释称「set by Pi's bridge」但全包 grep 无赋值点——#545 env 注入只做了一半（外来变量 scrub 做了，workspace 变量注入没做）。extension.js 的 resolvePiWorkspaceDir() 能正确算出 projectDir 但只用于 DB 归因，从未传给子进程。

【修复（本机）】pi-web 为 systemd user 服务（~/.config/systemd/user/pi-web.service）。在 unit 加 Environment="PI_WORKSPACE_DIR=/home/zanebono/software/Syntopica"（或 CONTEXT_MODE_PROJECT_DIR=同值，全平台 escape hatch）→ daemon-reload → restart pi-web。注意 restart 会终止 pi-web 内运行中的会话，需择机。局限：单 server 单固定根，多仓库场景需上游按 session 注入根治（可提 issue：github.com/mksglu/context-mode，证据 detect.js 注释与实现脱节）。

【包内 workspace 变量清单】PI_WORKSPACE_DIR（pi 平台最高优先，声明由 bridge 设实际无人设=bug）/ PI_PROJECT_DIR / CONTEXT_MODE_PROJECT_DIR（全平台通用，永不被 scrub，README 未记载）/ PWD / process.cwd()（兜底=本次事故落点）。另 CONTEXT_MODE_DIR 是存储根非 workspace 根，勿混用。

<!-- pinned 2026-09-29T13:53:32Z -->
