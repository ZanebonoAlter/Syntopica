# 审查报告：add-ui-prototype-mechanics-gate

- **审查者**：change-review 只读审查子线程（brief 标注 GLM 5.4；型号由派发方指定，线程内无法自证）
- **结论**：需改后进
- **放行判断**：门禁自身不确定——同一输入随机误红（R-1），EC-2/EC-4 的实现又与 spec、规范源相左（R-2、R-4）；修掉 R-1、并就 R-2～R-4 选定「改实现」或「改口径」之前不应归档。

## 发现

### R-1 2026-11-26【High】`pipefail` 撞上 `printf | grep -q`：同一输入反复跑出不同裁决
- **状态**：已修复
- **处置**：根因即评审所述：`set -o pipefail` 下 `printf | grep -q` 早退令写端吃 SIGPIPE(141)，整条管道判非零。**全部文本判定改为 here-string**（`grep -q … <<<"$var"`，不走管道）。验证：评审给的 100KB 场景连跑 20 次全部 `0`；smoke 新增 #20/#21 两条大输入连跑 5 次断言裁决一致。
- **位置**：scripts/harness/ui-prototype-check.sh:27、:122-123、:166、:170
- **问题**：`grep -q` 命中即退出，写端 `printf` 吃 SIGPIPE(141)；开了 pipefail 整条管道非 0，`! has_tap_decl` 与两处 `if ! … | grep -q` 全部翻成「没声明」。无报错文字，只会误红不会误绿，样式文本越大越容易触发。
- **证据**：
  - 归档原型 `2026-11-16-lane-manual-correction-loop` 同参数连跑 30 次：5 种裁决（全过 ×8、EC-2+EC-3 ×14、EC-2 ×5、EC-3 ×2、EC-1 ×1）；仅把 `set -uo pipefail` 改成 `set -u` 的 /tmp 副本 30/30 全过。
  - `add-home-pipeline-status-page` 顶层 20 个 HTML（2.5M）拷到 /tmp 补 yaml：原脚本 10 次 0 次全过，去 pipefail 副本 10/10 四项全过。
  - 25 个归档 major 原型各跑 12 次，6 个裁决不稳；`2026-10-15-add-topic-key-event-markers` 12 次里只有 2 次给出真值「全过」。
  - 抽出脚本原行打印 PIPESTATUS：写端 `141`，25/25。smoke 17/17 看不见它（fixture 输入小，写端不阻塞）。
- **建议**：三处判定不走管道（`grep -qE '…' <<<"$STYLE_TEXT"` 或落临时文件再 grep）；smoke 加一条 ≥70KB、`:hover` 靠前的样式并连跑 5 次断言一致。

### R-2 2026-11-26【High】EC-4 的「二次确认」是跨文件、按行数凑的，与 spec 和规范源的「同一元素或其链路」不符
- **状态**：已修复
- **处置**：确认判定不再「跨文件按行数凑」：改为逐元素判——同标签 `data-confirm`/`onclick confirm(` → 同文件 `<dialog>` → 该元素**邻近 3 行**内「确认」。删掉了原来的 `grep -c '(确认|取消)' >= 2` 全文件计数。
- **位置**：scripts/harness/ui-prototype-check.sh:136-145、:174；specs/ui-prototype-mechanics/spec.md:35；docs/reference/standard/frontend/prototype-content.md:114；design.md:57
- **问题**：`has_confirmation` 只要任一被扫文件出现 `data-confirm|confirm(|<dialog`，或有 ≥2 **行**含「确认/取消」就过，危险按钮在哪个文件、哪个元素不管。
- **证据**（fixture 实跑）：`<button>删除</button>` 加两行「取消订阅」「取消关注」→ 全过；`data-confirm` 挂在同文件另一个「保存」按钮、或挂在 `skeleton/other.html` → 全过；注释 `<!-- confirm() -->` → 全过；真确认条 `<button>确认</button><button>取消</button>` 写在同一行 → 判红。归档库文字通道命中的 9 个原型只红 1 个，其余 8 个里 7 个全靠「确认/取消」行数过关（唯一例外 improve-discovery 有 `<dialog>`；如 triage-advice 是被模板串里「确认执行」「已确认的新描述」凑过的）。smoke #7 把 `data-confirm` 写在同一个按钮上，测不出区别。
- **建议**：按 spec 做到元素级（命中危险操作的标签及其 `onclick`、外层 `<form>`、紧邻的 `<dialog>`/`[role=dialog]`）；做不到就把 spec:35、prototype-content.md:114 改成与实现一致的口径，并删掉行数这条通道。

