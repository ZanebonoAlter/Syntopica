<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## N/A Reason

本 change 修复测试运行环境三处问题（vitest NODE_ENV 钉死、useState mock registry 用例间清理、巡检后端分片超时上界），改动范围仅 `front/vitest.config.ts`、`front/vitest.setup.ts`、`scripts/harness/test-patrol.sh`，不触及任何用户可见界面，与 proposal 的 `ui-impact: none` 声明一致。
