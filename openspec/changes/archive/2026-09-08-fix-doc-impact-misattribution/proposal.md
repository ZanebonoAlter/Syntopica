<!-- complexity: simple -->
<!-- ui-impact: none -->

# fix-doc-impact-misattribution

## Why

归档门禁的 doc-impact 对账存在四条误归因通道，把其他 change 的改动/工具自管文件算到当前 change 头上：① edit.map 归属集合被污染（openspec archive 移动的文件、共享文档都记入会话绑定 change）；② 归属轨无记录时回退**全树** git diff，树上其他 change 的未提交脏文件全部误入；③ configuration 域正则 `config.*\.ya?ml` 误匹配 `openspec/changes/configure-dsh-energy-research/.openspec.yaml`；④ check-standards E 段溯源检查遍历全部 archive change，§12「归档后补溯源」时间窗内的债会 block 下一个 change 的归档。后果是豁免通胀：2026-08-26 以来 12 个归档中 10 个带 `doc-impact-excuse`、4 个带「无 flow 影响」，且 coordinate-concurrent-changes task 7.5 明确要求「doc-impact-excuse 停止新增」但 2026-09-07 仍在新增——误报不堵死，豁免就永远是归档的最快出路（排查证据：docs/research/doc-impact-attribution/explore-findings.md）。

## What Changes

- **归属输入黑名单（双侧）**：`doc-impact.sh verify` 的启发式输入（归属轨与全树回退轨）与 `lib/edit-map.ts` 的 edit.map 采集侧统一剔除：`openspec/changes/**/.openspec.yaml`（openspec 工具自管元数据）、`openspec/changes/archive/**`（归档移动产物）、以及 AGENTS.md 等仓库级共享文档（共享文档不触发"疑似遗漏"，但 checkbox 显式声明的对账不受影响）。
- **configuration 域正则收紧**：`config.*\.ya?ml` 改为不误匹配 openspec change 目录（排除 `openspec/` 前缀路径），保留对真实配置文件（docker-compose*.yml、config*.yaml 等）的命中。
- **回退全树降级为警告**：归属轨无记录（冷启动/工具类 change）时，verify 不再因回退全树的启发式命中判 FAIL，改为输出「无法归属，全树回退仅提示」的 warn 语义（退出码不因此非零）；「声明了未更新」「路径不存在」等基于 git 事实的对账规则保持 FAIL 不变。
- **E 段溯源宽限期**：check-standards.sh E 段对**归档时间在 N 天内**（默认 3 天）的 archive change 免检溯源链接，溯源债仍由 §12 归档后流程催收，不再 block 时间窗内其他 change 的归档。
- **配套 smoke 断言**：既有 `doc-impact.smoke.sh` / `check-standards.smoke.sh` 增补误归因回归用例（他人归档移动文件不触发疑似遗漏、configure-*.openspec.yaml 不命中 configuration、无归属记录回退仅警告、宽限期内未溯源不 FAIL）。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `doc-impact-gate`: 「归档前对账（verify）」requirement 变更——启发式输入增加黑名单过滤（双侧轨道）、configuration 域正则收紧、归属轨无记录时回退全树从 FAIL 降级为警告；check-standards E 段溯源检查增加归档宽限期。
- `concurrent-change-coordination`: 「文件归属地图落库」requirement 变更——edit.map 采集侧剔除黑名单路径（openspec 元数据/归档移动产物/仓库级共享文档），归属集合不再累积工具自管文件。

## Impact

- `scripts/doc-impact.sh`（verify 启发式输入与正则）、`scripts/check-standards.sh`（E 段宽限期）、`scripts/doc-impact.smoke.sh`、`scripts/check-standards.smoke.sh`（回归用例）。
- `.pi/extensions/lib/edit-map.ts`（采集侧黑名单；gitignored，入库代码快照同步 `docs/research/`）。
- `openspec/specs/doc-impact-gate/spec.md`、`openspec/specs/concurrent-change-coordination/spec.md`（后者当前为 coordinate-concurrent-changes 的 delta capability，本 change 归档需晚于它，见 design）。
- 存量 `doc-impact-excuse` 注释保持兼容不判 FAIL（既有语义），但新通道堵死后预期不再新增。
- 不影响产品 API、业务数据、数据库结构；`concurrency-status.sh` 的归属对照读同一 edit.map 数据，黑名单路径自然不再出现在归属地图。
