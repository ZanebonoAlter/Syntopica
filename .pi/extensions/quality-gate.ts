/**
 * quality-gate.ts — pi extension：turn_end 增量门禁（软提示）
 *
 * 设计决策（见 docs/reference/开发执行规范.md §4.1 门禁分层表）：
 * 1. 监听 turn_end 而非 tool_result：每回合跑一次，避免 agent 批量 edit 触发 N 次门禁
 * 2. 不跑 go test：影响包无法自动判定（codegraph affected 实测误报），全量 go test ./... 100s+Docker+flaky；
 *    测试靠 agent 按 AGENTS.md 手动跑 + §11 归档门禁兜底
 * 3. 前端只跑 pnpm lint：唯一 WSL/bash 安全的前端门禁；typecheck/test:unit/build 需 cmd.exe，bash 脆弱
 * 4. 软提示不硬阻断：deliverAs:"steer" 把失败喂回 agent 上下文，让它自己修；不 return block/isError，避免长任务卡死
 * 5. 纯文档/纯对话回合零成本放行（git diff 无 .go/.ts 改动即跳过）
 *
 * 增量路由（harness-observability-fixes design D1，2026-08）：
 * 6. 触发范围 = 本回合相对快照的新增/变化路径，非 git 累积 diff——修复循环里混合改动
 *    每轮全量 7 条命令（pnpm lint 平均 22.3s 黑洞）的根因是后者。会话边界
 *    （session_start 非 startup）以当时 git 状态初始化基线，会话前残留脏文件进基线不触发。
 * 7. 失败粘性：上回合失败未转绿的命令本回合必重跑（即使触发集空/纯对话），防止
 *    「agent 口头说修了但没改文件」时门禁沉默；转绿或会话边界移除。
 *
 * 采样/短路/分级（harness-quick-wins，2026-09，实测依据见该 change explore-findings）：
 * 8. 成功采样记账：ok=false 全量 + ok=true 锚点/每 5 连续成功采样（状态机 lib/gate-sample.ts），
 *    分母按锚点计 1、采样条 ×N 还原（spec：harness-fact-log「门禁记账」）。
 * 9. 同根因短路：lint 输出含 typechecking error 即编译失败，vet/build/test 必红，
 *    跳过执行（未执行零记账）；否则 lint 先行 + vet/build 并行（domain tests 保持串行）。
 * 10. steer 分级：[回归]（曾绿变红，必须修）/ [中间态]（从未绿，轻提示；若正在推进可继续），
 *     归档前全绿硬要求不变（§11 spec-gate 不受分级影响）。
 * 11. 前端门禁走 eslint --cache（增量，22.3s 黑洞→秒级）；eslint.config.* 本会话
 *     变更后去 cache 全量兑底；pnpm lint 脚本保持全量（人工/归档语义）。
 * 12. interop 健康防护（harden-gate-interop-health，2026-09）：后端门禁与 pnpm lint
 *     统一走 cmd.exe interop（vsock），通道级故障（UtilAcceptVsock ETIMEDOUT，实测
 *     8/28、9/3、9/5 三次故障期单次持续 40+ 分钟）会连环打红全部命令且被粘性放大成
 *     假 [回归]。turn_end 门禁前廉价探测（cmd.exe /C echo ok，正常 ~40ms），失败则
 *     本轮 cmd 链路门禁整体短路（fail-open）+ policy.decision(interop-down)，零
 *     gate.check 假失败；diag 特征命中（UtilAcceptVsock / <N>WSL ERROR 前缀）的失败
 *     不进粘性、归因环境；cmd 链路命令失败提示标（wsl环境）。
 */
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { statSync } from "node:fs";
import { join } from "node:path";
import { logEvent } from "./lib/harness-log";
import { detectActiveChange } from "./lib/active-change";
import { syncEditMap, resetEditMapState } from "./lib/edit-map";
import { truncateDiagGate, isInteropFailure } from "./lib/failure-classify";
import { logPolicyDecision } from "./lib/policy-decision";
import { computeTriggerSet, type FileStat } from "./lib/trigger-set";
import {
	initGateOkState,
	stepGateOk,
	isCompileFailure,
	type GateOkState,
} from "./lib/gate-sample";

