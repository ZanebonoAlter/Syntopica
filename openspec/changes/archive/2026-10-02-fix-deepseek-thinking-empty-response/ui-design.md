<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## N/A Reason

本 change 为 airouter 平台层修复（模型思考档配置修正 + 失败调用观测记账补强），前端零改动（proposal 已声明 `ui-impact: none`）：不新增页面/组件/交互，不改变任何用户可见的界面结构与状态。`ai_call_logs` 失败行新增落库的 `token_usage` 与 finish_reason 仅影响后端日志数据，现有消费方（session 聚合按成功行读取，形状不变）不受影响。
