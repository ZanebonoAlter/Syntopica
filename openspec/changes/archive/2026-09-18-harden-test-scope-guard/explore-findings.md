
## 评审结论：D1 顺序矛盾与 spec 措辞冲突（实现前必修）

评审结论（2026-09-18，对照 test-scope-guard.ts/policy-decision.smoke.cjs/quality-gate.ts/change-scope.sh 逐项核实）：

【事实核对全对】143 行/:58 只挂 bash/单正则；取证 15/15 误报与 pin:d30cecfe 一致；change-scope.sh:158 建议全量提示语、:120 影响包形态原文一致；quality-gate 只跑 vet/build ./... + 影响包 go test -short（:545-555），pi.exec 边界成立；.gitignore:65=docs/research；仓库根无 go.mod；68 用例计数、8 Scenario 映射、validate 通过、主 spec Requirement 在 :181。

【必须修①】design D1 顺序矛盾：掩蔽在①、-c 递归在②，则 bash -c 'go test ./...' 的 payload 在①就被引号掩成占位符，②抽不出东西，TC-B1-06 必挂。修法：-c payload 抽取基于原始文本、在掩蔽之外完成（引号作定界符），payload 递归后再各自走①-④。

【必须修②】delta spec「命中任一放行语境时零提醒零记账」与「归档语境放行时附登记指引」矛盾（B8-05/B7-03 都要求 info 提醒）；改为「零 warn/block 警示、零 policy.decision（info 指引除外）」，并写明依赖变更放行是否附登记指引（当前代码同分支会附）。

【建议修③】-c 递归需继承 -c 调用点之前解析的 cwd（TC-B1-06 的 cd 在 payload 外），design 未写。

【建议修④】自豁免回路：hard 的 block reason 以 tool error 回到会话条目，reason 含「归档」（现状已如此）、新文案还含「依赖变更|建议全量」→ 首次 block 后同会话同命令全部静默豁免。修法：reason/notice 统一 [test-scope-guard] 前缀（reason 现缺）+ hasArchiveContext 过滤含该前缀的条目；至少记入风险表。

【建议修⑤】放行语境/逃生注释判定必须作用于原始命令文本（掩蔽前）——# archive-gate、# 依赖变更 都在注释里，掩蔽后即被剥掉。

【建议修⑥】test-cases §1 fixture 需 git init（design 风险表有、§1/T1 漏）；实现若用 pi.exec 跑 git，smoke makePi() 需补 exec 桩。

【小修】design 引 quality-gate.ts:188/:242 实为 :197/:271（join 在 :507）；D4「或未提供 language」与 §1 缝表不一致（ctx_execute schema language 必填，统一为非 shell 不解析）；proposal「4 个用例」实为 5 场景 8 断言（期望 warn/block 恰 4 条 + t4b 指引断言必挂）；未闭合 heredoc 保守处理未规定；B1 可补 bash -c 'cd backend-go && go test ./...' 纯函数用例。

<!-- pinned 2026-09-18T12:26:52Z -->
