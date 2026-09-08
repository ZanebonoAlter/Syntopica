
## flow 约束节体积与 declaration 注入数据

declaration 注入现状与 flow 约束节体积实测（2026-09-01）：
- declaration 注入 382 次 / 2.4MB，均值 6.3KB/次，最大单次 12.3KB；多域 change 单回合堆叠达 12~16KB（实测 08-31 一回合：declaration 12296B + 3609B + jit 3110B + change-file 2342B）。
- 9 个 flow 域「业务约束与不变量」节体积：daily-report 12.9KB、data-enrichment 12.2KB、discovery 7.6KB、reading 5.2KB、ai-summary 4.3KB、scheduler 4.0KB、content-enrichment 3.9KB、semantic-board 3.5KB、topic-graph 3.1KB。两个 12KB 级大块头是多域声明 change 的开销主力。
- spec 锚：openspec/specs/constraint-injection/spec.md「每 turn system prompt 强制注入」requirement 第 87 行段——声明域"每回合从文件重解析，SHALL NOT 进入只增不减粘性集合"（红线层改造保持该语义即可）；jit-path 复用 doc-impact-applies 标签（细节层按需的既有通道）；minSectionBytes 缺省 512（红线层很小，需适配：红线层提取失败回退全节）。
- 改写风险锚：约束节改写是 9 文档批量操作，改写期间归档的业务 change 若同动 flow 约束节会冲突，实施时需挑 flow 文档安静窗口。
- 效果核对方案（上线后）：constraint.inject 事件核对 declaration bytes 下降幅度 + jit-path 命中是否补位细节层。

<!-- pinned 2026-09-01T14:05:02Z -->