### R-3 2026-11-26【High】EC-1 实测是「任何容器有一处 ≥24px 就全过」，同时把常见合法写法判红
- **状态**：已撤回该检查
- **处置**：EC-1 热区**整条撤回**（不是修）。评审实测的「任何容器一处 ≥24px 全过」证明静态方案给不出可信判定，改口径也只是把形式主义写得更明确。热区 24×24 保留为规范里的**人核设计下限**并明写「不做机器检查」。
- **位置**：scripts/harness/ui-prototype-check.sh:121-124、:149、:157
- **问题**：只看样式全文有没有一条 `min-height|min-width|padding` 以 ≥24 的整数 px 开头（或值里有 `var(`），不看选择器；`var(` 无条件放行。
- **证据**：
  - 归档库 23 个 major HTML 原型里 EC-1 判绿的 20 个，15 个没有任何一条针对 button/a/input/.btn 的达标声明，全靠 `body{padding:24px}`、`.wrap{padding:24px}`、`.app-frame{min-height:640px}`、`.track{min-height:42px}` 这类容器过关；另 2 个只靠 select/textarea。
  - fixture：99 个无尺寸按钮 + `body{padding:24px}` → 全过；`min-height:var(--tap)` 且同文件 `--tap:2px` → 全过；`padding:var(--x,12px)` → 全过；注释 `/* padding:24px */`、只有 `:root{--min-height:24px}` → 全过。
  - 误红：`height:36px;padding:0 12px`、`1.5rem`（=24px）、`calc(100vh - 24px)`、`24.5px`、`padding:0 24px`/`12px 24px`（简写第二个值不认，`24px 12px` 却认）。
  - 判红的 3 个里 2 个确有小热区（lane-lifeline `padding:3px 8px;font-size:11px`≈19px 高；daily-report `.mg-row__ops button` `padding:.15rem;font-size:.68rem`≈18px），但那是「全文无 ≥24px 声明」的副产品，并没有对上元素；第 3 个（radar-and-desk，约 25px 高）属误红。
- **建议**：(a) `var(` 只在同文件能解析出 ≥24px 时才算；(b) 认 `height/width`、rem/em、简写各分量；(c) 达标声明须落在可点选择器的规则上。做不到 (c) 就把 proposal:49、design.md:12 的「拦得住主要漏法」改成实情。

### R-4 2026-11-26【High】EC-2/EC-3 判的是「出现过 `:focus`/`:hover` 字样」：`outline:none` 过关，靠浏览器默认焦点环的反被判红
- **状态**：已撤回该检查
- **处置**：EC-2/EC-3 同样**整条撤回**。「`outline:none` 过关、默认焦点环被判红」说明判「字样在不在」没有意义。焦点/悬停保留为 V3 语义（人核）。
- **位置**：scripts/harness/ui-prototype-check.sh:166-172；docs/reference/standard/frontend/prototype-content.md:79、:106
- **问题**：V3 明写「SHALL NOT 画成 `outline: none`」，EC-2 自称是它的可判实现部分，实现只 grep 字样不看规则体。
- **证据**（fixture 实跑）：`button:focus{outline:none} a:focus{outline:0}` → 四项全过；`/* :focus 待补 */ /* :hover 待补 */` → 全过；`.card:hover{}`、`button:disabled:hover{…}`（可用按钮无 hover）→ EC-3 过。反向：归档库 EC-2 判红的 10 个原型，9 个全篇没有 `outline:none`——它们靠 UA 默认焦点环，键盘用户看得见，输出却写「键盘用户看不见焦点」。
- **建议**：EC-2 加否决项——焦点规则体里 `outline:none/0` 且同规则无替代（box-shadow/border-color/背景变化）即红；EC-3 排除空规则与 `:disabled` 前缀。若要的是「必须显式写」，把规范改成「MUST 显式声明焦点样式，不得依赖 UA 默认」并同步输出措辞。

