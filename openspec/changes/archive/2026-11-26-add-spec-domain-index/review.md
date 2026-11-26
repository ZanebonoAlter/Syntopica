# Review — add-spec-domain-index

> 基线：工作树未提交快照（2026-11-25 23:46）。审查期间 tasks.md / stage.md 被更新（23:29：1.2–8.5 已勾、stage=verify），以下按更新后版本核对。只读审查，未改任何仓库文件；fixture 验证均在 `/tmp` 下用 `DOMAIN_HISTORY_ROOT` 或抽取的 J 段副本完成。

## 核实记录（通过项，供免重查）

- 词表：`scripts/harness/domain-history.sh:28-29` 的 10 个业务域与 `docs/reference/flow/*.md` basename 逐项一致（不含 README），5 个工具域与 spec:18 一致；`--list` 15 项 = spec 词表 15 项 = doc-authoring.md:33 的「10 + 5」。
- marker 全量（脚本比对 177 个，非抽样）：177/177 有 marker；词表外 0、超 3 域 0、marker 超出前 8 行 0、位于首个 `#` 之后 0；与定稿表不一致 1 条（R-2）。
- 两跳并集：:170 `IFS=', '` 只按逗号/空白切分，`semantic-board` 不会被切成 `semantic` + `board`；fixture 里标 `semantic-board, topic-graph` 的 capability 同时命中两个域，`board` 作为查询词被「未知域名」拒绝。
- 解析边界（fixture 实测）：`<!--domain: x-->`（无空格）、`a,b`（无空格）、首行为空行、`<!-- constraint-domains: ... -->` 写在 marker 之前，均解析正确；archive 目录 283 个全带日期前缀。
- 归档合并保留 marker：openspec 2.1.1 `dist/core/specs-apply.js:308,492` 重建主 spec 时 `## Requirements` 之前的内容（含头部注释）原样保留；`openspec validate --specs` 177 个全过。
- 账面数字：283 个 archived change、526 条 delta、488 可推导、38 孤儿，与 proposal / design 一致。
- J 段基本行为：新 capability 无主 spec → WARN 不 FAIL；主 spec 缺 marker → WARN；marker 在第 9 行 → WARN。`bash scripts/harness/check-standards.sh --change add-spec-domain-index` → 通过 336 / 失败 0 / 警告 1。
- smoke 15 项通过；`bash -n` 无语法错误；`openspec validate add-spec-domain-index --strict` valid。
- D1–D6 均有落点（D1 marker + doc-authoring 行；D2 词表常量；D3 两跳；D4 五段 + 退出码 0；D5 草表；D6 J 段），缺口只列在下面 R-3 / R-4 / R-6 / R-7。
- 未立项小项（备查，不计入条目）：:199-201 digest 失败分支不可达、fixture 根下业务域会打印「工具域无 flow 红线」；无日期且含 ≥3 个连字符的 archive 名在 :219 / :239 会被截坏（现无此类目录）；proposal 写「仅加一行」实为 +2 行（marker + 空行）；smoke 未覆盖第 3 段（fixture 无 digest）。

### R-1 2026-11-26【High】时间线「[含孤儿 delta]」标记恒真：`${has_orphan:+}` 判非空不判真
- **状态**：已修复
- **修复**： `has_orphan` 改为空串/1 两态（:155/:179）；smoke ⑨ 重做为按行断言 + fixture 加真实孤儿 delta，新增「无孤儿 change 不带标记」反证；旧的全输出 grep 断言已注释掉
- **位置**：scripts/harness/domain-history.sh:155、:163、:179（:182 的 `orphans` 只累加、从不输出）；scripts/harness/domain-history.smoke.sh:153-155
- **问题**：`has_orphan=0` 是非空串，`${has_orphan:+|orphan}` 照样展开（最小复现：`h=0; echo "x${h:+|orphan}"` → `x|orphan`）。结果第 2 段每一行命中 change 都带标记：`bash scripts/harness/domain-history.sh semantic-board` 12/12 行全带，例如 `retire-person-extraction`（唯一 delta 是 `tagging-domain`，有 marker）。标记因此零信息量，而它是 pi-extensions.md:44 宣传的输出特征。smoke ⑨ 是假阳性通过：fixture 里没有「命中且含孤儿」的 change（孤儿只在独立的 `2026-01-04-orphan`，它根本不进时间线），「含孤儿 delta」断言靠的正是这个 bug；同组 `check_absent "已退役能力"` 把 ADDED 标题交给只读 REMOVED 的第 4 段，即使去掉 :235 的过滤也不会出现，等于没测过滤。
- **建议**：:179 改按值判断（`has_orphan` 置空 / `1` 后沿用 `:+`，或先算 `flag` 再拼接）。smoke ⑨ 重做 fixture：在一个命中 change 下追加孤儿 delta（放 REMOVED 段），断言该 change 带标记、另一个无孤儿 change 不带标记（按行 `check_absent`），且孤儿的 REMOVED 标题不进第 4 段。

