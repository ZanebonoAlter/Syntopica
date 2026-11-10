## Context

试点现有默认 trace 已实测返回 0：53 pass / 1 fail / 2 未覆盖、55 张点名截图。脚本目前职责是证据存在性，spec-gate 与 archive-readiness 已消费其退出码；重复发明结果 parser 会制造漂移。只读复核两轮的落点为现有脚本加交付模式，父线程否决“转挂存在的 change 即放行”、全局任意一张宽屏图替所有故事，以及把必验故事清单留给未来变更的建议。

本次是流程 change（UI none）。产品的布局、导航、正文承载和关系浏览另在原工坊 major change；其审批仍 pending，不因本 change 进入实现而被放行。

## Goals / Non-Goals

**Goals**
- 证据与结果职责分开，但共享脚本/parser。
- 阻断已知最新失败、核心未覆盖、必验缺失与非法条目；有效复验保留历史后可解除。
- 必验故事用自己的最新截图证明各自承诺视口，不用文件名或全局图片清单代替。
- 在现有八节规划内补产品职责和组件宿主；明确新旧合同兼容边界。

**Non-Goals**
- 不建验收平台、事件状态库、截图视觉识别器或反篡改系统。
- 不改业务数据库、API、匹配/关系算法或候选生命周期。
- 不在此 change 代替工坊原型审批，不运行全量应用测试/构建，不把虚构原型样例当生产验收。

## Decisions

### D1 现有脚本增加 --delivery，共用一次解析
- **落点**：`scripts/harness/ui-verification-trace.sh`；`scripts/harness/ui-verification-trace.smoke.sh`。
- **取舍**：不新增独立结果脚本，也不在扩展里二次解析；模式名称分清职责比复制 parser 更小、更可维护。

默认调用仍只判证据齐备；`<dir> --delivery` 在同次运行中先证据、再适用的结果规则。扩展和自查各改为单次 delivery 调用，不先后 spawn 两遍。现有表格/标题解析在同一处记录规范化 A-ID、判定、依据、行序、截图引用及非法形似记录；存在性计数保留所有合法条目，结果视图按编号取最后记录。A1/A-1 标题与表格同样规范化，属于兼容扩展而不是另造格式。

### D2 一个版本 marker + 一张必验表，不设第二套视口事实源
- **落点**：`openspec/schemas/syntopica-ui/templates/ui-design.md`；`openspec/schemas/syntopica-ui/schema.yaml`；本 change 的 `specs/ui-delivery-acceptance/spec.md`。
- **取舍**：拒绝独立验收 JSON/YAML 文档，也不再增加全局 ui-viewports marker，避免规划表、全局视口和实测记录三套来源。

major v1 仅在 ui-design 文件开头连续单行HTML注释区写唯一 `<!-- ui-acceptance-contract: v1 -->`：可穿插空行，遇首个非注释/非空白内容即结束，与 `head_markers()` 完全同域。正文、blockquote之后等位置的实际合同注释属于位置错误，存量major也必须报错，不能当作未迁移放行；围栏/行内代码里的说明示例不是声明。Acceptance 内写：

```markdown
### Delivery Contract
| A-ID | 必验故事与打开状态 | 必验视口 |
| --- | --- | --- |
| A-43 | 文章内打开计算、展开输入、返回原文章 | 1440x900,1920x1080 |
| A-55 | 阅读中展开板块/日期选择、切换后仍知上下文 | 1440x900,1920x1080,2560x1440 |
```

表必须唯一/非空；编号去连字符后统一比较，数值须为正整数，重复、空故事、畸形视口和非法行均报具体位置。表中第三列仅机器视口列表，窄屏“验提示”等解释放故事列，不混在数值里。无效 v1 不回退为存量。

固定发布边界为 `created >= 2026-10-27` 的新 syntopica-ui major 必须 v1；更早的未迁移 major 保留证据兼容但明确声明没有结果判定。显式 v1 不受日期豁免。缺创建日期不能猜成旧，需补元数据或合同。none/minor/旧 spec-driven 不追加 major 规则。不新增可覆盖发布日期/跳过交付的环境变量，保留既有显式绕过留痕机制。

### D3 场景编号稳定，复验追加；转挂不构成放行
- **落点**：`scripts/harness/ui-verification-trace.sh`；`specs/ui-verification-trace-gate/spec.md`；`test-cases.md` D01–D17。
- **取舍**：不把每次尝试再造一套全局 attempt ID/时间状态机；但也不接受“最后随手写 pass”或“转挂空 change”即成功。

任何最新 fail 都阻断，包括非必验场景；必验没有合法记录/最新非 pass 同样阻断。非必验未覆盖须有原因并逐条告知。v1 只把根级 `### A…` 场景标题和首列处于A编号位置的表格行视作条目候选；畸形编号/判定/缺依据均阻断，不能被 parser 丢弃后洗回旧 pass。表头/分隔行、blockquote、列表、散文、围栏代码中的A-ID提及不计条目、不阻断；保留试点说明文字与叙述史，D09b/D09c分别钉住散文与代码样例边界。

