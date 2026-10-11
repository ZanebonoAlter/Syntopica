# Human Feedback — fix-aux-label-idle-tx-cache

> **定位**：记录用户在 change 生命周期内对产物（proposal/design/ui-design/原型/实现/文档）的反馈，作为本 change 修订依据；也是 harness-retro `--lifecycle` 体检卡「人类反馈段」的计量输入。人写给人的修订账本，不进 events.db（文件随 change 归档永久保存）。
> **消费方**：① 本 change 修订（条目 → 对应制品增量）；② harness-retro --lifecycle 计量（条数/归因分布/未处置数）；③ 归档对账（archive-readiness 顺带 feedback.sh verify——未处置条目提醒处置，不许静默丢）。
> **格式**（条目级契约，一行正则全解析）：条目标题行 `### FB-<N> <日期>【<类型>|<归因>】<标题>`；条目含状态行 `- **状态**：<状态>`。词表见 `bash scripts/harness/feedback.sh menu`。

## 反馈清单

### FB-1 2026-10-07【产品|backend】后端两次不可用：本 change 增量维护的三处 map 原位写致 concurrent map fatal

- **状态**：已消化（hotfix 已随 fix-board-match-cross-domain 会话完成并部署验证；design D2 / test-cases / tasks 语义同步已于归档前补齐——详见下方处置记录末尾）
- **原文要点**：用户在 fix-board-match-cross-domain 会话期间两次报告后端不可用（「我后端咋没了」「咋又挂了，是被系统杀了？」），第一次旧 nohup.out 已被覆盖、第二次抓到 fatal 现场。
- **已核实事实**（2026-10-07，nohup.out 崩溃栈 + 代码比对）：
  - 死因 `fatal error: concurrent map read and map write`，崩溃点 `auxlabel.sqlMergeMatcher`（auxiliary_label_service.go:1006，打标队列 RetagArticle→AttachAuxiliaryLabels→planAuxiliaryLabels 链路），非系统 OOM（dmesg/journalctl 无 kill 记录，前端 next-server 全程存活）。
  - 根因是本 change 引入的「缓存失效增量维护」：三处 mergeEmbeddings 原位写/删（applyCreate:908 写、applyDisable:932 删、applyMerge:951 删）虽在 mu 锁内，但读方 sqlMergeMatcher 经 GetOrLoad 拿到 map 引用后无锁遍历读——「锁内写 + 锁外读同一 map」在 Go 是不可恢复的 fatal。design 中「append 超过读者快照长度安全」的论证只对 slice 成立，对 map 不成立（写会动 hmap bucket，读者 hash 查找时崩）。
  - 第一次宕机大概率同因（时间线吻合，日志无直接佐证）。
- **处置记录**（hotfix 已做，auxlabel 包测试全绿、已部署 health 200）：
  - 移除三处 mergeEmbeddings 原位写/删，向量新鲜度回退 10min TTL 全量重拉兜底（正确性无损，代价仅新标签 L2 可见性延迟 ≤TTL）；activeLabels 的 slice 增量操作保留（安全）。
  - 三个增量缓存测试（TestAuxLabelCacheCreate/Disable/MergeAliasIsIncremental）断言同步改为「map 不被原位修改 + activeLabels 增量保留」——原断言在保护 fatal 行为。
- **后续动作**（2026-10-10 归档前已全部完成）：
  - ✅ design.md D2 改写为修订版：仅 activeLabels 半边增量合法（append / fresh-slice 替换两种读者安全形态），mergeEmbeddings 半边不得原位写（无锁读者约束），新向量可见性回退 TTL 全量重拉；替代方案表补写时复制/原位写的否决理由。
  - ✅ test-cases.md 效果核对节按新语义复核（map 大小不变 + activeLabels 线性 +3；「全量拉 N→1」限定为创建触发口径，补 ≤TTL 可见性延迟说明）；tasks 1.1/1.2 描述同步改写（勾选状态维持——增量意图在 slice 半边保留，测试全绿）。
  - 若业务上确需新标签即时进 L2 缓存：正确方向是写时复制（整 map 拷贝替换引用，读者持旧快照安全）或分片锁，需先评估 17.5k 行向量的拷贝成本，不得回到原位写（已写入 design D2 替代方案否决节）。
  - 事故关联记录：fix-board-match-cross-domain/human-feedback.md FB-2 原始账目（处置细节同源）。