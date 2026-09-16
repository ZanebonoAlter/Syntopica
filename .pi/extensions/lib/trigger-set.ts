/**
 * trigger-set — 门禁增量路由纯函数（harness-observability-fixes design D1）
 *
 * 门禁触发集 = 相对上回合快照的新增或内容变化路径。快照 = git 脏文件集
 * （tracked diff + untracked）的 {mtimeMs, size}；会话边界（session_start）以当时
 * git 状态初始化基线——会话前残留改动进基线，不触发门禁（非本会话 agent 所为）。
 *
 * mtime+size 双键而非内容 hash：编辑必然更新两者；hash 需每回合全量读文件，
 * WSL drvfs 上 IO 不可接受。纯函数无 IO，确定性（同输入同输出）。
 */

export interface FileStat {
	mtimeMs: number;
	size: number;
}

/** 触发集：curr 中新增的路径，或 mtime/size 任一变化的路径。纯删除不触发。 */
export function computeTriggerSet(
	prev: Map<string, FileStat>,
	curr: Map<string, FileStat>,
): Set<string> {
	const out = new Set<string>();
	for (const [path, st] of curr) {
		const old = prev.get(path);
		if (!old || old.mtimeMs !== st.mtimeMs || old.size !== st.size) {
			out.add(path);
		}
	}
	return out;
}
