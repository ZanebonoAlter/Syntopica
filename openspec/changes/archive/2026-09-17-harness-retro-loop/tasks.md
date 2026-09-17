# Tasks

## 1. 脚本骨架与只读消费（spec：只读消费事实账本 / 空库与「无发现」语义 / 输出形态不进入注入通道）

- [x] 1.1 新建 `scripts/harness-retro.sh` 骨架：`set -u` + 与 `scripts/scenario-trace.sh` 同款头注释（用法 / 退出码 / 依赖 / 指向本 change 的 spec）；参数解析 `--db`（缺省 `.pi/harness/events.db`）/`--days`（缺省 7）/`--change`/`--json`/`--save-baseline`/`--baseline`/`--warn-threshold`（缺省 5）/`--help`；未知参数与非法取值（`--days` 非数字或 ≤0、阈值 0 或非数字）→ 用法提示 + 非 0 退出；验证：`bash scripts/harness-retro.sh --days abc` 退出码非 0 且输出用法；`--days 0`、`--warn-threshold 0`、未知参数 `--nope` 同样拒绝（白盒 T14、T12 参数侧）
- [x] 1.2 只读开库与身份校验：以 `sqlite3 -readonly` 打开 `--db` 指定路径，先 `PRAGMA application_id` 校验等于魔数 `0x53594E54`，再做任何查询；库不存在 / 非本应用库 / 文件损坏 → 中文原因（含路径）+ 非 0 退出，MUST NOT 输出看似正常的空报告；验证：`bash scripts/harness-retro.sh --db /tmp/nonexistent.db` 非 0 退出并打印路径；对 `application_id=0` 的临时库同样拒绝，且事后该库未被写入（`sha256sum` 不变）
- [x] 1.3 窗口与空库语义：窗口按 UTC 左闭计算；长度严格大于被消费 kind 的最短保留期（30 天）时打印「可能已被 TTL 清扫，差值可能失真」提示（等于 30 天不提示）；窗口内零事件时输出「窗口内无数据」并以 0 退出，MUST NOT 输出失败率数值；验证：`--days 45` 出现提示、`--days 30` 不出现；空 fixture 库退出码 0 且无失败率字样（故事 C2/C3/C6）
- [x] 1.4 输出形态与基线落盘：默认输出六段中文文本报告，`--json` 输出同结构机器可读形态；`--save-baseline [path]`（缺省 `.pi/harness/retro-baseline.json`）幂等覆盖快照；`--baseline` 不可解析时降级为纯报告 + 「基线不可用」提示（退出码 0）；脚本 MUST NOT 写 events.db、MUST NOT 触碰 `.pi/constraint-injection.json` 与 `.pi/extensions/`；验证：`bash scripts/harness-retro.sh --json | python3 -m json.tool` 可解析；保存基线后 `sha256sum .pi/harness/events.db` 不变；损坏基线（写入乱码）后退出码仍为 0 且有降级提示（故事 C8/C9/C10、T15）

## 2. 分段统计与分母口径（spec：采样加权还原的分母口径 / 报告分段与可回检指标）

- [x] 2.1 分母加权还原：单条 SQL 以 `CASE` 加权求和还原「实际执行次数」——`ok=false` 计 1；`ok=true` 无 `sampled` 计 1；`--sampled=true` 按 `n` 计（缺 `n` 回退缺省 N=5）；`n` 非法（0/负数/非数字）保守计 1 并输出降级警告；payload 不可解析或缺 `ok` 字段的行跳过并输出一次性降级警告；验证：fixture（6 失败 + 1 锚点 + 2 采样 `n=5`）报告还原执行次数 17、失败率 6/17，且报告显式标注口径（白盒 T1-T7，故事 A2）
- [x] 2.2 门禁失败聚类段：按命令、按 domain 包（从命令提取包路径）、按 diag 特征归并为簇，簇内计数排序取 top-N；单条 diag 展示截断 ≤120 字符；验证：fixture 含编译失败族与测试失败族的多个包时，输出按簇归并而非平铺清单（故事 A5）
- [x] 2.3 回归翻转段与重复失败热点段：翻转段统计同一 `(session, cmd)` 由绿转红（`flip`）的次数与占比；热点段统计同一会话内同 `cmd` + 同 `diag` 的连续重复次数 ≥ 阈值者（死循环信号）；验证：fixture 构造连续重复 6 次同 diag 的失败 → 出现在热点段并显示连续次数（白盒 T10）
- [x] 2.4 注入面健康段：`constraint.inject` 按文档路径聚合命中次数与字节合计，输出「零命中文档」清单（死约束候选）与字节分布的 top-N；验证：fixture 含 3 个文档的注入事件（其中一个零命中）→ 该段列出零命中文档（故事 A1 第⑥段）
- [x] 2.5 change 归属限定与查询安全：`--change <name>` 使各段仅由 `change` 列等于该值的事件构成（空值等价全局）；所有取值经参数化或严格转义，MUST NOT 字符串拼接进 SQL；验证：`--change "x'; drop table events; --"` 执行后 `select count(*) from events` 与执行前一致（白盒 T8、故事 A4）
- [x] 2.6 数值展示口径：失败率以 `awk` 计算整数百分比（禁用 bash 整数除法）；某 `(session, cmd)` 失败数为 0 时只输出执行次数、不输出失败率（避免除零）；验证：fixture 只含成功事件 → 输出无 `0%` 类伪值；含 1 失败 0 成功 → 显示 100%（白盒「聚合与展示边界」）

