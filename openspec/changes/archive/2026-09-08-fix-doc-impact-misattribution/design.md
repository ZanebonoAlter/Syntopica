# Design — fix-doc-impact-misattribution

## Context

四条误归因通道的现场与代码定位见 proposal 与 `docs/research/doc-impact-attribution/explore-findings.md`。涉及三个实现点：`scripts/doc-impact.sh` verify（启发式输入与正则）、`scripts/check-standards.sh` E 段（溯源宽限期）、`.pi/extensions/lib/edit-map.ts`（采集侧剔除）。关键约束：

- **本 change 归档必须晚于 `coordinate-concurrent-changes`**（未归档）——本 change 的 delta spec 叠在其 delta 之上（它首次定义了归属轨与 `concurrent-change-coordination` capability），先合入它的版本再合入本修改。
- 存量事实库中的污染条目（如 split-board-upgrade-directions 归属集合里的 archive/.openspec.yaml）**不做回溯清洗**——消费侧过滤兜底（verify 读集合时先过黑名单，库里存了什么都不影响判定），edit.map 是 append-only 账本，改历史违背事实库设计。

## Goals / Non-Goals

**Goals:**

- 四条误归因通道全部堵死，"其他 change 的改动/工具文件误入当前 change 归档"归零
- 豁免通道（doc-impact-excuse）停止通胀：新误报源消失后无需新增豁免
- 双侧黑名单行为一致：采集侧与消费侧对同一 fixture 路径判定相同（工具自管类）

**Non-Goals:**

- 不清洗/改写历史 edit.map 事件（append-only）
- 不改 `concurrency-status.sh`（读同一 edit.map，共享文档保留在归属地图中，并发冲突感知不降级）
- 不重构既有 8 域启发式正则全集（只动 configuration 域 + 黑名单层）
- 不改 spec-gate 归档五检查结构（E 段宽限期是 check-standards 内部行为）

## Decisions

### D1. 黑名单双层分工：采集侧窄、消费侧宽

- **采集侧**（`edit-map.ts`，quality-gate turn_end 调用）：仅剔除**工具自管文件**——`openspec/changes/archive/**` 与 `openspec/changes/*/.openspec.yaml`。这些是 openspec CLI 操作产物，任何 change 的真实工作范围都不含它们。
- **消费侧**（`doc-impact.sh verify` 启发式输入）：剔除工具自管文件 **+ 仓库级共享文档**（`AGENTS.md`、`front/AGENTS.md`、`backend-go/AGENTS.md`、`docs/reference/constraints-index.md`）。

**为什么共享文档不进采集侧剔除**：AGENTS.md 被两个 change 同时编辑是真实的并发冲突，concurrency-status 的冲突标记依赖它在归属集合中；而 doc-impact 的"疑似遗漏"语义不该被它触发——两层语义不同，各取所需。**备选**（共享文档也剔除出 edit.map）被否：削弱并发感知。

### D2. 黑名单实现：两处镜像 + smoke 同 fixture 防漂移

TS（edit-map.ts）与 bash（doc-impact.sh）无法共享常量，模式列表在两侧镜像维护（前缀 `openspec/changes/archive/`、前缀+后缀 `openspec/changes/<name>/.openspec.yaml`、精确集合 for 共享文档）。`doc-impact.smoke.sh` 增补同一组 fixture 路径断言：对工具自管路径，"采集侧不落集合"（经单测/白盒或模拟 turn 断言）与"消费侧不触发域命中"行为一致，漂移即红。**备选**（JSON 配置文件单一事实源）被否：多一个运行时依赖且 bash 解析成本大于收益，黑名单本身极稳定。

### D3. configuration 正则收紧：前缀排除而非重写

保持 `config.*\.ya?ml` 主体不动，前置条件"行不以 `openspec/` 开头"。**为什么**：该正则的真实目标（`config.yaml`、`*.config.yaml`）语义没问题，唯一实证误报源是 openspec change 目录路径（`configure-dsh-energy-research/.openspec.yaml`）；重写正则反而可能改变既有命中面（deployment 域管 docker-compose、configuration 域不管它，维持现状边界）。黑名单（D1/D2）已剔除 `openspec/changes/**`，正则前缀排除是第二道防线——防御未来黑名单外的 openspec 顶层 yaml 误命中。

### D4. 回退降级：heuristic_track 三态

`heuristic_track` 从两态（ownership/fallback）扩为语义三态：`ownership`（归属轨命中→FAIL 不变）、`fallback`（回退轨命中→输出 `[提示] 无法归属: <domain>（全树回退仅提示，归属轨无记录）` 到 stderr，不计 fail、不改退出码）、规则 2/5（声明了未更新/路径不存在）保持全树 FAIL。**防逃逸论证**：edit.map 是 harness 自动落的（apply 档会话绑定即记录），agent 无法谎称无归属；只有 edit.map 上线前的存量 change 与全程无档会话 legitimately 落入 fallback，这正是"证据弱不该硬 block"的人群。

### D5. E 段宽限期：字典序日期比较，常量 3 天

沿用既有 CUTOFF 同款手法：`GRACE_DAYS=3` 顶部常量，`date -d "-${GRACE_DAYS} days" +%F` 生成阈值，与 archive 目录名前 10 字符字典序比较（GNU date，WSL bash 环境）。**为什么 3 天**：§12 补溯源通常在归档当天到次日完成，3 天覆盖节假日/周末场景且不至于让债积压失控；常量可调，实战再收敛。

## Risks / Trade-offs

- [共享文档剔除后，真的以改 AGENTS.md 为主体的 change（agent-guide 类）漏报"疑似遗漏"] → 缓解：文档影响声明义务不变，此类 change 应在 tasks.md 文档节 checkbox 显式列出（规则 2/5 全树对账仍 FAIL 兜底）；smoke 用例覆盖"显式声明 AGENTS.md 仍对账"
- [回 fallback 轨降级开逃逸口，存量 change 借机不声明] → 缓解：降级仅作用于启发式（规则 3/4），声明缺失（规则 1）与声明文件对账（规则 2/5）仍 FAIL；edit.map 上线后新 change 全程有归属记录
- [黑名单两侧镜像漂移] → 缓解：D2 smoke 同 fixture 断言，任何一侧改动跑 smoke 即红
- [3 天宽限期判断与 §12 催收节奏不匹配] → 缓解：常量可调；E 段 FAIL 语义不变（超期仍催），仅时间窗后移
- [本 change 自身被 spec-gate 拦截（改门禁的人被门禁拦）] → 缓解：本 change 的 doc-impact 声明如实包含 standard/flow 域（改 开发执行规范.md 与 spec 文档），归档时走正常检查

## Migration Plan

1. 实现 + smoke 全绿后，主体 commit（脚本 + extension 快照同步 `docs/research/`）。
2. 部署即生效：脚本无状态；extension 随 pi 会话重载加载新 edit-map 采集逻辑。
3. **存量污染集合不清洗**：verify 消费侧过滤兜底（D1）；下一轮归档 split-board-upgrade-directions 时其"疑似遗漏 standard"（layout.md 归属轨命中）按新规则判定——若 layout.md 确为该 change 会话编辑则仍 FAIL 属正确归属，需其 tasks.md 补声明 `standard` 或完成 layout.md 的文档对账。
4. 回滚：revert 单 commit 即可，无数据迁移。

## Open Questions

（无）
