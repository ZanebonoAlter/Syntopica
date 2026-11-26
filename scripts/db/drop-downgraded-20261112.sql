-- retire-downgraded-mount（2026-11-12）
-- 退役 topic_tag_board_labels.downgraded 列：挂载行为不变（min(2,N) 放宽准入保留），
-- 仅移除标记本身。不删除任何挂载行。
-- 幂等：可重复执行。执行时机：先发新版代码并重启（停写停读该列）→ 再跑本脚本。

ALTER TABLE topic_tag_board_labels DROP COLUMN IF EXISTS downgraded;