## 3. 故障分离与软提醒失效（spec：harness 自身故障与 agent 失败分离 / 软提醒失效判定）

- [x] 3.1 故障段分离：故障段只收 `policy.decision` 中 `action=fail-open` 的事件（如 `interop-down` / `quota-query-failed` / `ui-gate-check-failed`），并按 `policy + reasonCode` 聚合；`block` / `warn` / `bypass` 一律不入该段（`warn` 归第④段，`block` 单独计数但不进 agent 失败分母）；报告 MUST 标注「fail-open 已从 agent 失败中排除」；验证：fixture 仅 3 条 fail-open、零门禁失败 → 故障段 3、失败段 0 且带排除标注；再混入 `block`/`bypass` → 二者不出现在故障段（故事 B1/B2）
- [x] 3.2 软提醒失效段：按 `policy + reasonCode` 聚合 `action=warn`，计数 ≥ 阈值（缺省 5，可 `--warn-threshold` 调整）者列出并显示次数，低于阈值者 MUST NOT 出现在任何分段；阈值非法已在 1.1 拒绝；验证：fixture 一组 warn 计数=5（边界命中）、一组=4（不出现）（故事 B3/B4、白盒「阈值边界」）

## 4. 方法论交付与仓库注册（spec：复盘方法论随脚本交付）

- [x] 4.1 新增 `.agents/skills/harness-retro/SKILL.md`：覆盖「何时跑」（change 归档后 / 定期 / 怀疑某规则无效时）、命令用法与参数表、六段报告的逐段解读、**改进项产出判据**（每条改进项绑定一个可回检指标 + 观察窗口；仅出现在单个 session 的偶发事件不得直接升格为规则；针对单一任务过优化的规则会损害泛化）、以及与本仓库考古能力 `harness-facts` 的分工（前者「从数据找改进项」，后者「当时为什么注入了这条」）；验证：文档含上述五节，且参数表与 `scripts/harness-retro.sh --help` 输出一致（人工比对）
- [x] 4.2 注册引用防孤立：根 `AGENTS.md` 在 harness 相关段落（pi 扩展全景表之后的约束注入/门禁说明附近）与技能清单处引用 `harness-retro`；`.agents/skills/harness-facts/SKILL.md` 补「与 harness-retro 的分工」交叉引用；验证：`grep -rn 'harness-retro' AGENTS.md .agents/skills/harness-facts/SKILL.md` 均命中（故事 D1）
- [x] 4.3 `docs/reference/开发执行规范.md` 的归档流转一节（§12 附近）补一句可选步骤：归档后可选跑 `bash scripts/harness-retro.sh --days 7`（或与基线比对）观察该类事件是否下降；明确 MUST NOT 接 hook（不新增运行时耦合）；验证：`grep -n 'harness-retro' docs/reference/开发执行规范.md` 命中，且该段说明其为**可选人工步骤**（无「必须」「自动」措辞）
- [x] 4.4 完工汇报（AGENTS.md 要求）：说明 (a) 用户可见行为变化（无产品界面变化；新增一条终端命令）、(b) 需要用户手动执行的操作（无；首次运行自动生成基线，可随时删除）、(c) 旧数据降级（无数据迁移；events.db 不受影响）；验证：汇报文本含 (a)(b)(c) 三节

## 5. 测试

