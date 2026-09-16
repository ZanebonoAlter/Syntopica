<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->
<!-- ui-front-path-excuse: tasks 中的 front/ 路径仅为注释措辞去 WSL 化（nuxt.config.ts 的绑定理由注释、app/utils/api.ts 的「WSL 工具」用词）与 front/AGENTS.md 文档，不触及任何组件、样式、路由或交互，无用户可见界面变化，故 ui-impact 仍为 none -->

## N/A Reason

本 change 只动开发工具链与环境表述：`.pi/extensions/quality-gate.ts` 的平台自适应分流、Docker 镜像可达性配置、以及活文档/spec 的 WSL 措辞去化。不改任何界面结构、布局、样式或交互，前端仅有两处注释措辞与一份 `front/AGENTS.md` 说明性文档受影响，无用户可见行为变化。proposal 声明 `ui-impact: none`，无界面需要设计，故不制作原型。

（依据 `docs/reference/standard/frontend/layout.md` 的分档判据：page shell 四模式、dialog 四档、双视口验收均不适用。）
