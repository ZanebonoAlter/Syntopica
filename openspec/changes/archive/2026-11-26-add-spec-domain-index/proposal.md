<!-- complexity: simple -->
<!-- ui-impact: none -->

## Why

`openspec/specs/` 有 **177 个 main spec、27203 行**，但**没有任何业务域归属**：按名称前缀分组，**96 个落进 `other`**，连「它属于哪个功能域」都判断不出来。

后果三处（均已实测发生）：

1. **改功能串不起历史**：想知道「这个域以前定过什么契约、有没有设计冲突、哪些能力必须沿用」，只能靠 review 线程手工 `git show HEAD:` 翻旧组件——`docs/reference/开发执行规范.md` §0.6 review 要点 1.5 的「旧能力去向表」就是这个痛点的补丁，而它靠人力。
2. **写 delta 不知道改哪个 spec**：177 个 capability 无索引轴，新 change 的 delta 只能赌，已经出现过「两个 in-progress change 各自 MODIFIED 同一 Requirement 且互不包含」的 High（`2026-11-25-add-key-event-other-lane`）。
3. **文档体量无法按域评估**：`docs/research/process-arch-rework/issues.md` ISS-04 的「规模账单」要求按功能域看文档体量，域归属缺失导致落不了地。

注：main spec 膨胀**不会**导致上下文膨胀——实测 `openspec instructions apply` 的 `contextFiles` 只读 change 内 341 行，main spec 零行。代价不在 token，在「找不到」与「改矛盾」。

## What Changes

- **177 个 `openspec/specs/*/spec.md` 头部加 `<!-- domain: <域名列表> -->`**，取值封闭（10 个业务域 + 5 个工具域），支持多值。归类由模型判定 + 人工复核，草表见 `spec-domains-draft.md`。
- **新增 `scripts/harness/domain-history.sh <域>`**：按「delta capability → main spec domain」两跳推导，输出该域的 main spec 清单、历史 change 时间线、flow 红线、以及「可能冲突 / 必须沿用」提示段。实测两跳覆盖率：526 个历史 delta 中 488 个（93%）能推导。
- **`standard/shared/doc-authoring.md` 登记新头部注释语法**（`<!-- domain: -->`），与既有 `constraint-domains` / `doc-impact-applies` 并列。
- **归档门禁 warn（不 block）**：新 change 的 delta 若引入新 capability，而该 capability 的 main spec 无 domain marker → 提示补标。

**非目标**：不合并 177 个 spec（那是不可逆大手术，本 change 只补索引，随时可回退）；不改任何 spec 的正文内容。

## Capabilities

### New Capabilities

- `spec-domain-index`：main spec 的业务域声明、历史 change 的域归属两跳推导、按域反查历史契约。路径 `openspec/specs/spec-domain-index/spec.md`。

### Modified Capabilities

无。

## Impact

- **新增文件**：`scripts/harness/domain-history.sh`、`scripts/harness/domain-history.smoke.sh`、`openspec/specs/spec-domain-index/spec.md`、`openspec/changes/add-spec-domain-index/spec-domains-draft.md`
- **改动文件**：177 个 `openspec/specs/*/spec.md`（仅头部加一行）、`docs/reference/standard/shared/doc-authoring.md`（注册点）、`docs/reference/harness/pi-extensions.md`（脚本清单）
- **不涉业务域**：纯工具链/文档索引，不改任何业务代码、不改任何 spec 正文
- **回退成本**：删 177 行 marker + 删一个脚本，零副作用
- **并发影响**：4 个活跃 change 不受影响——marker 加在 main spec 头部，与 delta 编辑无交集；但归档时若同时改同一 spec，merge 需注意（仅一行，冲突可忽略）
