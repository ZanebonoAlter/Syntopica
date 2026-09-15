<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->
<!-- ui-front-path-excuse: tasks 中的 front/ 路径（nuxt.config.ts、app/utils/api.ts 及其单测）是 API 取址与 dev 代理的网络管线改动，不触及任何组件/样式/交互，无用户可见界面变化，故 ui-impact 仍为 none -->

## N/A Reason

本 change 只动开发期网络拓扑与配置默认值（后端端口、dev 代理、apiBase 解析、文档），不触及任何界面结构、布局、样式或交互；proposal 声明 `ui-impact: none`，无数码界面需要设计。
