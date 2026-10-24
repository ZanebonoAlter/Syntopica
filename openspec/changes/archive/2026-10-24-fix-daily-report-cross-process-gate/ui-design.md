<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## N/A Reason

本 change 为纯后端修复（日报生成域闸升级 PG advisory lock + SaveReport 原子 upsert），依据 proposal 头部 `ui-impact: none` 声明：无任何用户可见界面变化——手动触发 409 文案、调度 skip 行为、通知口径均保持既有契约不变，前端零改动。
