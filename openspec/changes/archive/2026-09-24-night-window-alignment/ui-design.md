<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## N/A Reason

本 change 为纯后端管线行为调整（firecrawl 调度门控、tag 队列消费顺序与 TTL、日报生成时机），与 proposal 的 `ui-impact: none` 声明一致：无新增接口结构、无交互模式变更。日报晚到属数据时机变化，前端沿用既有未生成空态；调度器状态卡对 firecrawl 运行态的展示为既有能力，不需新组件或布局调整。