### R-2 2026-11-26【High】落盘 marker 与定稿表不一致：`section-relations`（177 个全量比对唯一一条）
- **状态**：已修复
- **修复**： 人工裁定：以 marker 为准（DailyReportSection 属日报），草表改为 `daily-report, topic-graph` 并记理由；全量对账命令写进 tasks 8.6，当前 diff 无输出 = 177 条一致
- **位置**：openspec/specs/section-relations/spec.md:1（`<!-- domain: daily-report, topic-graph -->`）≠ openspec/changes/add-spec-domain-index/spec-domains-draft.md:141（`topic-graph`，h）
- **问题**：复现命令如下，输出 `133c133`。根因迹象：该文件 marker 的 mtime 是 23:00（首批 4 个之一），其余 171 个是 23:22 批量写入；任务 2.1 的幂等规则「已有 marker 的跳过」使首批写入后草表的改判不回写。marker 值反而更贴 Purpose（「DailyReportSection 之间的…关系」「日报生成管线中」），更像草表行滞后。tasks 2.1 / 2.2 已勾「定稿」「校验」，但目前没有任何机械对账能发现这类漂移。

```bash
diff <(grep -E '^\| [a-z0-9-]+ \| [0-9]+ \| ' openspec/changes/add-spec-domain-index/spec-domains-draft.md \
        | awk -F' \\| ' '{sub(/^\| /,"",$1); print $1"\t"$3}' | tr -d ' ' | sort) \
     <(for f in openspec/specs/*/spec.md; do printf '%s\t%s\n' "$(basename "$(dirname "$f")")" \
         "$(sed -n '1,8{/<!--[[:space:]]*domain:/{s/.*domain:[[:space:]]*//;s/-->.*//;s/[[:space:]]*$//;p;q}}' "$f")"; done | tr -d ' ' | sort)
```

- **建议**：人裁一次该 capability 的归属（倾向把草表行改成与 marker 一致并记一句理由）；把上面的对账命令写进 tasks 8.x 验证节长期保留。

### R-3 2026-11-26【Medium】孤儿 delta 没有清单：spec Scenario / design D3 / doc-authoring 三处承诺无落点
- **状态**：已修复
- **修复**： 实现第 6 段「孤儿 delta」：逐条列 change + 孤儿 capability + 补标方式，当前 9 个 change；design D3 数字更正为 53%（151/283，含孤儿者仅 5 个有声明）
- **位置**：openspec/changes/add-spec-domain-index/specs/spec-domain-index/spec.md:52-55；design.md:19-23 与 :53「首版就在 orphan 清单里显式列出」；docs/reference/standard/shared/doc-authoring.md:33「其历史 delta 在第 2 段计为孤儿」；scripts/harness/domain-history.sh:147、:155-183
- **问题**：(a) 脚本没有任何 orphan 输出。现状 24 个 archived change 带 38 条孤儿 delta（23 个已退役 capability）：其中 9 个 change 的 delta 全是孤儿，15 个域的输出里都不会出现（以 `2026-04-29-tree-bridge-phase` 逐域验证为 0 命中）；其余 15 个只是把孤儿 capability 名混在时间线的 caps 列表里，不指明哪个是孤儿。`$orphans` 计数从未打印。(b)「由人工补标」没有机制：脚本不读 archive change 的任何域声明，也没有覆盖入口。(c) D3 论据「实测只有 8% 有 constraint-domains」失实：`grep -lE '<!--[[:space:]]*constraint-domains:' openspec/changes/archive/*/proposal.md | wc -l` = 151/283 = 53%。两跳推导的结论仍站得住（24 个带孤儿的 change 里只有 5 个有声明），但设计记录要更正。
- **建议**：二选一并同步三处制品。(1) 实现：加第 6 段「孤儿 delta」，列 change 名 + 孤儿 capability；(2) 降级：Scenario 改为「仅在时间线对命中 change 标注」，doc-authoring 行同步，D3 数字改 53%、「人工补标」改成实际情况。

### R-4 2026-11-26【Medium】J 段只查「有没有」：空值、词表外值、超 3 域都判 OK；tasks 2.2「校验脚本」已勾但不存在
- **状态**：已修复
- **修复**： J 段增值校验（词表/空值/≤3 域），实测把 branding 改成 `board` 会 WARN；无 delta spec 改中性 INFO 不计分；doc-authoring 与 spec Scenario 措辞对齐实现
- **位置**：scripts/harness/check-standards.sh:305-327；specs/spec-domain-index/spec.md:32-35（「词表外的值被拒…校验脚本报错」）；tasks.md:12（2.2 已勾）；docs/reference/standard/shared/doc-authoring.md:33（「词表外域名 → 静默少结果」）
- **问题**：`marker_of` 只判断前 8 行有没有含 `domain:` 的行。复现：

