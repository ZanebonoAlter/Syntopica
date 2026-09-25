<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->
<!-- ui-front-path-excuse: tasks.md 的 doc-impact-excuse 行提及 front/app/features 是为描述并发 change（board-signal-reports）混入归属集合的污染文件（误报来源），非本 change 自身改动；本 change 零前端代码 -->

## N/A Reason

本 change 为部署工具链与仓库文件布局调整：移动根目录 compose/Dockerfile/init 脚本、修复引用、新增 `scripts/deploy/deploy-remote.sh` 远程推送脚本。不新增、修改或触碰任何前端页面、组件、交互模式——与 proposal 的 `ui-impact: none` 声明一致。不制作原型。