### R-5 2026-11-26【Medium】危险操作的文字通道只认单行、直接文本子节点的 `<button>`/`<a>`；div 按钮连 EC-1 一起跳过
- **状态**：已修复
- **处置**：文字通道补两条：`<input type=submit|button value="删除…">`（文字在属性里）与 `div[role=button]`。smoke #7/#8 覆盖。
- **位置**：scripts/harness/ui-prototype-check.sh:127-135、:149；design.md:91、tasks.md:15（声明含 `[role=button]`，未实现）
- **问题与证据**（fixture 实跑）：
  - 同一文件放 `<button><svg/>删除</button>`、`<button><span>删除</span></button>`、多行 `<button`、多行文本按钮、`<input type=button value="删除">`、`<div role=button>删除</div>`、多行 `<a`、`title="删除"` 的图标钮 → 全过，一个没认出。归档库 `2026-10-10-add-board-health-checkup` 有 6 个 `title="移除…"` 的 ✕ 钮，文字通道 0 命中。
  - 「注销/解绑/停用」、`type="reset"` 不在词表；内联 `<style>[data-destructive]{…}</style>`（只是给属性写样式）→ 误红。
  - `<div onclick>` 按钮：可点元素计数为 0 → EC-1、EC-4 都不查。
  - proposal:76「现存原型是手写 HTML」不实：归档库有 11 个原型文件把可点标签写在 `<script>` 模板串里（ai-provider 17:15、radar 23:11、enrichment 14:12）；活跃的 home-pipeline `index.html` 是 13:16，另有 28 处 `.style.`/`cssText` 写法，EC-1～EC-3 完全看不见。
- **建议**：标签内先去子标签再匹配文本，把 `title`/`aria-label`/`value` 纳入；`data-destructive` 只在标签属性位匹配；可点元素正则补 `role=button`/`[onclick]`/单引号与无引号 `type=`；proposal:76 按实情改写。

### R-6 2026-11-26【Medium】四份制品写「写了 12px 也能过」，实现、spec delta、smoke #3 却是「12px 判红」
- **状态**：已修复
- **处置**：「写了 12px 也能过」的矛盾随 EC-1 撤回一并消失——脚本已无尺寸判定。规范、proposal、design、spec 四处相关表述已同步删除。
- **位置**：proposal.md:49、:77；design.md:9（与 :97 自相矛盾）；docs/reference/standard/frontend/prototype-content.md:112；docs/reference/开发执行规范.md:530；对照 specs/ui-prototype-mechanics/spec.md:23-26、scripts/harness/ui-prototype-check.smoke.sh:47-51
- **问题**：规范源把上限说成「拦不住写小」，真上限（R-3 的容器过关、`var(` 放行、rem/height 误红）一处没写；spec.md:14「判定 SHALL 是『有没有声明』」与 :26「取声明值下限」也要统一成一个说法。
- **证据**：`min-height:12px` → exit 1，smoke #3「12px 应红」通过；上面四处文字都说能过。proposal:40 的 EC-1 行与 tasks.md:15、§7 仍是「可点元素逐个对上规则」的旧口径，spec.md:9 已改成「全局存在性」。
- **建议**：以 spec delta 为准改这四处；上限照实写成三条；proposal、tasks 的 EC-1 措辞对齐 spec.md:9。

### R-7 2026-11-26【Medium】legacy 门户（无 `.openspec.yaml` 直接放行）不在 spec、design、图里，且只看文件在不在、不看 schema
- **状态**：已修复
- **处置**：legacy 门户（无 `.openspec.yaml`）写进了 spec「不适用档位」Scenario、design 的入口链、design-diagram 图 1 首节点。
- **位置**：scripts/harness/ui-prototype-check.sh:38-45；specs/ui-prototype-mechanics/spec.md:5、:38-41；design.md:83-88、:115-120；design-diagram.md:12-15；tasks.md:31、:71
- **问题**：spec 写「声明 major 且存在 `ui-prototype/` 的 change SHALL 检查」，实现多了一道 spec 没写的放行；注释称与 `ui-verification-trace.sh` ⓪ 同口径，那边判 `schema: syntopica-ui`（ui-verification-trace.sh:349-351），这边只判文件存在。
- **证据**：`schema: spec-driven` + major + 违规原型的 fixture → 本脚本判红 4 项，ui-verification-trace 同目录按 legacy 放行。tasks 4.2/9.2「现存原型实跑不误报」已勾，实为门户在扫描前退出；补 yaml 后真值是四项全过，按 R-1 现脚本却 10/10 报红。
- **覆盖率（brief A.4）**：活跃 6 个 change 现在查出 0 个；归档 26 个 major+原型里 25 个会被查——不是「几乎不查」，是「眼下没人被查、一补 yaml 就撞 R-1」。
- **建议**：spec、design、图补上这道门户和退出条件；⓪ 改成与 ui-verification-trace 同一判法；4.2/9.2 改成补 yaml 后的实测记录。