同编号出现先前非 pass 后，要以新 pass 解除，依据须以“复验”开头、说明实际检查结果，并点名至少一张不在该场景最近非 pass 依据中的新截图；保留旧记录，追加在文件末尾。若是必验场景，其最新依据还须覆盖全部本行视口。

**诚实边界**：脚本能核对当前文件中的历史、行序与新引用，不能从一份快照证明旧 fail 从未被删除、复验确实运行、或新文件不是旧图的副本；这些由 review 对照 diff 和实际浏览器证据核实，不新增状态库掩饰这一边界。

### D4 真实 PNG 尺寸仅在该场景最新依据内核对
- **落点**：`scripts/harness/ui-verification-trace.sh` 的截图提取/解析辅助；`scripts/harness/ui-verification-trace.smoke.sh`。
- **取舍**：不用文件名、手填尺寸 JSON 或图片识别服务；使用已有宿主标准命令读取 PNG 头，避免新增依赖。

默认模式的历史速记/存在性兼容保持；v1 支持无空格裸相对路径与Markdown链接，空格路径用 `[说明](<子目录/有 空格.png>)`，中文路径/子目录有效。只用明确引用解析目标，不猜同名图片。引用真实解析后须留在 `ui-verification/` 内，绝对路径、越界/逃逸链接不参与；不去其他原型目录找同名图。先校验 PNG 签名、可读完整 IHDR 和正宽高，再匹配合同值；非 PNG 可作附加证据但不满足视口。全页长截图/裁剪图不等于承诺的视口截图，不能因文件名带1440就通过。

每个必验 A-ID 仅用其最新 pass 依据的引用匹配自己的全部视口；不能借其他条目、全局清单或旧失败记录。尺寸通过仍需人看“图里是不是宣称的打开态”，实际遮挡/点击命中等机械断言仍由浏览器执行。

### D5 自查和硬门禁调用同一结果入口
- **落点**：`.pi/extensions/spec-gate.ts:checkUiEvidence`；`scripts/harness/archive-readiness.sh`；各自定向 smoke。
- **取舍**：优先改现有 UI 项为证据+交付两层的单次调用，不增加一套第八检查或两次重复 spawn；扩展只消费结果。

script 明确返回 1 视为 UI 未就绪，沿用 `ui-verification-missing`；不得误标环境 `ui-gate-check-failed`。缺脚本/spawn 环境故障保持现有 fail-open 显著告知，已有显式 bypass 纪律保持。存量降级只提示事实，不写成功 policy 噪声。对账文档与输出要区分痕迹成功和结果成功。

### D6 规范收口到已有权威源，试点不清旧账
- **落点**：`docs/reference/开发执行规范.md` §0.6/§5.3/§11；`docs/reference/standard/frontend/layout.md`；`docs/reference/standard/shared/test-design.md`（含 JIT）；`docs/reference/standard/shared/doc-authoring.md`；`docs/reference/harness/pi-extensions.md`。
- **取舍**：不新增平行规范文件或每次事故一个专用禁令；只补清职责、复验和结果判断的边界。

动作作用域/主入口/必要快捷理由放 IA/Interaction，组件宿主与滚动/返回放 Component Reuse，关系等价放旧能力去向复核，打开态与逐场景视口放 Acceptance。原生语义不是禁用对象；主题/密度/焦点/展开态必须适配。试点 UI 合同获批后同步产品制品，R-11…R-16 和 root A-43 fail / A-55、A-56 未覆盖不得在仅修规范时改成已解决。

## Risks / Trade-offs

- 合同越写越大 → 以真实任务切片，不给每个控件造故事；每行范围和视口须可解释。
- 仅元数据全绿仍可造假 → 脚本不承担审美、截图内容、真实运行的裁判；独立 review 保留。
- 新旧格式混排 → parser 共用、默认模式定向回归；strict 只在 v1/新 major 生效。
- 日期边界手写可被改动 → 明确其为迁移规则，不宣称反篡改；非法/未知不猜成旧。
- 引入迭代时误卡其他 active change → 不扫全树旧变更，验证按显式 change 参数；试点显式接入，不硬追拦全部存量。

## Migration Plan

1. 本流程 change 完成规范/模板、脚本和集成 smoke；默认模式回归通过。
2. 为工坊 ui-design 加入 v1 和固定必验表（不得标批准）；确认 delivery 清楚列出当前 fail、核心未覆盖和缺失场景，自查同源。保留失败证据。
3. 用户批准工坊修订原型后才实施产品修复，在隔离数据验证写故事；真实复验追加 root 账本并保留旧失败。
4. 试点正路径实际通过后完成本流程 change 的最终验证；不在只有 fixture 阳性时声称试点真实成功或提前归档。

## Open Questions

无需再向用户询问技术门禁默认值：以上为已授权范围内的最小设计裁决。产品原型的布局/关系呈现仍需用户明确批准；这是独立阻断，不由技术合同代替。
