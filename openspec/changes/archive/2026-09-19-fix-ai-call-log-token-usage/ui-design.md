<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## N/A Reason

本变更纯后端观测性修复（airouter 解析与写库管道），不新增、不修改任何用户可见界面结构或交互模式。管理端调用日志/会话详情的 token 展示位沿用现有组件与字段（`token_usage`/`summary.total_tokens`），仅数据从全零变为真实值。依据 proposal 头部的 ui-impact: none 声明。