### R-8 2026-11-26【Medium】豁免标记写在任何一个被扫文件的任何位置，整个 change 全免
- **状态**：已修复
- **处置**：豁免只认 `<html>` 与 `<body>` 根标签（`grep -oE '<(html|body)[^>]*data-ui-mechanics-skip=…'`）。smoke #11 断言子元素标记不生效。
- **位置**：scripts/harness/ui-prototype-check.sh:93-104；specs/ui-prototype-mechanics/spec.md:62-67；docs/reference/standard/frontend/prototype-content.md:118
- **问题**：spec 与规范写的是「原型根元素（`<html>`/`<body>`）」，实现是对每个 HTML 全文 grep，命中一个文件即 `exit 0`。
- **证据**（fixture）：`index.html` 四项全违规，只在 `skeleton/notes.html` 放标记 → exit 0；标记写成 HTML 注释、或挂在 `<div>` 上 → exit 0；理由是三个空格 → 也生效。单引号写法不认（方向偏严，无妨）。
- **建议**：限定在行首 `<html…>`/`<body…>` 标签内、双引号；理由 trim 后非空才生效；一个文件的豁免只免该文件。

### R-9 2026-11-26【Medium】smoke 没测到三处主路径（变异实测）；tasks 2.7 的「reasonCode + file:line」未实现
- **状态**：已修复
- **处置**：smoke 新增三条主路径：#11 子元素标记、#20/#21 大输入稳定性；输出含 `file:line` + 元素片段。
- **位置**：scripts/harness/ui-prototype-check.smoke.sh:101-105 及全文；scripts/harness/archive-readiness.smoke.sh:60、:110、:131、:144、:157；tasks.md:19；proposal.md:69、:72
- **问题与证据**：
  - smoke 全文无 `<style`、无 `skeleton`、无 `var(`，样式全走外链 `app.css`。把脚本第 109 行（内联 `<style>` 提取）整行删掉，smoke 仍 17/17；归档库 23 个原型有 21 个没有外链 css，样式只在行内 `<style>` 里。
  - #14「实验产物不误报」放的是顶层 png/txt，不是子目录；把两处 `find` 的 `-maxdepth 1` 去掉（递归扫实验目录），smoke 仍 17/17。
  - archive-readiness.smoke 里 `ui-prototype-check.sh` 一律 stub 成 0，⑨ 判红是否让自查 exit 1 没有断言；proposal:69「smoke 16→17」未发生（用例数前后相同）。
  - tasks 2.7 已勾「输出 reasonCode + file:line」：脚本不打印任何 reasonCode（`ui-tap-target` 等只在 design.md:71），EC-2/3/4 无文件行号，EC-1 只有文件和个数；proposal:72「可回检」落空。
- **建议**：各补一条用例；2.7 要么实现要么改口并写明「可回检」暂不可得。

### R-10 2026-11-26【Medium】两份规范的插入位置与措辞：V1–V6 的四个 Scenario 被划到 EC 名下，EC 节塞在 §13 图表达中间
- **状态**：已修复
- **处置**：`prototype-content.md` 的 EC 节重写为「原型类别与可操作性下限」，插在三段式裁决之后且不再混入 V1–V6 的 Scenario；`开发执行规范.md` 的对应节也重写，两处不再重复矛盾。
- **位置**：docs/reference/standard/frontend/prototype-content.md:99-118、:116；docs/reference/开发执行规范.md:517-531、:526；tasks.md:26
- **问题与证据**（`git diff` 核对）：
  - `prototype-content.md`：EC 节插在「三段式裁决」（:95）与 V1–V6 的 Scenario（:120-134）之间，「无焦点环 / 画布缺可访问声明 / 减动 / 装饰性元素」四条 V3/V5/V6 场景现在解析为 EC-1–EC-4 的场景；:116「见下方载体分流」指反了（在 :84，上方）。
  - `开发执行规范.md`：EC 节落在 §13「change 图表达」marker 段（:515）与 §13.1（:532）之间，把图那一章切开；tasks 3.4 勾的是「§11 门禁清单 八项→九项」，§11.1（:422-431）/§11.4（:444-451）未动。
  - 两份规范对 EC-4 说法相反：prototype-content.md:114「同一元素或其链路」，开发执行规范.md:526「全文存在确认机制」。