const CODE_TOOLS = new Set(["edit", "write", "bash", "apply_patch"]);
// interop 探测超时（harden-gate-interop-health D1）：正常 ~40ms，2s = 50 倍余量容忍
// 宿主瞬时卡顿，又远小于故障期单命令 ~10s 白等。特征识别见 lib/failure-classify
// isInteropFailure（双关键字并集锚定）。
const INTEROP_PROBE_TIMEOUT_MS = 2_000;
// edit.map 记账门控（fix-doc-impact-misattribution）：仅纯编辑工具回合记账。
// bash 回合（含只读命令/git 操作/构建）的快照 diff 会检出其他并行会话同期的
// 文件变化，记账会把他人改动误归到本会话绑定的 change 头上（会话间 mtime
// 串扰，第五误归因通道）；纯编辑工具的快照变化与本次调用强相关。
const EDIT_TOOLS = new Set(["edit", "write", "apply_patch"]);

/* 模块级会话状态（session_start 重置；ESM 模块缓存可能跨会话残留，显式清零） */
// git 脏文件快照（tracked diff + untracked 的 {mtime,size}）；null = 待初始化
let snapshot: Map<string, FileStat> | null = null;
// 上回合失败未转绿的命令标识（三件套 label / domain go test cmd / "pnpm lint"）
let stickyFailures = new Set<string>();
// 成功采样记账状态机 per (session, cmd)（harness-quick-wins D3/D4：锚点/采样/everGreen）
let gateOkStates = new Map<string, GateOkState>();
// eslint 配置本会话变更过 → 前端门禁去 cache 全量（harness-quick-wins D1 兑底）
let eslintCacheOff = false;
// 状态归属会话（防子线程 session 事件误清主会话状态；pi-subagents 共用模块实例）
let ownerSessionId: string | null = null;

/** git 脏文件集（tracked diff + untracked）→ {mtime,size} 快照；消失文件跳过。 */
function statGitFiles(repoRoot: string, files: string): Map<string, FileStat> {
	const out = new Map<string, FileStat>();
	for (const rel of files
		.split("\n")
		.map((s) => s.trim())
		.filter(Boolean)) {
		try {
			const st = statSync(join(repoRoot, rel));
			// mtime 取整毫秒：drvfs 的 mtimeMs 有亚毫秒抖动，同文件未动时两次读数需稳定
			out.set(rel, { mtimeMs: Math.round(st.mtimeMs), size: st.size });
		} catch {
			/* rename 中间态等消失文件：跳过（下回合重新出现按新增触发） */
		}
	}
	return out;
}

