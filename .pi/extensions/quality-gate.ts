/**
 * quality-gate.ts — pi extension：turn_end 增量门禁（软提示）
 *
 * 设计决策（见 docs/reference/开发执行规范.md §4.1 门禁分层表）：
 * 1. 监听 turn_end 而非 tool_result：每回合跑一次，避免 agent 批量 edit 触发 N 次门禁
 * 2. 不跑 go test：影响包无法自动判定（codegraph affected 实测误报），全量 go test ./... 100s+Docker+flaky；
 *    测试靠 agent 按 AGENTS.md 手动跑 + §11 归档门禁兜底
 * 3. 前端只跑 pnpm lint：门禁分层设计——typecheck/test:unit/build 留给 agent 手动执行与归档门禁
 *    兜底，与执行平台无关（linux-native-dev-environment 明确：MUST NOT 因平台能力变化扩大门禁范围）
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
 * 13. 执行链路平台分流（linux-native-dev-environment，2026-09）：第 12 条的防护以
 *     「宿主确有 cmd.exe」为前提，该前提在 Linux/macOS 宿主上不成立——那里 cmd.exe
 *     不可达，探测恒失败会让门禁每轮整体短路（静默失效）。故探测前置平台判定：
 *     `cmdExeReachable()` 为假即 native 模式（本机 go/golangci-lint/pnpm + cwd 参数
 *     执行，不探测、不短路、不标 wsl环境）；为真则维持第 12 条既有 cmd.exe 链路语义
 *     （每轮健康探测，因为 vsock 健康状态是逐回合的）。平台身份会话内缓存
 *     （session_start 重置），不逐回合重判——平台不会逐回合变，interop 健康会。
 * 14. native 工具链探测短路（harden-gate-native-toolchain，2026-09）：第 13 条的
 *    「native 不探测」存在盲区——pi 启动环境 PATH 缺 go/golangci-lint/pnpm 时命令
 *    秒败并经粘性放大成连环假失败（2026-09-17 实测两会话 945 条，占 7 天窗口失败
 *    56%）。故 native 模式补齐侧级工具链可达性探测（PATH accessSync 扫描，非 exec——
 *    spawn ENOENT 与真实失败不可区分）：某侧不可达即整体跳过该侧（fail-open）+
 *    边沿记账 policy.decision(toolchain-down)（进入短路态首个回合记一条，探测会话
 *    缓存后结果恒定，每回合重记违反低噪声约束）+ steer 提示修启动环境 PATH；事后
 *    isToolNotFound 特征兑底（探测后 PATH 内文件被删的窗口期），不进粘性不分级。
 *    windows 模式不参与（用 Windows 绝对路径执行，不经 bash PATH 查找）。
 */
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { statSync, accessSync, constants } from "node:fs";
import { join, delimiter } from "node:path";
import { logEvent } from "./lib/harness-log";
import { detectActiveChange } from "./lib/active-change";
import { syncEditMap, resetEditMapState } from "./lib/edit-map";
import { truncateDiagGate, isInteropFailure, isToolNotFound } from "./lib/failure-classify";
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

// 执行链路平台模式（linux-native-dev-environment D3）：null = 本会话未判定。判定结果在
// 会话内稳定复用——native 下不再探测（无跨系统链路可坏），windows 下仍需每回合健康探测
// （vsock 健康是逐回合属性）。修正条件化：只在 null 时判定，不在回合中途反复切换。
let execPlatform: "native" | "windows" | null = null;
// native 工具链可达性（harden-gate-native-toolchain D2/D3）：按侧缓存，null = 未探测；
// 探测结果会话内稳定（PATH 是进程属性不会中途变），session_start/shutdown 重置。
// backend 需 go 且 golangci-lint 双检（任一缺失整侧短路，跑一半只会制造半截信号）；
// frontend 需 pnpm 单检。
let nativeToolchain: { backend: boolean | null; frontend: boolean | null } = {
	backend: null,
	frontend: null,
};
// 短路边沿记账/提示状态（D5）：进入短路态的首个回合记一条 toolchain-down + 发一条
// steer，后续回合短路不重复（探测缓存后结果恒定，重记无信息增量，违反低噪声约束）。
let toolchainDownNotified: { backend: boolean; frontend: boolean } = {
	backend: false,
	frontend: false,
};