- [x] 5.1 新建 `scripts/harness-retro.smoke.sh`（fixture 驱动，断言风格对齐 `scripts/scenario-trace.smoke.sh`：退出码 + 关键字）：用 `sqlite3` 在 `mktemp -d` 下建同构库（`application_id=0x53594E54` + `events` 表 + 三组索引）并插入事件，旁置真实被测脚本逐 case 断言；验证：`bash scripts/harness-retro.smoke.sh` → 通过数 / 失败数打印，退出码 0
- [x] 5.2 用例覆盖故事 A-D：六段齐备且各带指标、分母加权还原（含缺 `n`、非法 `n`、不可解析行）、失败特征归并、`--change` 归属限定、基线生成与差值、基线损坏降级、故障分离（fail-open 排除 / block 与 bypass 不入段）、软提醒阈值边界（=5 命中 / =4 不出现）、只读性、窗口超期与常规、缺库 / 他库 / 空库 / 有失败仍 exit 0；验证：每条 case 打印 case 名与断言结果，失败时输出关键片段便于定位（26 条 spec Scenario 中 24 条落本 smoke，2 条人工见 7.8/7.9）
- [x] 5.3 只读性机械断言并入 smoke：运行前后 `sha256sum` 比对库文件与已存在 sidecar、`select count(*) from events` 一致、`git status --porcelain .pi/` 零变化；验证：人为在脚本里加一条 `INSERT` 时 smoke 必然失败（反向验证断言有效）
- [x] 5.4 注入安全用例（T8）：`--change` 值含 SQL 元字符时库行数不变、退出码仍为 0（正常报告）；验证：smoke 内含该 case 且断言 `count(*)` 前后一致

## 6. 文档

