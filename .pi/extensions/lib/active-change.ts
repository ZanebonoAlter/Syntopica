/**
 * active-change — openspec change 目录检测（共享模块）
 *
 * 自 constraint-injection.ts 提取（change: harness-facts-tier-a，design D1）：
 * quality-gate 记 gate.check 的 change 绑定与 constraint-injection 的档位绑定
 * 共享同一实现，保证两侧检测一致（同源函数，不引入新误差面）。
 */
import { existsSync, readdirSync, statSync } from "node:fs";
import { join } from "node:path";

export const OPENSPEC_CHANGES_DIR = "openspec/changes";
export const ARCHIVE_DIR = "archive";

export function listChangeDirs(cwd: string): { name: string; dir: string }[] {
	const root = join(cwd, OPENSPEC_CHANGES_DIR);
	if (!existsSync(root)) return [];
	const out: { name: string; dir: string }[] = [];
	for (const name of readdirSync(root)) {
		if (name === ARCHIVE_DIR) continue;
		const dir = join(root, name);
		try {
			if (statSync(dir).isDirectory()) out.push({ name, dir });
		} catch {
			/* ignore */
		}
	}
	return out;
}

/* 活跃 change 兜底判定：目录内最新文件 mtime 最新的非 archive change */
export function detectActiveChange(
	cwd: string,
): { name: string; dir: string } | null {
	let best: { name: string; dir: string; mtime: number } | null = null;
	for (const c of listChangeDirs(cwd)) {
		let mtime = 0;
		try {
			mtime = statSync(c.dir).mtimeMs;
			for (const f of readdirSync(c.dir)) {
				const fm = statSync(join(c.dir, f)).mtimeMs;
				if (fm > mtime) mtime = fm;
			}
		} catch {
			/* ignore */
		}
		if (!best || mtime > best.mtime) best = { ...c, mtime };
	}
	return best ? { name: best.name, dir: best.dir } : null;
}