/**
 * 指定名字的可执行文件是否在 PATH 上可达（accessSync X_OK 扫描）。
 *
 * 为何只用文件可达性检查而不用「探测调用失败」判定：`pi.exec` 底层 spawn 的 ENOENT
 * 被 execCommand 的 catch 统一转成 `{code:1, stdout:"", stderr:""}`（dist/core/exec.js），
 * 与「可执行文件存在但返回非零」在返回值上无法区分。
 * harden-gate-native-toolchain D1：native 工具链探测复用本函数，零开销且语义无歧义。
 */
function exeReachable(name: string): boolean {
	for (const dir of (process.env.PATH ?? "").split(delimiter)) {
		if (!dir) continue;
		try {
			accessSync(join(dir, name), constants.X_OK);
			return true;
		} catch {
			/* 该目录下没有，继续找 */
		}
	}
	return false;
}

/**
 * cmd.exe 是否可达（平台身份判定的唯一依据）。
 *
 * 为何不用「探测调用失败」来判定：`pi.exec` 底层 spawn 的 ENOENT 被 execCommand 的
 * catch 统一转成 `{code:1, stdout:"", stderr:""}`（dist/core/exec.js），与「cmd.exe 存在
 * 但返回非零」在返回值上无法区分。故先做 PATH 可达性检查：Windows 恒真（System32 必备），
 * WSL 经 appendWindowsPath 通常为真（保持既有 interop 语义），Linux/macOS 原生环境为假。
 */
