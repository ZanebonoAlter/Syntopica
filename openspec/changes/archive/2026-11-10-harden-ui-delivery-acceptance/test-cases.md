# test-cases: harden-ui-delivery-acceptance

## 判据与边界

主线程已定判据：默认模式仍为存在性；v1 任意最新 fail 红，核心缺失/未覆盖红；不能转挂洗绿；必验故事与视口逐条绑定；旧非 pass 之后的 pass 须明确复验和新截图。子线程只读枚举已复核，以下白盒矩阵为实现前用例。全部文件/PNG/change fixture 在临时目录创建；不调用真实 archive、不改业务数据、不跑全量应用测试。

## 故事 S1：开发者区分证据齐备与合同适用范围

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 对有合法fail/未覆盖且实物齐全的fixture跑默认模式 | 默认模式仍只检查证据 | exit0，输出证据而非结果合格 | shell smoke | scripts/harness/ui-verification-trace.smoke.sh |
| 2 | 对老日期显式v1与边界后的major调用delivery | 试点显式启用合同 / 新 major 缺合同阻断 | 显式v1必查；新major缺合同红 | shell smoke | 同上 |
| 3 | 老major无marker、非法marker、非major/旧schema分别检查 | 存量未迁移明确降级 / 非法合同不作存量放行 / 非 major 与旧 schema 兼容 | 老major只说明证据；非法红；none/minor/旧schema保持 | shell smoke | 同上 |

## 故事 S2：必验故事没有做过不能靠其他图片完成

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | v1表列A-43，账本无该ID | 必验故事从未验过 | 点名缺A-43，exit1 | shell smoke | scripts/harness/ui-verification-trace.smoke.sh |
| 2 | 空表、重复ID、无故事、坏视口、重复合同节 | 合同空壳或行不合法 | 各自明确红，不静默丢行 | shell smoke | 同上 |

## 故事 S3：失败保留后通过明确复验解除

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 表格/标题混排、A1/A-1同号，读取历史和最新结论 | 标准条目解析 / 格式对齐实战形态 / 复验沿用场景锚点保留失败 | 同parser，同号按行序，历史计数保留 | shell smoke | scripts/harness/ui-verification-trace.smoke.sh |
| 2 | 必验/非必验最新fail，另建真实承接change | 任意最新失败阻断 | 全部红，不接受转挂 | shell smoke | 同上 |
| 3 | 核心与非核心未覆盖 | 核心未覆盖阻断而非核心明确提醒 | 核心红，非核心原因透传 | shell smoke | 同上 |
| 4 | 末尾追加带复验与新图的pass，再检查缺标识/重复旧图 | 追加有效复验后解除 / 无复验或旧失败截图不能解除 | 真结构闭环可绿，缺项红 | shell smoke | 同上 |
| 5 | pass后追加非法判定/空依据/畸形A条目 | 非法条目不得被忽略洗绿 / 判定词表外不合法 | strict红，默认统计兼容 | shell smoke | 同上 |

## 故事 S4：每个场景自己覆盖承诺视口

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | A-55最新依据点名两种真实尺寸PNG | 逐场景视口齐全通过 | 本行两视口均绿 | shell smoke | scripts/harness/ui-verification-trace.smoke.sh |
| 2 | 4K图只给A-57/全局清单，A-55名下没有 | 别的页面宽屏图不能借用 | A-55缺视口红 | shell smoke | 同上 |
| 3 | 1440文件名但1280x633头；坏PNG/截断/非PNG | 真实尺寸而非文件名判定 | 不满足视口，不猜 | shell smoke | 同上 |
| 4 | 相对路径合法子目录/缺失/绝对/越界/逃逸链接 | 截图路径不得越界 | 仅目录内真实证据可参与 | shell smoke | 同上 |

## 故事 S5：归档自查与硬门禁得到一致结论

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 向自查与扩展stub注入相同脚本未就绪 | 自查与硬门禁同一结果 / 门禁与脚本同源判定 | 单次--delivery调用、同样阻断 | smoke | scripts/harness/archive-readiness.smoke.sh；.pi/extensions/tests/spec-gate.smoke.cjs |
| 2 | tasks关键词齐备，结果仍红 | 扩展内旧字样检查退役 | 无平行放行 | smoke | .pi/extensions/tests/spec-gate.smoke.cjs |
| 3 | 明确exit1与缺脚本/spawn异常分别触发 | 环境异常与明确未就绪分开 | 前者ui-verification-missing阻断，后者原fail-open留痕 | smoke | 同上 |
| 4 | 试点接入v1但问题未修 | 自查与硬门禁同一结果 | 真实负路径红，不修改root历史洗绿 | 人工/命令 | 人工：只读delivery与archive-readiness，不执行archive |

## 故事 S6：设计与review不再把接线当产品定位

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 核对模板和规范的动作/宿主/视图语义要求 | 全局动作与重复入口有职责 / 内联组件复用到新工作面 / 视图替换保留信息语义 | 既有八节有承接，无平行文档体系 | 人工 | 人工：模板/reference一致性review |
| 2 | 核对现有编译/截图与v1结果的分流说明 | major UI 双层验收通过 / 只有单元测试不得完成 major UI / 字样齐备但无实物不得通过 / 有截图但故事失败仍不得完成 | 不把默认trace或截图存在当交付合格 | 人工/smoke | 人工：规范与试点review；scripts/harness/ui-verification-trace.smoke.sh |

## 继承与调整

