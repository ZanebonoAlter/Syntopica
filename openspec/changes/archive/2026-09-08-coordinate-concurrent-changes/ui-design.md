<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->
<!-- ui-front-path-excuse: tasks.md/specs 中出现的前端路径（front/app/a.vue、front/app/api/dailyReports.ts）为归属地图/历史豁免声明的他人 change 脏文件示例，非本 change 前端改动 -->

## N/A Reason

本 change 为纯工具链/流程改动（extension 落库、bash 脚本、spec-gate 检查、开发执行规范修订），不触及任何前端路径、页面结构与交互模式，与 proposal 头 `ui-impact: none` 声明一致。concurrency-status.sh 的输出是面向 Agent 的终端文本，不构成用户界面。tasks.md/specs 中出现的前端路径（如 front/app/a.vue、front/app/api/dailyReports.ts）均为归属地图/豁免声明中的**其他 change 脏文件示例或实锚**，非本 change 的前端改动，与本 change 无关。
