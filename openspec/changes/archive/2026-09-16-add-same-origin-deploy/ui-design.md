<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->
<!-- ui-front-path-excuse: proposal/tasks 中的 front/ 路径全部是「引用既有实现作为依据」与「构建命令/产物路径」，本 change 对 front/ 目录零 diff，不涉及任何用户可见界面变更 -->

# UI Design: add-same-origin-deploy（none）

## 为什么是 none

本 change **不改任何前端代码**：`front/` 目录零 diff。产出是构建参数（`Dockerfile` 一行 ARG）、部署制品（Caddyfile + compose + README）与文档。前端渲染结果、组件、布局模式、弹窗尺寸档、主题、交互状态全部不变。

唯一与前端沾边的是**构建参数**（`NUXT_PUBLIC_API_BASE=/api` 构建期注入，影响产物内的 API 地址）与**访问入口**（`:80` 同源而非 `:3000`）——两者都不改变 UI 契约，只改变请求发往哪个 origin 与页面从哪个端口加载。

## N/A 一致性

- 布局契约（page shell 四模式 / dialog 四档 / 双视口验收）：不适用，无 UI 结构变更
- 状态契约（loading / empty / error）：不适用，无新增或修改的 UI 状态
- 原型：不需要（none 档豁免 `ui-prototype/`）
- 双视口验收证据：不需要（无 UI 变更可验）

前端路径出现在本 change 的 tasks 中仅为**构建命令与产物路径**（`front/.output/public`），非代码改动；若 `ui-design-gate` 的 `ui-impact-mismatch` 启发式因该路径误报，按 `ui-front-path-excuse` 约定豁免。