| 旧 Scenario | 处置 | 旧测试文件 | 动作 |
| --- | --- | --- | --- |
| major 档实物齐全通过 / 零痕迹 / 空壳 / 点名截图缺失 | 继承 | scripts/harness/ui-verification-trace.smoke.sh | 默认模式回归；delivery也先走证据层 |
| minor 指名映射通过 / minor 裸关键词不再放行 / none 档一致性 / 旧schema不硬阻断 | 继承 | 同上；.pi/extensions/tests/spec-gate.smoke.cjs | 不套major v1规则 |
| report标准/词表外/格式对齐 | 改语义扩展 | scripts/harness/ui-verification-trace.smoke.sh | 统一A-ID，两形态历史/最新；严格模式非法不静默 |
| 门禁与脚本同源 / 旧字样检查退役 | 改语义 | .pi/extensions/tests/spec-gate.smoke.cjs；scripts/harness/archive-readiness.smoke.sh | 改消费单次--delivery，环境分支不弱化 |

## 白盒附加：分支与边界矩阵

| 编号 | 条件/边界 | 期望 |
| --- | --- | --- |
| D01 | 默认合法fail+有原因未覆盖+图片在盘 | 证据exit0，不声称结果绿 |
| D02 | v1全pass、必验齐、逐项视口齐 | delivery exit0 |
| D03 | 最新fail在必验/非必验；真实转挂目录存在 | 都exit1 |
| D04 | 必验最新未覆盖/没有记录 | exit1 |
| D05 | 非必验有原因未覆盖，其他均绿 | exit0+编号原因提醒 |
| D06 | 旧fail→复验pass新图，标题/表格交叉 | exit0，保留历史计数 |
| D07 | 旧未覆盖→pass无复验标识 | exit1 |
| D08 | 复验标识但仅旧失败图/无新图 | exit1 |
| D09 | 早前pass后条目语法锚点内非法状态、空依据或畸形编号 | v1 exit1，不能回退旧pass |
| D09b | blockquote/列表/说明散文提及A43或A55，不是条目 | 不计数、不阻断，保留叙述史 |
| D09c | 围栏代码块含pass示例/畸形A行、普通表头/分隔行 | 不算真实条目；不能用代码样例满足必验场景 |
| D10 | 表缺失/空表/重复节、A1与A-1重复 | exit1 |
| D11 | ID为0/负数/空/非法字符；空故事 | exit1 |
| D12 | 视口为0、负、缺高、多x、文本尾巴、空列表 | exit1 |
| D13 | v1 marker重复/未知版本/实际marker位于正文或blockquote之后 | 新/老major均exit1，明确位置非法，不降级为未迁移 |
| D13b | 开头连续单行注释可穿空行；正文围栏/行内代码只展示marker示例 | 仅开头实际声明生效；代码示例不算声明；完全缺真实marker按发布日期规则处理 |
| D14 | 无marker且日期2026-10-26 / 10-27 / 10-28 | 老日期明确降级；边界当日及之后红 |
| D15 | 老日期显式v1且有fail | exit1 |
| D16 | major无marker缺创建日/坏日期 | 缺前置红；旧schema仍兼容 |
| D17 | none/minor/旧spec-driven | 原证据语义，零v1结果检查 |
| D18 | 必验A55两视口都在自己最新依据 | 真实尺寸均命中才绿 |
| D19 | 宽屏图只在别的A/旧记录/全局清单 | 不计入A55，红 |
| D20 | 文件名1440但真实1280x633 | 不能覆盖1440x900 |
| D21 | 缺文件/非PNG/坏签名/短IHDR/非正宽高 | 不满足视口，明确红 |
| D22 | 无空格裸相对路径、Markdown链接、角括号目标内含空格/中文/子目录 | 解析到明确目录内真实文件，不猜同名；均有正反fixture |
| D23 | 绝对/../越界/逃逸symlink/不存在 | 红，零证据目录外兜底 |
| D24 | 场景同号多次复验、最后又fail | 以最后fail红 |
| D25 | 自查/扩展stub观察命令参数 | 单次--delivery，无第二parser或关键词放行 |
| D26 | script正常exit1 vs spawn缺失/异常 | 结果红与原环境fail-open分开 |
| D27 | 试点当前A43 fail、A55/56未覆盖、其他必验缺失 | 明确负路径，旧证据通过不抵消 |
| D28 | 试点实际修复后逐故事新截图与复验齐全 | 真实正路径才可收口；原型或fixture阳性不能代替 |

## 固定变体走查

- 输入：空白/空字符串、A-ID重复及大小写非法、超长依据与中文/特殊路径；字段非法要明错，不静默忽略。
- 前置：无文件/空表/单场景/多场景、局部截图缺失、旧schema；不读取业务DB。
- 时间：发布边界前一天/当天/后一天；不取运行时当前日期改变同一change的结论。
- 幂等：同fixture连续执行两次结果一致；脚本无文件写入，部分证据补齐再执行可解释地解除；不宣称并发写安全（只读，不适用写竞态）。
- 可用性：中文错误指出A-ID/视口/路径与修复方法；输出区分存在性/结果/存量降级，不用“有图即绿”的措辞。
- 不适用：HTTP/SQL/真库迁移/AI效果/业务生命周期和全量应用测试，本change均不触发。

## 效果核对

- 基线：当前试点默认模式exit0而真实A43失败；root账本53 pass/1 fail/2未覆盖。
- 方法：先fixture确定各分支，再只读试点负路径；产品获批修复后实际复验正路径。
- 期望：不能再以默认证据成功放行试点；每个必验故事自己的视口齐备且无未处置最新失败才可收口。不把门禁失败率下降当产品质量提升。