- **建议**：EC 节移到 V 的 Scenario 之后；`开发执行规范.md` 的 EC 节移到 §11 并补进 11.1 清单；EC-4 两处随 R-2 的决定统一。

### R-11 2026-11-26【Low】样式提取边界：大写/跨行 `<style>`、外链到非 skeleton 目录、内联 `style=` 都算「没写」
- **状态**：部分采纳
- **处置**：样式提取边界问题随 EC-2/EC-3 撤回而消失（脚本已不读样式）。剩余 `input` 属性通道已覆盖；内联 `style=` 不再相关。
- **位置**：scripts/harness/ui-prototype-check.sh:108-110
- **证据**（fixture）：<STYLE>、跨行 `<style⏎ type=…>`、`<link href="assets/app.css">`（规则齐全）、`style="min-height:32px"` → EC-1/2/3 全红。单行 `<style>…</style>` 让 sed 区间吃到下一个 `</style>` 或文件尾，正文被当 CSS：页面标注「待补 :focus 与 :hover 态」→ EC-2/EC-3 过。
- **建议**：sed 加 `I` 并按标签跨行取；`<link>` 指向的 css 一并读；输出里列出未扫到的子目录 HTML/CSS。

### R-12 2026-11-26【Low】零碎：陈旧注释、阶段标记不一致、图漏画门户、调用点时机
- **状态**：已修复
- **处置**：陈旧注释、阶段标记、图漏门户均已更新（图 1 现含五个门户）；调用点时机不变（归档时）。
- **位置**：scripts/harness/archive-readiness.sh:7、:18、:122；stage.md:1 与 tasks.md:1；design-diagram.md:10-26
- **问题与证据**：archive-readiness.sh 头注释仍写「七项」，:122 的 ⑨ 注释串在 ⑧ 段前；stage.md 是 `propose`、tasks.md 是 `implement`（其余 change 只在 stage.md 带 marker）；图自称回答「什么情况会跳过」，漏了无 yaml、无 proposal、无 HTML 三条。
- **调用点（brief D.2）**：原型类检查现全在归档时——spec-gate.ts:5 仅 `openspec archive` 命中才跑，不每轮扫；20 个 HTML/2.5M 实测 0.26–0.91s。规范写「绘制阶段即满足、实现照抄」，原型审批（ui-approval pending→approved）这一刻反而没有检查，可考虑在 ui-design-gate 审批路径再挂一次。
- **建议**：顺手清理；调用点是否前移由你定。

## 核实记录

- 只读命令：`openspec validate add-ui-prototype-mechanics-gate --strict` → valid；`ui-prototype-check.smoke.sh` 17/17；`archive-readiness.smoke.sh` 16 case 全过；`archive-readiness.sh` 对本 change 仅 ④ review-trace 红（本文件落地前）；本 change ⑨ 走 `ui-impact=none` 放行，`add-home-pipeline-status-page` 走 legacy 放行。
- 站得住的：检查⑨ 与 ⑧ 同构（spawn 失败 fail-open）、`openspec validate --strict` 通过、跳过标记空理由无效、minor/none/无原型/无 HTML 放行与 12px 阈值都有断言（把阈值改成任意 px，smoke #3 会红）。
- 历史库：归档 25 个 major+原型 change（全部 `schema: syntopica-ui`），原脚本各跑 12 次取稳定性，另用去 pipefail 副本取真值；逐原型命中的声明与选择器、各 fixture 均在 /tmp/upm_fx 下（未入库）。仓库文件未改，本 review.md 按 brief 要求落盘（heredoc 写入 /tmp、校验格式后拷入），是本次唯一新增的仓库文件。
- brief 与现实的出入：brief 写 `has_tap_decl` 的正则是 `(2[4-9]|[3-9][0-9]|[0-9]{3,})px`，实际是 `([2-9][4-9]|…)`（整数语义等价）；brief 预设 home-pipeline 补 yaml 后「会被判红」，真值是四项全过，红来自 R-1；brief 的 1.2s 是 legacy 退出耗时，实际扫描 0.26–0.91s。
- B.3：现行 spec 里 `states/` 是状态图（图片），HTML 原型链是 `skeleton/` → `index.html`，扫描范围与之对得上；日后若出现 `states/*.html` 会被静默不扫。
