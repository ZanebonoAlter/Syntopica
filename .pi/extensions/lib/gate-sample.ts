/**
 * gate-sample.ts — quality-gate 成功采样记账状态机 + 同根因短路判定
 * （harness-quick-wins design D2/D3/D4）
 *
 * 契约锚：openspec/changes/harness-quick-wins/specs/harness-fact-log/spec.md
 * （MODIFIED「门禁记账」：ok=true 采样记账 / ok=false 全量 / 同根因短路未执行不记账）。
 * 纯函数无副作用：quality-gate.ts 持会话内 Map<cmd, GateOkState>，每命令执行后调
 * stepGateOk；分母统计口径 = 失败条数 + 锚点计 1 + 采样条 ×N（见 spec）。
 */

/** 连续成功每 N 次落库 1 条采样事件（缺省 5；锚点不占 N 周期） */
export const GATE_OK_SAMPLE_EVERY = 5;

export interface GateOkState {
	/** 当前连续成功计数（锚点后为 1；采样命中后归 0 重新起算） */
	consecOk: number;
	/** 上一轮该命令是否失败；初始视为 true——会话首条成功即锚点（与转绿锚点同规则） */
	lastWasFail: boolean;
	/** 本会话该命令是否曾成功过（曾绿）——steer 语气分级信号 */
	everGreen: boolean;
}

/** 会话初始态（每个 (session, cmd) 一份，session_start 非 startup 时重置）。 */
export function initGateOkState(): GateOkState {
	return { consecOk: 0, lastWasFail: true, everGreen: false };
}

export interface GateOkDecision {
	/** 本轮是否落库 gate.check（ok=false 恒 true；ok=true 仅锚点 / 采样点） */
	log: boolean;
	/** payload 附加标记：flip（会话锚点 / 转绿锚点）或 sampled+n（采样条） */
	flip?: true;
	sampled?: true;
	n?: number;
	/** 失败 steer 前缀（ok=true 为 null）：[回归]=曾绿变红（强催修）/ [中间态]=从未绿（轻提示） */
	failPrefix: "[回归]" | "[中间态]" | null;
	next: GateOkState;
}

export function stepGateOk(st: GateOkState, ok: boolean): GateOkDecision {
	if (!ok) {
		return {
			log: true,
			failPrefix: st.everGreen ? "[回归]" : "[中间态]",
			next: { consecOk: 0, lastWasFail: true, everGreen: st.everGreen },
		};
	}
	// 锚点：会话内首个成功（!everGreen）或失败转绿后的首个成功（lastWasFail）
	if (st.lastWasFail || !st.everGreen) {
		return {
			log: true,
			flip: true,
			failPrefix: null,
			next: { consecOk: 1, lastWasFail: false, everGreen: true },
		};
	}
	const consec = st.consecOk + 1;
	if (consec >= GATE_OK_SAMPLE_EVERY) {
		return {
			log: true,
			sampled: true,
			n: GATE_OK_SAMPLE_EVERY,
			failPrefix: null,
			next: { consecOk: 0, lastWasFail: false, everGreen: true },
		};
	}
	return {
		log: false,
		failPrefix: null,
		next: { consecOk: consec, lastWasFail: false, everGreen: true },
	};
}

/**
 * 同根因编译失败判定（D2 短路哨兵）：golangci-lint 输出含 `typechecking error`
 * （golangci 的 typechecking 错 = go 编译器前端错误）或 `[build failed]` 时，
 * go vet / go build / domain go test 必然同因失败——本轮跳过执行（未执行零记账）。
 * 刻意窄匹配：unused / gofmt 等 lint 规则错误不含这些特征，不误短路。
 * 确定性：同输出字节同判定（纯字符串包含）。
 */
export function isCompileFailure(lintOutput: string): boolean {
	return (
		lintOutput.includes("typechecking error") ||
		lintOutput.includes("[build failed]")
	);
}