<!-- doc-impact: none(纯 harness 工具链改动：新增只读报告脚本 + 冒烟测试 + agent 技能文档，并更新根 AGENTS.md / 开发执行规范的流程措辞；不涉及业务链路、API、数据库、架构、代码规约、配置、部署任一域文档的语义变更，产品行为与用户界面零变化) -->
<!-- doc-impact-excuse: flow/api/database/standard/configuration/deployment=启发式命中全部来自**其他在途 change 的脏文件**（topicgraph/service/daily_report_article_filter*.go、reader/handler/feed_handler.go、platform/articlerefs/*、configs/config.yaml、init.sh、standard/backend/testing.md 等，见 `bash scripts/concurrency-status.sh` 归属地图），本 change 未触及其中任一文件 -->

- [x] 6.1 文档域声明对账：归档前重跑 `bash scripts/doc-impact.sh suggest openspec/changes/harness-retro-loop`，确认「none」声明与启发式命中的差异全部由他 change 脏文件解释；验证：`bash scripts/doc-impact.sh verify openspec/changes/harness-retro-loop` 无 FAIL（excuse 覆盖的域不判 FAIL）
- [x] 6.2 知识库一致性：根 `AGENTS.md`（4.2）与 `docs/reference/开发执行规范.md`（4.3）的引用在同一措辞下互指同一命令；验证：`grep -rn 'harness-retro' AGENTS.md docs/reference/开发执行规范.md .agents/skills/harness-retro/SKILL.md .agents/skills/harness-facts/SKILL.md` 四处命中且命令文本逐字一致
- [x] 6.3 无 flow 变更溯源需求：本 change 不新增/修改任何 `docs/reference/flow/` 文档，故 §12 变更溯源表无需追加；验证：本 change 实际改动文件仅为 `openspec/changes/harness-retro-loop/**`、`scripts/harness-retro.sh`、`scripts/harness-retro.smoke.sh`、`.agents/skills/harness-retro/SKILL.md`、`.agents/skills/harness-facts/SKILL.md`、`AGENTS.md`、`docs/reference/开发执行规范.md` 七类（`check-standards.sh` F 段 doc-impact 声明 none 通过、154/0）；共享工作树上 `git status -- docs/reference/flow/` 的命中属其他在途 change（内容为 feed 级联删除 / 单元话题等，非本 change 主题）

## 7. 验证

- [x] 7.1 `bash scripts/harness-retro.smoke.sh` → 期望打印全通过（失败数 0）、退出码 0
- [x] 7.2 `bash scripts/harness-retro.sh --days 7`（真实账本）→ 期望六段齐备、退出码 0；并用独立 SQL 复算至少一项指标核对一致，例如失败条数：`sqlite3 .pi/harness/events.db "select count(*) from events where kind='gate.check' and json_extract(payload,'\$.ok')=0 and date(ts)>=date('now','-7 day');"` → 期望与报告数值一致；同时核对加权分母 > 落库条数（证明采样已还原）
- [x] 7.3 `bash scripts/harness-retro.sh --days 45` → 期望出现「可能已被 TTL 清扫」提示；`bash scripts/harness-retro.sh --days 30` → 期望无该提示
- [x] 7.4 `time bash scripts/harness-retro.sh --days 7`（真实账本）→ 期望耗时 < 1s（纯 SQL 聚合）
- [x] 7.5 基线回检演示：`bash scripts/harness-retro.sh --save-baseline` 后再次运行 → 期望输出「较基线 ±」列（无变化时差值为 0）；验证：`.pi/harness/retro-baseline.json` 生成且可 `python3 -m json.tool` 解析
- [x] 7.6 `openspec validate harness-retro-loop` → 期望 valid
- [x] 7.7 `bash scripts/scenario-trace.sh openspec/changes/harness-retro-loop` → 期望退出码 0（下表 26 条 Scenario 全部有映射）
- [x] 7.8 `bash scripts/doc-impact.sh verify openspec/changes/harness-retro-loop` 与 `bash scripts/check-standards.sh --change harness-retro-loop` → 期望无 FAIL
- [x] 7.9 人工核对（无法脚本化的两条 Scenario）：「方法论文档存在且被注册引用」→ `grep -rn 'harness-retro' AGENTS.md .agents/skills/harness-retro/SKILL.md` 命中（实际 4 处注册/引用点）；「判据可与报告指标对齐」→ 按 skill 流程产出 4 条改进项并逐条落到指标名上：
  - ① `harness.fail_open`：42 条 fail-open 全为 `quality-gate/interop-down`（平台分流前的环境故障）→ 期望值 0，观察窗口 7 天（用 `--save-baseline`/`--baseline` 回检）
  - ② 族计数「并发/环境冲突」：5 条 `parallel golangci-lint is running`（属并发会话撞锁，非代码问题）→ 已在并发 change 中加 `--allow-parallel-runners`，回检该族归零
  - ③ `soft.stale_groups` / `soft.warn_total`：`test-scope-guard/full-go-test` 26 次 ≥ 阈值 5 → 软提醒未被采纳（升级为 block 需用户确认，因其改变行为）
  - ④ `inject.zero_hit_docs`：4 篇索引内文档窗口内零命中（死约束候选，如 `standard/backend/ai-logging.md`）→ 逐个判定是「真死约束」还是「触发关键词缺失」

| Scenario | 测试文件 |
|---|---|
| 只读打开不产生任何写入 | scripts/harness-retro.smoke.sh |
| 库不存在时 fail-loud | scripts/harness-retro.smoke.sh |
| 拒绝非本应用的库 | scripts/harness-retro.smoke.sh |
| 按锚点与采样权重还原分母 | scripts/harness-retro.smoke.sh |
| 采样条缺 n 时回退缺省权重 | scripts/harness-retro.smoke.sh |
| 非法采样权重保守计 1 | scripts/harness-retro.smoke.sh |
| 不可解析行跳过且告警 | scripts/harness-retro.smoke.sh |
| 报告显式标注口径 | scripts/harness-retro.smoke.sh |
| 六段齐备且各带指标 | scripts/harness-retro.smoke.sh |
| 失败特征归并 | scripts/harness-retro.smoke.sh |
| 按 change 归属限定 | scripts/harness-retro.smoke.sh |
| fail-open 不计入 agent 失败 | scripts/harness-retro.smoke.sh |
| 阻断类裁决不混入故障段 | scripts/harness-retro.smoke.sh |
| 超阈值提醒纳入 | scripts/harness-retro.smoke.sh |
| 阈值边界按「等于即命中」 | scripts/harness-retro.smoke.sh |
| 低于阈值不产生噪声 | scripts/harness-retro.smoke.sh |
| 生成基线与差值 | scripts/harness-retro.smoke.sh |
| 基线损坏时降级 | scripts/harness-retro.smoke.sh |
| 超期窗口给出提示 | scripts/harness-retro.smoke.sh |
| 常规窗口无提示 | scripts/harness-retro.smoke.sh |
| 空库不产出伪指标 | scripts/harness-retro.smoke.sh |
| 有失败但退出码仍为 0 | scripts/harness-retro.smoke.sh |
| 运行后账本零新增 | scripts/harness-retro.smoke.sh |
| 不触碰注入配置 | scripts/harness-retro.smoke.sh |
| 方法论文档存在且被注册引用 | 人工（7.9 grep 注册引用命中） |
| 判据可与报告指标对齐 | 人工（7.9 按 skill 产出改进项并落到指标名） |
