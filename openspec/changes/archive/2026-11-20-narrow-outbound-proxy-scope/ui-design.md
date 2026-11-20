# UI Design: narrow-outbound-proxy-scope

<!-- ui-impact: none -->

本 change 为纯后端出站路由收窄（httpclient transport 选择），不涉及任何界面、交互或视觉变更；设置页代理 section 仅服务端注释变化，前端无改动。

## N/A Reason

无需 UI 原型与视觉验收：全部改动在 backend-go 出站路由层（transport 选择与作用域标记），无新页面/组件/样式/交互；`GET/POST /api/settings/proxy` 请求与响应结构不变（仅服务端注释文案收窄），现有设置页无需调整也无从验收视觉变化。