export default function (pi: ExtensionAPI) {
	pi.on("session_start", async (event, ctx) => {
		// ⚠ reason "startup" 跳过（与 constraint-injection 同款防御）：pi-subagents 派发
		// 子线程时向同一共享模块实例发 session_start{startup}——清掉会把主会话
		// 快照/粘性清零（实测教训见 constraint-injection.ts 注释）。
		if (event.reason === "startup") return;
		ownerSessionId = ctx.sessionManager?.getSessionId?.() ?? null;
		snapshot = null;
		stickyFailures = new Set();
		gateOkStates = new Map();
		eslintCacheOff = false;
		resetEditMapState(); // edit.map 归属累计随会话边界清零（跨 session 并集由库内 base 续接）
		// 基线初始化：会话开始时的 git 脏文件（含上个会话残留）全部进基线不触发
		try {
			const changed = await pi.exec("git", ["diff", "--name-only", "HEAD"], {
				timeout: 10_000,
			});
			const untracked = await pi.exec(
				"git",
				["ls-files", "--others", "--exclude-standard"],
				{ timeout: 10_000 },
			);
			const toplevel = await pi.exec("git", ["rev-parse", "--show-toplevel"], {
				timeout: 10_000,
			});
			const root = toplevel.stdout.trim();
			if (root) {
				snapshot = statGitFiles(
					root,
					`${changed.stdout}\n${untracked.stdout}`,
				);
			}
		} catch {
			/* 非 git 仓库/异常：保持 null，turn_end 时 lazy 兜底 */
		}
	});

	pi.on("session_shutdown", async (_event, ctx) => {
		// 仅归属会话自身的 shutdown 清理（子线程结束的 shutdown 不动主会话状态）
		const sid = ctx.sessionManager?.getSessionId?.() ?? null;
		if (ownerSessionId && sid && sid !== ownerSessionId) return;
		snapshot = null;
		stickyFailures = new Set();
		gateOkStates = new Map();
		eslintCacheOff = false;
		ownerSessionId = null;
		resetEditMapState();
	});

	pi.on("turn_end", async (event, ctx) => {
		// 1. 本回合有代码类工具调用才触发；纯对话回合零成本跳过。
		//    粘性失败非空时例外：即使纯对话也重跑失败命令（design D1 失败粘性）。
		const touchedCode = (event.toolResults ?? []).some((r) =>
			CODE_TOOLS.has(r.toolName),
		);
		if (!touchedCode && stickyFailures.size === 0) return;

		// 2. git 脏文件集（已跟踪 + 未跟踪新文件，Write 产生的也算）
		let files = "";
		let repoRoot = "";
		try {
			const changed = await pi.exec("git", ["diff", "--name-only", "HEAD"], {
				signal: ctx.signal,
				timeout: 10_000,
			});
			const untracked = await pi.exec(
				"git",
				["ls-files", "--others", "--exclude-standard"],
				{ signal: ctx.signal, timeout: 10_000 },
			);
			files = `${changed.stdout}\n${untracked.stdout}`;
			// 用 git 仓库根而非 process.cwd()：扩展进程的 cwd 可能不在 /mnt 下（如 /root/...），
			// winPath 会原样返回 WSL 路径，cmd.exe cd 不认 → "系统找不到指定的路径"。
			const toplevel = await pi.exec("git", ["rev-parse", "--show-toplevel"], {
				signal: ctx.signal,
				timeout: 10_000,
			});
			repoRoot = toplevel.stdout.trim();
		} catch {
			return; // git 不可用（非 git 仓库/异常），静默放行，不阻塞 agent
		}
		const curr = statGitFiles(repoRoot, files);
		if (snapshot === null) snapshot = curr; // lazy 兜底：session_start 未初始化时以当前状态为基线
		const trigger = computeTriggerSet(snapshot, curr);
		snapshot = curr; // 判定后更新快照（无论本回合是否跑门禁）

		// 2.5 edit.map 归属聚合（coordinate-concurrent-changes；门控收紧见
		//     fix-doc-impact-misattribution）：仅本回合有纯编辑工具（edit/write/apply_patch）
		//     调用时记账——bash 回合快照 diff 检出的其他会话文件变化不串扰进归属集合
		//     （会话间 mtime 串扰，第五误归因通道）；工具自管路径黑名单在 syncEditMap
		//     内部兑底。文档编辑也进归属地图；纯对话回合已在 step 1 早退（syncEditMap
		//     内吞异常，不影响门禁）。
		if (
			trigger.size > 0 &&
			(event.toolResults ?? []).some((r) => EDIT_TOOLS.has(r.toolName))
		) {
			const sidEm = ctx.sessionManager?.getSessionId?.();
			if (sidEm) syncEditMap(repoRoot, sidEm, trigger);
		}

		// D1 兑底：eslint 配置文件本会话变更过 → 缓存不可信（一旦命中，会话内持续全量）
		if ([...trigger].some((p) => /^front\/eslint\.config\.[^/]+$/.test(p))) {
			eslintCacheOff = true;
		}

		// 3. 增量路由两侧：触发集命中，或该侧存在粘性失败命令（重跑催修）
		const trigBackend = [...trigger].some((p) => /^backend-go\/.*\.go$/.test(p));
		const trigFrontend = [...trigger].some(
			(p) => /^front\//.test(p) && !/\.md$/.test(p),
		);
		// 粘性命令永远不是 "pnpm lint" 即后端侧；pnpm lint 即前端侧
		const stickyBackend = [...stickyFailures].some((c) => c !== "pnpm lint");
		const stickyFrontend = stickyFailures.has("pnpm lint");
		const isBackend = trigBackend || stickyBackend;
		const isFrontend = trigFrontend || stickyFrontend;
		if (!isBackend && !isFrontend) return; // 纯文档改动/无变化放行（跳过侧零记账）

		// 3.5 interop 健康探测（harden-gate-interop-health）：本轮需执行 cmd 链路门禁
		//     （后端三件套+域测试 / pnpm lint 共享同一 vsock 通道）前探测一次，失败则
		//     整体短路（fail-open）。探测超时/exit≠0/抛异常同路径；故障期重试无意义
		//     （design Non-Goals），恢复交给人（wsl --shutdown）。edit.map（step 2.5）已
		//     在探测前记账，归属地图不受短路影响。
		const sessionId = ctx.sessionManager?.getSessionId?.();
		const probeT0 = Date.now();
		let interopDown = false;
		try {
			const probe = await pi.exec("cmd.exe", ["/C", "echo ok"], {
				signal: ctx.signal,
				timeout: INTEROP_PROBE_TIMEOUT_MS,
			});
			interopDown = probe.code !== 0;
		} catch {
			interopDown = true; // 超时/异常同 fail-open（spec：探测调用异常也 fail-open）
		}
		if (interopDown) {
			// 短路记账（spec：interop 探测短路被记账且不双写）：一条 policy.decision
			// （统一 helper：reasonCode 白名单归一、durationMs 非有限正数自动省略、
			// change 不传 = 自动检测活跃 change）；被跳过的门禁命令零 gate.check
			// （否则账本退回连环假失败老问题）。记账旁路化（helper 内 fail-loud）。
			if (sessionId) {
				logPolicyDecision(repoRoot, {
					sessionId,
					policy: "quality-gate",
					action: "fail-open",
					reasonCode: "interop-down",
					durationMs: Date.now() - probeT0,
				});
			}
			pi.sendMessage(
				{
					customType: "quality-gate-interop-down",
					content:
						"⚠️ 质量门禁本轮跳过：WSL interop 环境故障（cmd.exe 探测失败，非代码问题，(wsl环境) 链路）。建议重启 WSL（wsl --shutdown）后继续；本回合改动暂未被门禁覆盖，恢复后下回合自动续跑",
				},
				{ deliverAs: "steer", triggerTurn: true },
			);
			return;
		}

		// 4. 跑快门禁（全部 <5s，WSL/bash 安全）
		const failures: string[] = [];
		// 环境故障提示段（harden-gate-interop-health：diag 特征命中的失败单独归因，
		// 不混入门禁失败列表——避免 agent 误当代码问题去修）
		const envFailures: string[] = [];
		// gate.check 记账（harness-quick-wins D3/D4）：ok=false 全量记 + 分级前缀；
		// ok=true 采样记（锚点 flip / 每 N 连续成功 sampled，状态机 lib/gate-sample.ts）。
		// change 绑定与 constraint-injection 共享 detectActiveChange（同源，lazy 一次）；
		// 无 sessionId 时跳过记账，sticky/采样状态照走（与既有语义一致；
		// sessionId 已在 step 3.5 探测前取得）。
		let boundChange: string | null | undefined;
		const gateLog = (cmd: string, code: number, ms: number, output: string) => {
			const ok = code === 0;
			const d = stepGateOk(gateOkStates.get(cmd) ?? initGateOkState(), ok);
			gateOkStates.set(cmd, d.next);
			// 当前 gateLog 覆盖的命令（后端三件套/域测试/pnpm lint）全部经 cmd.exe
			// interop 执行，失败行统一标（wsl环境）链路标注（spec：不弱化真实失败的
			// 修复义务——真实失败仍走粘性+分级）。
			if (ok) stickyFailures.delete(cmd);
			else if (isInteropFailure(output)) {
				// 环境故障（spec：不进粘性、不按 [回归]/[中间态] 分级）：探测通过但命令
				// 执行中途 interop 挂的兜底；gate.check 照记全量失败（diag 含特征可考古）。
				envFailures.push(
					`[${cmd} (wsl环境)] exit ${code}：输出含 WSL interop 故障特征（UtilAcceptVsock），判定为环境故障而非代码问题；不计入粘性重跑，恢复后自动续跑`,
				);
			} else {
				stickyFailures.add(cmd);
				failures.push(
					`${d.failPrefix}[${cmd} (wsl环境)] exit ${code}\n${tail(output, 30)}`,
				);
			}
			if (!sessionId || !d.log) return;
			if (boundChange === undefined) {
				boundChange = detectActiveChange(repoRoot)?.name ?? null;
			}
			logEvent(repoRoot, {
				kind: "gate.check",
				sessionId,
				change: boundChange,
				payload: {
					cmd,
					phase: "turn_end",
					ok,
					ms,
					diag: ok ? null : truncateDiagGate(output),
					...(d.flip ? { flip: true } : {}),
					...(d.sampled ? { sampled: true, n: d.n } : {}),
				},
			});
		};

		if (isBackend) {
			// 后端 Go 工具链在 Windows（D:\tool\Go\bin\go.exe = go1.25.6）；WSL 仅有 Go 1.18，
			// 其 go.mod 解析器不认 `go 1.25.0`（Go 1.21+ 三段式合法格式）→ 假阳性。
			// 故后端门禁必须走 cmd.exe 调 Windows Go（与前端 cmd.exe 规范一致，见 AGENTS.md）。
			// 路径用正斜杠且不加引号：cmd.exe /C 对反斜杠+引号组合会触发引号吞噬（见
			// windows-scripting-pitfalls skill 铁律 #4），实测 `cd /d "D:\\..." && go build` 报
			// "文件名、目录名或卷标语法不正确"。项目根无空格，正斜杠裸路径最稳。
			const backendWin = `${winPath(repoRoot)}/backend-go`;
			// 用 Windows 绝对路径，不依赖 PATH——pi extension 的 Node.js 进程在 WSL PATH 下
			// 找 cmd.exe，但 cmd.exe 继承的环境可能不含 %USERPROFILE%\go\bin 等用户 PATH 条目。
			// Windows Go: D:\tool\Go\bin\go.exe (go1.25.6)
			// Windows golangci-lint: C:\Users\Admin\go\bin\golangci-lint.exe (v2.12.2)
			const goExe = "D:\\tool\\Go\\bin\\go.exe";
			const linterExe = "C:\\Users\\Admin\\go\\bin\\golangci-lint.exe";
			// D2：lint 先行作短路哨兵——typechecking error 即编译失败，vet/build/test
			// 必然同因失败，跳过执行（未执行零记账）；否则 vet/build 并行。
			// domain tests 保持串行（总预算 5min 语义 + 并发峰值保守，design D2 修订）。
			const runWin = async (cmdline: string) => {
				const t0 = Date.now();
				const r = await pi.exec(
					"cmd.exe",
					["/C", `cd /d ${backendWin} && ${cmdline}`],
					{ signal: ctx.signal, timeout: 120_000 },
				);
				return { code: r.code, ms: Date.now() - t0, out: `${r.stdout}\n${r.stderr}` };
			};
			const lint = await runWin(`${linterExe} run ./...`);
			gateLog("golangci-lint", lint.code, lint.ms, lint.out);
			if (lint.code !== 0 && isCompileFailure(lint.out)) {
				// 同根因短路：本回合仅 lint 一条事件（spec scenario「同根因短路未执行不记账」）
			} else {
				const rest: ReadonlyArray<readonly [string, string]> = [
					["go vet", `${goExe} vet ./...`],
					["go build", `${goExe} build ./...`],
				];
				const rs = await Promise.all(
					rest.map(([label, cmdline]) =>
						runWin(cmdline).then((r) => ({ label, ...r })),
					),
				);
				for (const { label, code, ms, out } of rs) gateLog(label, code, ms, out);

				// 影响包测试（tier=domain）：change-scope 路径映射 → go test -short，仅跑本轮
				// 改动命中的 domain 包（-short 下 DB 集成测试自动 skip，无需 Docker）。
				// platform/skeleton 档不另跑 test：上方全量 vet/build 已覆盖。
				// 总预算 5min，超预算的包 warn 不跑（计入 failures 但语义是提醒；未执行不记账）。
				try {
					const scope = await pi.exec(
						"bash",
						[`${repoRoot}/scripts/change-scope.sh`, "--json"],
						{ signal: ctx.signal, timeout: 15_000 },
					);
					if (scope.code === 0) {
						const targets = JSON.parse(scope.stdout) as {
							testTargets: { cmd: string; tier: string }[];
						};
						const domainTests = targets.testTargets.filter(
							(t) => t.tier === "domain",
						);
						const deadline = Date.now() + 300_000;
						for (const t of domainTests) {
							if (Date.now() > deadline) {
								failures.push(
									`[${t.cmd}] 跳过：domain 测试总预算 5min 已耗尽，请手动补跑`,
								);
								continue;
							}
							const r = await runWin(t.cmd);
							gateLog(t.cmd, r.code, r.ms, r.out);
						}
					}
				} catch {
					// change-scope 脚本不可用/JSON 解析失败 → fail-open，不影响既有门禁
				}
			}
		}

		if (isFrontend) {
			// D1：门禁走 Windows 侧 eslint --cache（增量；实测 WSL DrvFS I/O 慢 ~17 倍：
			// 热缓存 WSL 35s vs Windows 2.1s，eslint 计算本身只占零头）；
			// front/package.json 的 pnpm lint 保持全量（人工/归档语义，WSL 可跑）。
			// eslint.config.* 本会话变过后缓存不可信，去 cache 全量兑底。
			const cacheArgs = eslintCacheOff
				? ""
				: " --cache --cache-location node_modules/.cache/eslint/.eslintcache";
			const t0 = Date.now();
			const r = await pi.exec(
				"cmd.exe",
				[
					"/C",
					`cd /d ${winPath(repoRoot)}/front && pnpm exec eslint .${cacheArgs}`,
				],
				{ signal: ctx.signal, timeout: 120_000 },
			);
			gateLog("pnpm lint", r.code, Date.now() - t0, `${r.stdout}\n${r.stderr}`);
		}

		// 5. 失败 → 软提示（steer：本回合工具跑完后、下次 LLM 调用前注入）
		if (failures.length > 0) {
			pi.sendMessage(
				{
					customType: "quality-gate-failure",
					content: `⚠️ 增量门禁未通过（[回归]=上回合尚绿必须修复；[中间态]=新代码中间态，若正在推进可继续，回合末复检；归档前全绿硬要求不变）：\n\n${failures.join("\n\n")}`,
				},
				{ deliverAs: "steer", triggerTurn: true },
			);
		}
		// 5.5 环境故障 → 单独软提示（不与门禁失败混排；agent 不应据此修代码）
		if (envFailures.length > 0) {
			pi.sendMessage(
				{
					customType: "quality-gate-interop-failure",
					content: `⚠️ 门禁遭遇 WSL interop 环境故障（非代码问题，不计粘性）：\n\n${envFailures.join("\n\n")}\n\n建议：wsl --shutdown 重启 WSL 后继续；恢复后下回合门禁自动续跑`,
				},
				{ deliverAs: "steer", triggerTurn: true },
			);
		}
	});
}

/** 取字符串尾部 n 行，避免 payload 过大 */
function tail(s: string, n: number): string {
	const lines = s.split("\n");
	return lines.slice(Math.max(0, lines.length - n)).join("\n");
}

/** WSL 路径 → Windows 路径（/mnt/d/... → D:/...，用正斜杠避免 cmd.exe 引号吞噬，见 skill 铁律 #4）；非 /mnt 挂载路径原样返回 */
function winPath(p: string): string {
	const m = p.match(/^\/mnt\/([a-z])\/(.*)$/);
	if (!m) return p;
	return `${m[1].toUpperCase()}:/${m[2]}`;
}
