# 前端双主题系统（Theming）

> **权威源**：本文件是双主题/设计系统的唯一权威。`front/AGENTS.md` 顶部「Frontend-Specific Conventions」的红线要点深链指向本文件。
> 互补：`architecture/frontend.md` §设计系统 描述设计系统的架构定位。

## 视觉基调

editorial / magazine（杂志感），**禁止回退到蓝紫色 SaaS 视觉**。

## 三层 Token 架构

| 层 | 命名前缀 | 职责 |
|----|---------|------|
| Primitive（Layer 1） | `--raw-*` | 原始色值调色板 |
| Semantic（Layer 2） | `--color-*` | 语义化颜色，组件**只能引用这一层** |
| Component（Layer 3） | `--dialog-*` 等 | 组件级专用变量 |

**硬约束：组件只引用 Layer 2 语义 token，不直接使用原始色值（`--raw-*`）。**

## 主题切换

- 通过根元素 `data-theme="editorial|dark"` 切换
- 使用 `useTheme()` composable 管理主题状态
- 主题变量集中复用 `app/assets/css/main.css`

## 统一组件（必须复用，禁止各写一套）

| 场景 | 组件 |
|------|------|
| 页面骨架（reader/contained/workspace/split） | `AppPageShell` |
| 对话框（尺寸档 sm/md/lg/xl，92vw 上限） | `AppDialog`（新代码用 `size`，旧 `width` 勿用） |
| 按钮 | `AppButton` |
| 开关 | `AppToggle` |
| 输入框 | `AppInput` |
| 区块标题 | `AppSectionHeader` |

页面布局模式与弹窗尺寸档的完整契约（批准基线、双视口验收、例外登记）→ [layout.md](layout.md)。

## 图标

使用 Iconify，不引入新的图标库。

## Observability 数据展示分层

可观测信号（质量 / 匹配度 / 贴合度等）的前端展示遵循**分层哲学**，避免数字污染沉浸阅读：

- **正文极轻**：用形态 / 色彩 / 降级表达状态，**不直接渲染数字**。
- **分数进探究区**：具体数值与明细只在 hover 或展开的探究区呈现。
- **软降级保信息**：离群 / 异常项做视觉降级（灰 token + 折叠 + 标记），**不删除**，由用户甄别。
- **token 派生**：降级 / 分档样式从 Layer 2 语义 token 派生，不在 utils 里硬编码颜色（纯函数只产布尔判定 / 中文标签，如 `topicAnchor.ts` / `matchQuality.ts` / `threadFit.ts`）。

具体三系列实例（日报 section 头部徽章 + thread 行降级）见 [daily-report.md](../../flow/daily-report.md) §4；本规范仅收敛为跨 feature 可复用的展示约定。

## 资料来源

本规范源自 v1.3.3 架构深化，内容收敛自原 `front/AGENTS.md` §主题系统 与 `development.md` §前端样式约定。