```bash
R=$(mktemp -d); mkdir -p $R/openspec/specs/{badval,emptyval} $R/openspec/changes/c1/specs/{badval,emptyval}
printf '<!-- domain: board -->\n# x\n' > $R/openspec/specs/badval/spec.md; printf '<!-- domain: -->\n# y\n' > $R/openspec/specs/emptyval/spec.md
{ echo 'set -u; cd "$1"; CHANGE_NAME="$2"; PASS=0; FAIL=0; WARN=0; ok(){ echo "[OK] $1"; }; fail(){ echo "[FAIL] $1"; }; warn(){ echo "[WARN] $1"; }'; sed -n '304,336p' scripts/harness/check-standards.sh; } > $R/jseg.sh
bash $R/jseg.sh $R c1   # → [OK] 本 change 触及的 capability 均已声明 domain
```

  契约源写「报错并要求改用词表」，实现与 doc-authoring 都是静默放过，三方不一致；多域 ≤3 同样无人校验。`ls scripts/harness | grep domain` 只有 domain-history.sh 与其 smoke，2.2 所说的校验脚本没有落地（当前 177 个实测词表 / 数量都干净，但没有可复跑的检查）。另：无 specs 目录的 change（如 `add-home-pipeline-status-page`）也打同一句 OK 且 PASS +1，空集不该计分。
- **建议**：J 段在 `marker_of` 后加值校验（按 `,` 切分、trim、对 15 词表精确比对、数量 ≤3），非法值 WARN 并列出（仍不 block，符合 D6）；无 specs 目录改中性提示、不计分；doc-authoring 行措辞与实现对齐。要么补出 2.2 的校验脚本，要么把 2.2 改述为「由 J 段承担」。

### R-5 2026-11-26【Medium】第 2 / 5 段数量口径与契约、标题对不上
- **状态**：已修复
- **修复**： 第 5 段按 change 日期倒序 head -15（实测 232 条→显示 15）；第 2 段 Scenario 改为「总数 + 最近 12 条」；标题写实数
- **位置**：scripts/harness/domain-history.sh:186-192（`head -12`）、:226-227（标题「最近 15 条」）、:228-246（无截断）；specs/spec-domain-index/spec.md:47-50（「包含所有…archived change」）
- **问题**：`bash scripts/harness/domain-history.sh semantic-board` 第 5 段输出 232 条（标题说 15），顺序是目录遍历 = 日期升序（2026-05-26 → 2026-11-25），最旧的排最前，与第 2 段倒序相反；605 行输出里 232 行堆在第 5 段，开工前阅读基本被淹没。第 2 段 Scenario 写「清单包含所有」，实际 117 个只列 12 条且没有参数放开。
- **建议**：第 5 段按 change 日期倒序后取 15（或把标题改成实数）；第 2 段加 `--all`，或把 Scenario 改成「计数 + 最近 12 条」。

### R-6 2026-11-26【Low】勾选与留痕对不上：1.2 两个 l 档无裁决记录、1.3 证据 12 条 < 任务自定的 ≥15、草表统计过期
- **状态**：已修复
- **修复**： 两个 l 档已裁决（otel-business-tracing 维持 ai-summary、topic-watch 主域 semantic-board）；6 个 Purpose 缺省的 h 降 m；统计重算为 h=66 m=111 l=0；review 自抽 18 个 h 档 18/18 已记入 stage.md
- **位置**：tasks.md:6-7；stage.md:8（「抽查 12/12 正确」）；spec-domains-draft.md:192-193（`otel-business-tracing`、`topic-watch` 仍写「暂判」）、:202（「h=71 m=103 l=3；多域=71」，表内实为 72 / 103 / 2、多域 72）
- **问题**：D5 的「人工只复核 l 与 ?」在草表里看不到结论（对照 `board-signal-reports` 有「已定」）。按草表自己的规则「Purpose 缺省的一律不给 h」（:5），`api-docs`、`architecture-docs`、`configuration-docs`、`database-docs`、`development-docs`（Purpose 以 TBD 开头）与 `concurrent-change-coordination`（无 Purpose 节）共 6 行应降 m。补充：本次抽 18 个 h 档（agent-guide-slim、api-docs、analysis-method-library、board-health-checkup、bipartite-relation-matching、daily-report-system、desktop-shell、feed-candidate-catalog、frontend-interaction-contract、research-data-sources、section-lifecycle、thread-lineage、tagging-domain、test-infrastructure、workshop-board-console、lane-heat-metric、unified-dialog、board-signal-reports），Purpose 与主域 18/18 自洽——结论不受影响，只是记录要补齐。
- **建议**：给两个 l 档各补一句裁决；stage.md 的抽查数改成实际样本或补抽到 ≥15；更正统计与 6 行置信。

