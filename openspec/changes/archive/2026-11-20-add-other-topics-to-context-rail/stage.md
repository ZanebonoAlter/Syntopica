<!-- stage: implement -->
<!-- constraint-domains: semantic-board, daily-report -->

# add-other-topics-to-context-rail — 阶段载体

- 规划制品（proposal / ui-design / specs delta / design / tasks）已全部完成，planning complete。
- 当前进入实现期；实现顺序见 `tasks.md` 第 1–2 节（T1–T9），review 见 T10。
- 探索期落点与实测事实见 `explore-findings.md`（实现期自动注入，无需重探）。
- 约束域声明与 `proposal.md` 头部并集一致；实现期若发现新域，在本文件头部补（只增不减）。
- ⚠️ 并发会话注意（已核实并处置）：`openspec/specs/workshop-board-console/spec.md` 的「治理右轨与治理抽屉」现处于**回退后的旧基线**——`console-rails-live-signals` 于 13:29 提交（`5284af47`）同步的五块两层增量，被其后 `f908cc53`（15:03 归档 `simplify-workshop-matrix-copy`，基于旧基线写 MODIFIED）覆盖。本 change 的 delta 采用 **REMOVED + ADDED 整条重写**，ADDED 以「五块 + 当日日报三层 + 其他话题层」的**正确版本**为基底，归档净效果同时抦正该回退（详见 delta 的「基线说明」段）。归档前必读。
