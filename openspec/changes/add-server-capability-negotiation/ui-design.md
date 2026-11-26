<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## N/A Reason

本 change 是**纯服务端能力**：

- 新增的 `GET /api/capabilities` 是只读 JSON 端点，无用户可见界面
- 档位过滤发生在进程启动期与调度注册期，不产生任何界面元素
- 前端消费该端点（据其隐藏/降级功能入口）属**另一个 change**，本 change 不触碰 `front/`

`ui-impact: none` 与本 change 的实际改动面一致：产出为后端接口契约与进程行为契约。