### R-7 2026-11-26【Low】登记面四处缺口
- **状态**：已修复
- **修复**： doc-authoring 门禁索引加 J 行、§11.4 提 J 段、pi-extensions 头部辖区加 domain-history、tasks 补 7.4「归档后给本 change 的 main spec 补 marker」
- **位置**：docs/reference/standard/shared/doc-authoring.md:114-125（门禁索引无 J 行，而同文件「维护提醒」要求改 check-standards 段语义必须同步门禁索引）；docs/reference/开发执行规范.md:417（§11.4 段清单仍是「A-D + F + G」）；docs/reference/harness/pi-extensions.md:4（头部 `doc-impact-applies` 列了 constraint-digest / change-scope / change-abandon，没列 `scripts/harness/domain-history.sh`，以后改脚本不会把该文档带进路由清单）；tasks.md 全文（无「归档后给 `openspec/specs/spec-domain-index/spec.md` 补 marker」条目）
- **问题**：最后一条会自咬——openspec 2.1.1 给新 capability 生成的骨架不带 marker（`dist/core/specs-apply.js` 的 `buildSpecSkeleton`，delta 的头部注释也不会搬运），归档后下一次全仓 J 段会对本 change 自己 WARN。
- **建议**：四处各补一行。

### R-8 2026-11-26【Low】第 4 / 5 段节头大小写敏感，2 个归档 delta 的 MODIFIED 被静默跳过
- **状态**：已修复
- **修复**： REQ_TITLES_BY_SECTION 改 tolower 比较；smoke 新增 ⑪「Modified（大写 M）也被扫到」
- **位置**：scripts/harness/domain-history.sh:88；openspec/changes/archive/2026-09-28-add-datasources-test-shard/specs/test-debt-patrol/spec.md:3、openspec/changes/archive/2026-09-28-split-retro-warn-classes/specs/harness-retro-loop/spec.md:3（均为 `## Modified Requirements`）
- **问题**：正则 `^## MODIFIED Requirements` 区分大小写，`dev` / `harness` 域查不到这 3 条 MODIFIED 标题。
- **建议**：比较前对节头 `tolower`，或让 `want` 同时匹配两种写法。

### R-9 2026-11-26【Low】性能可接受；负载敏感来自逐文件 fork，只标优化位置
- **状态**：已采纳
- **修复**： 不优化：空闲 4.7s / 负载 23.5s，不在门禁链路，开工前一次性查询可接受。优化点已在报告里标位，留待变慢再动
- **位置**：scripts/harness/domain-history.sh:135 与 :81-83；:128 与 :70；:75；:198-199；:216-220 / :236-240
- **问题**：同一命令空闲 4.7 s、机器负载高时 23.5 s（brief 的 9.4 s 在区间内）。不在门禁链路（archive-readiness / spec-gate 只调 check-standards），作为开工前手动工具可接受。耗时构成（xtrace 实测 11.2 s）：第 1 段 4.6 s——每个 in-domain spec 一次 `git log`（52 次 ≈1.0 s，改一次 `git log --name-only -- openspec/specs` 全量只要 0.16 s），同一文件的 `spec_domain_raw` 在 :128 与 :70 各跑一遍（354 次 sed），:75 每个域一次 `echo | sed`；:199 的 constraint-digest 单次 4.2 s（负载高时最大单块）；第 4 / 5 段每个 delta 一次 awk（428 次）。
- **建议**：按上列顺序处理即可，前两项不动结构；digest 可挪到第 4 / 5 段之后或给开关。

### R-10 2026-11-26【Low】E 段溯源豁免与 doc-impact 声明不对齐（顺带修复）
- **状态**：已修复
- **位置**：scripts/harness/check-standards.sh 的 E 段豁免判据
- **问题**：E 段只认 tasks.md 里字面「无 flow 影响」，而纯工具链 change 常规写法是 `<!-- doc-impact: none(理由) -->`（如 `2026-11-23-fix-editmap-deploy-artifact-pollution`），语义等价却被判「未溯源」，阻断本 change 归档。
- **修复**：豁免判据增加机器可读等价形式 `<!-- doc-impact: none`；check-standards smoke 仍 19/19，`2026-11-23-fix-editmap...` 现判「豁免溯源」。

- 放行判断：需改后进 ｜ R-1 是确定性错误输出且 smoke 假阳性，R-2 是落盘 marker 与定稿表已漂移而对账缺位；两条修掉、R-3 / R-4 各自收口（实现或改制品）后可进。