function cmdExeReachable(): boolean {
	if (process.platform === "win32") return true;
	return exeReachable("cmd.exe") || exeReachable("cmd");
}

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
		execPlatform = null; // 平台判定随会话边界重判（spec：判定在会话内稳定）
		nativeToolchain = { backend: null, frontend: null }; // 工具链探测随会话边界重探（D3）
		toolchainDownNotified = { backend: false, frontend: false }; // 短路边沿状态随会话边界清零（D5）
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
		execPlatform = null;
		nativeToolchain = { backend: null, frontend: null };
		toolchainDownNotified = { backend: false, frontend: false };
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
			// 用 git 仓库根而非 process.cwd()：扩展进程的 cwd 可能不在仓库树下（如 /root/...），
			// winPath 会原样返回非 /mnt 路径，cmd.exe cd 不认 → "系统找不到指定的路径"。
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
		// 短路（step 3.6）可中途置 false，故用 let
		let isBackend = trigBackend || stickyBackend;
		let isFrontend = trigFrontend || stickyFrontend;
		if (!isBackend && !isFrontend) return; // 纯文档改动/无变化放行（跳过侧零记账）

		// 3.5 执行链路判定 + interop 健康探测（harden-gate-interop-health 语义在
		//     linux-native-dev-environment 下扩展为三态）：
		//       native  —— cmd.exe 不可达 → 本机工具链执行（不探测、不短路）
		//       windows —— cmd.exe 可达且探测健康 → 既有 cmd.exe interop 链路
		//       down    —— cmd.exe 可达但探测失败 → 整体短路（fail-open，既有语义）
		//     平台判定只在 execPlatform 为 null 时做一次（会话内稳定）；windows 下探测
		//     仍需每回合执行（vsock 健康是逐回合属性）。探测超时/exit≠0/抛异常同走
		//     down；故障期重试无意义（design Non-Goals），恢复交给人。edit.map
		//     （step 2.5）已在探测前记账，归属地图不受短路影响。
		const sessionId = ctx.sessionManager?.getSessionId?.();
		const probeT0 = Date.now();
		if (execPlatform === null) {
			execPlatform = cmdExeReachable() ? "windows" : "native";
		}
		let interopDown = false;
		if (execPlatform === "windows") {
			try {
				const probe = await pi.exec("cmd.exe", ["/C", "echo ok"], {
					signal: ctx.signal,
					timeout: INTEROP_PROBE_TIMEOUT_MS,
				});
				interopDown = probe.code !== 0;
			} catch {
				interopDown = true; // 超时/异常同 fail-open（spec：探测调用异常也 fail-open）
			}
		}
		if (interopDown) {
			// 短路记账（spec：interop 探测短路被记账且不双写）：一条 policy.decision
			// （统一 helper：reasonCode 白名单归一、durationMs 非有限正数自动省略、
			// change 不传 = 自动检测活跃 change）；被跳过的门禁命令零 gate.check
			// （否则账本退回连环假失败老问题）。记账旁路化（helper 内 fail-loud）。
			// 此分支只在 windows 模式可达（native 不探测）。
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

		// 3.6 native 工具链可达性短路（harden-gate-native-toolchain D1/D2/D3/D5）：
		//     PATH 缺工具链时整侧跳过（fail-open），另一侧照常；探测 accessSync 文件
		//     可达性（非 exec，spawn ENOENT 与真实失败不可区分）；按侧缓存会话内稳定；
		//     进入短路态的首个回合记一条 toolchain-down + 发一条 steer（边沿触发，
		//     探测缓存后结果恒定，每回合重记违反低噪声约束）。windows 模式不参与
		//     （Windows 绝对路径执行，不经 bash PATH 查找）。
		if (execPlatform === "native") {
			if (isBackend && nativeToolchain.backend === null) {
				nativeToolchain.backend = exeReachable("go") && exeReachable("golangci-lint");
			}
			if (isFrontend && nativeToolchain.frontend === null) {
				nativeToolchain.frontend = exeReachable("pnpm");
			}
			if (isBackend && nativeToolchain.backend === false) {
				if (!toolchainDownNotified.backend) {
					toolchainDownNotified.backend = true;
					if (sessionId) {
						logPolicyDecision(repoRoot, {
							sessionId,
							policy: "quality-gate",
							action: "fail-open",
							reasonCode: "toolchain-down",
							target: "backend",
						});
					}
					pi.sendMessage(
						{
							customType: "quality-gate-toolchain-down",
							content:
								"⚠️ 质量门禁本轮跳过（后端）：本机 PATH 缺 go / golangci-lint（非代码问题，本机链路）。请检查 pi 启动环境 PATH（工具链安装位置或启动方式，如经 systemd/桌面自启时未走 login profile），修复后重启会话恢复门禁覆盖",
						},
						{ deliverAs: "steer", triggerTurn: true },
					);
				}
				isBackend = false;
			}
			if (isFrontend && nativeToolchain.frontend === false) {
				if (!toolchainDownNotified.frontend) {
					toolchainDownNotified.frontend = true;
					if (sessionId) {
						logPolicyDecision(repoRoot, {
								sessionId,
								policy: "quality-gate",
								action: "fail-open",
								reasonCode: "toolchain-down",
								target: "frontend",
							});
					}
					pi.sendMessage(
						{
							customType: "quality-gate-toolchain-down",
							content:
								"⚠️ 质量门禁本轮跳过（前端）：本机 PATH 缺 pnpm（非代码问题，本机链路）。请检查 pi 启动环境 PATH（工具链安装位置或启动方式，如经 systemd/桌面自启时未走 login profile），修复后重启会话恢复门禁覆盖",
						},
						{ deliverAs: "steer", triggerTurn: true },
					);
				}
				isFrontend = false;
			}
			if (!isBackend && !isFrontend) return; // 两侧均被短路：本轮零命令零记账（零 gate.check 假失败）
		}

		// 4. 跑快门禁（单条命令量级秒级；命令集与执行平台正交，仅链路不同）
		const failures: string[] = [];
		// 环境故障提示段（harden-gate-interop-health：diag 特征命中的失败单独归因，
		// 不混入门禁失败列表——避免 agent 误当代码问题去修）。harden-gate-native-toolchain
		// D4：kind 分流恢复建议——interop（仅 windows）建议 wsl --shutdown；toolchain
		//（仅 native）建议修 pi 启动环境 PATH，不得错发（spec 链路标注约束）。
		const envFailures: { kind: "interop" | "toolchain"; text: string }[] = [];
		// gate.check 记账（harness-quick-wins D3/D4）：ok=false 全量记 + 分级前缀；
		// ok=true 采样记（锚点 flip / 每 N 连续成功 sampled，状态机 lib/gate-sample.ts）。
		// change 绑定与 constraint-injection 共享 detectActiveChange（同源，lazy 一次）；
		// 无 sessionId 时跳过记账，sticky/采样状态照走（与既有语义一致；
		// sessionId 已在 step 3.5 探测前取得）。
		let boundChange: string | null | undefined;
		// 链路标注（linux-native-dev-environment spec：标注 MUST 与实际执行方式一致）：
		// windows 模式标跨系统链路（wsl环境），native 模式标本机原生——不得张冠李戴。
		const linkTag = execPlatform === "windows" ? " (wsl环境)" : " (本机)";
		const gateLog = (cmd: string, code: number, ms: number, output: string) => {
			const ok = code === 0;
			const d = stepGateOk(gateOkStates.get(cmd) ?? initGateOkState(), ok);
			gateOkStates.set(cmd, d.next);
			// 当前 gateLog 覆盖的命令按模式经 cmd.exe interop（windows）或本机 PATH（native）
			// 执行，失败行按实际链路标注（windows 标（wsl环境）；native 标本机）。
			// spec：不弱化真实失败的修复义务——真实失败仍走粘性+分级。
			if (ok) stickyFailures.delete(cmd);
			else if (execPlatform === "windows" && isInteropFailure(output)) {
				// 环境故障（spec：不进粘性、不按 [回归]/[中间态] 分级）：探测通过但命令
				// 执行中途 interop 挂的兜底；gate.check 照记全量失败（diag 含特征可考古）。
				// 仅 windows 模式参与判定：native 无跨系统链路，同名字符串不得触发环境归因。
				envFailures.push({
					kind: "interop",
					text: `[${cmd}${linkTag}] exit ${code}：输出含 WSL interop 故障特征（UtilAcceptVsock），判定为环境故障而非代码问题；不计入粘性重跑，恢复后自动续跑`,
				});
			} else if (execPlatform === "native" && isToolNotFound(output)) {
				// 工具链缺失归因（harden-gate-native-toolchain D4）：探测缓存后 PATH 内文件
				// 被删的窗口期兑底；不进粘性、不分级，gate.check 照记全量失败（diag 含特征
				// 可考古）。仅 native 模式参与判定：windows 用 Windows 绝对路径执行不经 bash
				// PATH 查找，同名字符串不得触发跨语义归因（与 isInteropFailure 门控对称）。
				envFailures.push({
					kind: "toolchain",
					text: `[${cmd}${linkTag}] exit ${code}：输出含工具缺失特征（command not found），判定为本机工具链环境问题而非代码问题；不计入粘性重跑，修复 pi 启动环境 PATH 后重启会话恢复`,
				});
			} else {
				stickyFailures.add(cmd);
				failures.push(
					`${d.failPrefix}[${cmd}${linkTag}] exit ${code}\n${tail(output, 30)}`,
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
			// 后端门禁双模式（linux-native-dev-environment D2）：
			//   native  —— 本机 PATH 的 go / golangci-lint，工作目录经 ExecOptions.cwd 指定
			//              （不依赖 shell cd，也不做 Windows 路径转换）
			//   windows —— cmd.exe interop 调 Windows 工具链（WSL 侧 Go 版本与 go.mod 不匹配
			//              的历史原因，行为原样保留）。路径用正斜杠且不加引号：cmd.exe /C 对
			//              反斜杠+引号组合会触发引号吞噬（见 windows-scripting-pitfalls skill
			//              铁律 #4），实测 `cd /d "D:\\..." && go build` 报
			//              "文件名、目录名或卷标语法不正确"。项目根无空格，正斜杠裸路径最稳。
			const isNative = execPlatform === "native";
			const backendDir = join(repoRoot, "backend-go");
			const backendWin = `${winPath(repoRoot)}/backend-go`;
			// native 走 PATH；windows 用 Windows 绝对路径，不依赖 PATH——pi extension 的
			// Node.js 进程在 WSL PATH 下找 cmd.exe，但 cmd.exe 继承的环境可能不含
			// %USERPROFILE%\go\bin 等用户 PATH 条目。
			// Windows Go: D:\tool\Go\bin\go.exe (go1.25.6)
			// Windows golangci-lint: C:\Users\Admin\go\bin\golangci-lint.exe (v2.12.2)
			const goExe = isNative ? "go" : "D:\\tool\\Go\\bin\\go.exe";
			const linterExe = isNative
				? "golangci-lint"
				: "C:\\Users\\Admin\\go\\bin\\golangci-lint.exe";
			// D2：lint 先行作短路哨兵——typechecking error 即编译失败，vet/build/test
			// 必然同因失败，跳过执行（未执行零记账）；否则 vet/build 并行。
			// domain tests 保持串行（总预算 5min 语义 + 并发峰值保守，design D2 修订）。
			const runBackend = async (cmdline: string) => {
				const t0 = Date.now();
				const r = isNative
					? await pi.exec("bash", ["-c", cmdline], {
							cwd: backendDir,
							signal: ctx.signal,
							timeout: 120_000,
						})
					: await pi.exec("cmd.exe", ["/C", `cd /d ${backendWin} && ${cmdline}`], {
							signal: ctx.signal,
							timeout: 120_000,
						});
				return { code: r.code, ms: Date.now() - t0, out: `${r.stdout}\n${r.stderr}` };
			};
			// --allow-parallel-runners（linux-native-dev-environment）：开发机可能同时有多个
			// 会话/手动跑 lint（并发 change 共享工作树），golangci-lint 默认对同机实例加文件锁，
			// 后到者报 `parallel golangci-lint is running` exit 3——那是环境冲突不是代码回归，
			// 被 [回归] 分级会诱导 agent 去修不存在的代码问题（2026-09-16 实测踩中）。
			const lint = await runBackend(`${linterExe} run --allow-parallel-runners ./...`);
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
						runBackend(cmdline).then((r) => ({ label, ...r })),
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
						[`${repoRoot}/scripts/harness/change-scope.sh`, "--json"],
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
							const r = await runBackend(t.cmd);
							gateLog(t.cmd, r.code, r.ms, r.out);
						}
					}
				} catch {
					// change-scope 脚本不可用/JSON 解析失败 → fail-open，不影响既有门禁
				}
			}
		}

		if (isFrontend) {
			// D1：门禁走 eslint --cache（增量；windows 分支经 cmd.exe 跑 Windows 侧——
			// 实测 WSL DrvFS I/O 慢 ~17 倍：热缓存 WSL 35s vs Windows 2.1s，eslint 计算本身
			// 只占零头；native 分支本机直接跑，无 DrvFS 开销）。
			// front/package.json 的 pnpm lint 保持全量（人工/归档语义）。
			// eslint.config.* 本会话变过后缓存不可信，去 cache 全量兑底。
			const cacheArgs = eslintCacheOff
				? ""
				: " --cache --cache-location node_modules/.cache/eslint/.eslintcache";
			const t0 = Date.now();
			const r =
				execPlatform === "native"
					? await pi.exec("bash", ["-c", `pnpm exec eslint .${cacheArgs}`], {
							cwd: join(repoRoot, "front"),
							signal: ctx.signal,
							timeout: 120_000,
						})
					: await pi.exec(
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
		// 5.5 环境故障 → 按特征分流软提示（不与门禁失败混排；agent 不应据此修代码）。
		// interop 条目仅 windows 可产生、toolchain 条目仅 native 可产生（gateLog 门控）；
		// 恢复建议按 kind 分流，不得错发（native 场景禁「重启 WSL」类跨系统建议）。
		if (envFailures.length > 0) {
			const interopItems = envFailures.filter((e) => e.kind === "interop");
			const toolchainItems = envFailures.filter((e) => e.kind === "toolchain");
			const parts: string[] = [];
			if (interopItems.length > 0) {
				parts.push(
					`⚠️ 门禁遭遇 WSL interop 环境故障（非代码问题，不计粘性）：\n\n${interopItems.map((e) => e.text).join("\n\n")}\n\n建议：wsl --shutdown 重启 WSL 后继续；恢复后下回合门禁自动续跑`,
				);
			}
			if (toolchainItems.length > 0) {
				parts.push(
					`⚠️ 门禁遭遇本机工具链环境问题（非代码问题，不计粘性）：\n\n${toolchainItems.map((e) => e.text).join("\n\n")}\n\n建议：检查 pi 启动环境 PATH（go/golangci-lint/pnpm 安装位置或启动方式），修复后重启会话恢复门禁覆盖`,
				);
			}
			pi.sendMessage(
				{
					customType: "quality-gate-env-failure",
					content: parts.join("\n\n"),
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
