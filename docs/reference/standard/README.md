# 代码规范（Standard）

> **权威层**：本目录是前后端**代码规范、项目约束、Lint/测试配置的唯一权威源**。
> `front/AGENTS.md` 与 `backend-go/AGENTS.md` 只保留红线速查要点 + 深链指向本目录（方案 B，防孤立）。

## 这层装什么

| 写的内容 | 放哪 |
| --------- | ------ |
| 代码怎么写（命名、import、错误处理、目录归属） | `frontend/code-style.md` / `backend/code-style.md` |
| 双主题 / 设计系统 / Token 架构 | `frontend/theming.md` |
| 页面布局模式 / 弹窗尺寸档 / 双视口验收 | `frontend/layout.md` |
| 交互友好性 / 可观测性展示分层（状态标记左对齐、状态说明不伪装动作） | `frontend/interaction-conventions.md` |
| Lint 工具配置（ESLint / golangci-lint） | `frontend/lint.md` / `backend/lint.md` |
| 测试框架、运行方式、编写约定 | `frontend/testing.md` / `backend/testing.md` |
| 后端包结构 / domain 白名单 | `backend/package-layout.md` |
| AI 调用记录（LLM 调用必须记什么、编排 session 串联） | `backend/ai-logging.md` |
| 提交前检查、Branch/PR | `shared/commit-pr.md` |
| reference 文档新增/修订标准（目录职责、头部注释语法、注册点 checklist） | `shared/doc-authoring.md` |

**不装什么**：业务流程链路 → 去 `flow/`；架构骨架 → 去 `architecture/`；执行流程/门禁/归档 → 去 `开发执行规范.md`。

## 目录结构

```
standard/
├── README.md                  # 本文件（索引）
├── frontend/
│   ├── theming.md             # 双主题三层 Token
│   ├── layout.md              # 页面 shell 四模式 + dialog 尺寸档 + 双视口验收
│   ├── interaction-conventions.md # 交互友好性/可观测性展示分层
│   ├── code-style.md          # 目录归属/API归一化/Store/事件流/拆分
│   ├── lint.md                # ESLint 配置
│   └── testing.md             # Vitest + Playwright
├── backend/
│   ├── package-layout.md      # 三层包结构 + domain 白名单
│   ├── code-style.md          # 命名/错误处理/分层
│   ├── ai-logging.md          # AI 调用记录规范（prompt/session/token 必记）
│   ├── lint.md                # golangci-lint 配置
│   └── testing.md             # testing + testcontainer + DSN 安全红线
└── shared/
    └── commit-pr.md           # 提交前检查 + Branch/PR
```

## 验收门禁

本目录由 `scripts/check-standards.sh`（L1）静态校验，归档前随《开发执行规范》§11.4 归档门禁一起跑。脚本现含 A-G 段：

- **A** 文档完整性（standard/flow 关键文件存在 + flow 五段式结构校验）
- **B/C** 后端 domain 白名单 + 三层包结构 / 前端 ESLint + Token 三层 + 双主题
- **D** 防孤立引用（每个 `standard/**/*.md` 至少被一处 AGENTS.md 或本 README 引用）
- **E** flow 变更溯源链接（archive 后校验）
- **F** doc-impact 声明对账（`doc-impact.sh verify`，见《开发执行规范》§11.4）
- **G** 导航层文档死链检查（docs/README + reference 一级 + flow/architecture 导航文档）

## 与《开发执行规范》的边界

《开发执行规范》回答 **"什么时候做什么、门禁跑什么、怎么归档"**（流程）；
本目录回答 **"代码长什么样、测试怎么配、包怎么分层"**（规范）。
开发执行规范的 §4.2/4.3/4.4/5.2/5.3/5.4 只保留一行深链指向本目录，不再重复内容。
