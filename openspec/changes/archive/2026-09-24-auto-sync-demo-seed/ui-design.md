<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## N/A Reason

本 change 是纯部署/运维脚本（定时同步 wrapper + systemd timer + 安全断言），无任何用户可见界面变化，前端零改动。proposal 已声明 `ui-impact: none`，依据：改动面仅 `scripts/deploy/` 与 `~/.config/systemd/user/`，不触及 `front/` 任何路径。
