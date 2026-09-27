/**
 * quota-gate.ts — pi extension：Agent 子线程派发前额度门禁
 *
 * 设计决策（见 openspec/changes/agent-quota-gate/design.md D1-D5）：
 * 1. 挂 tool_call 事件，仅 event.toolName === "Agent"（pi-subagents 注册的子线程工具）时介入；
 *    其余工具调用零开销直接放行，不发起任何网络请求
 * 2. provider 解析：model 参数为 provider/modelId 全称直取 provider；省略 model 取会话默认
 *    （ctx.model.provider）；裸 modelId 按 pi 的字母序规则解析（getAvailable 里筛 id 匹配、
 *    按 provider 名排序取第一个），解析结果 ≠ 默认供应商时记 warning 并在阻断 reason 里标注
 * 3. per-provider 适配器归一化为统一 QuotaStatus；无适配器 provider（opencode-go 等）放行，
 *    每会话用 ctx.ui.notify 提示一次
 * 4. 内存缓存（TTL 默认 3 分钟）+ 同 provider 并发共享 in-flight Promise；请求 5s 超时、
 *    带浏览器 UA；全链路 try/catch，任何异常/非预期格式一律 unknown → fail-open 放行
 * 5. 仅判定为 low/exhausted 时返回 { block: true, reason } 阻断；reason 中文，含 provider、
 *    各窗口/余额剩余、最近重置时间、建议动作（换 provider 或等重置）
 *
 * 阈值：窗口剩余百分比默认 10（QUOTA_GATE_WINDOW_PCT）、DeepSeek 余额默认 1 元
 * （QUOTA_GATE_MIN_BALANCE）、缓存 TTL 默认 3 分钟（QUOTA_GATE_TTL_MS）。
 */
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { logPolicyDecision, type PolicyDecisionInput } from "./lib/policy-decision";

// ---------- 阈值常量（环境变量可覆盖） ----------

/** 窗口（5h/周）剩余百分比阈值：低于此值视为见底 */
const WINDOW_PCT = envNum("QUOTA_GATE_WINDOW_PCT", 10);
/** DeepSeek 余额阈值（元） */
const MIN_BALANCE = envNum("QUOTA_GATE_MIN_BALANCE", 1);
/** 额度查询结果缓存 TTL（毫秒） */
const TTL_MS = envNum("QUOTA_GATE_TTL_MS", 3 * 60 * 1000);
/** 单次额度查询超时上限（秒），避免 tool_call 拖慢派发 */
const REQUEST_TIMEOUT_MS = 5000;
/** 浏览器 UA：GLM 端点有反爬，模拟浏览器请求头 */
const BROWSER_UA =
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36";

// ---------- 统一判定模型 ----------

type QuotaStatus = {
	kind: "ok" | "low" | "exhausted" | "unknown";
	/** 中文人话摘要，如 "5h窗口剩 8%，每周剩 42%" */
	summary: string;
	/** 最近的重置时间描述 */
	resetHint?: string;
};

/** 单个额度窗口（GLM/Kimi 的 5h 窗口与每周窗口共用此结构） */
type WindowInfo = {
	label: string;
	/** 剩余百分比 0-100 */
	remainPct: number;
	/** 重置时间：GLM 为 epoch 毫秒数，Kimi 为 ISO 字符串 */
	resetTime?: string | number;
};

/** 适配器签名：入参 apiKey + 取消信号，输出统一状态 */
type QuotaAdapter = (apiKey: string, signal: AbortSignal) => Promise<QuotaStatus>;

/**
 * 本扩展只用到的 ExtensionContext 子集（结构兼容，便于独立类型检查）。
 * ctx.model?.provider、ctx.modelRegistry、ctx.ui.notify、ctx.signal 均来自 pi 运行时。
 */
type GateCtx = {
	model?: { provider?: string } | undefined;
	modelRegistry: {
		getAvailable(): { id: string; provider: string }[];
		getProviderAuth(
			provider: string,
		): Promise<{ auth?: { apiKey?: string } } | undefined>;
	};
	ui: { notify(message: string, type?: "info" | "warning" | "error"): void };
	signal?: AbortSignal | undefined;
	cwd?: string;
	sessionManager?: { getSessionId?(): string | undefined } | undefined;
};

// ---------- 缓存与状态 ----------

/** per-provider 结果缓存 */
const cache = new Map<string, { status: QuotaStatus; fetchedAt: number }>();
/** 同 provider 并发查询共享的 in-flight Promise */
const inflight = new Map<string, Promise<QuotaStatus>>();
/** 已提示过"未接入额度查询"的 provider（每会话一次） */
const notifiedProviders = new Set<string>();
/** 主入口注入的 pi 实例：getQuota 等模块级函数落 custom_message 用 */
let piRef: ExtensionAPI | null = null;

// ---------- 主入口 ----------

export default function (pi: ExtensionAPI) {
	piRef = pi; // 供模块级函数（getQuota fail-open 分支）落 custom_message
	pi.on("tool_call", async (event, ctx) => {
		// D1：只处理 Agent 工具，其余工具零开销放行
		if (event.toolName !== "Agent") return;
		try {
			return await gateAgentDispatch(event.input, ctx);
		} catch (err) {
			// 扩展自身异常绝不阻断派发（fail-open）
			console.warn(`[quota-gate] 未预期异常，放行：${String(err)}`);
			return;
		}
	});
}

/** 对一次 Agent 派发执行额度判定，返回 { block: true, reason } 或 undefined（放行） */
async function gateAgentDispatch(
	input: Record<string, unknown>,
	ctx: GateCtx,
): Promise<{ block: true; reason: string } | undefined> {
	// D2：解析目标 provider；fuzzy 解析风险是用户可感知决策，记 policy.decision(warn)
	const { provider, fuzzyRisk, defaultProvider } = resolveProvider(input, ctx);
	if (!provider) {
		// 无法确定 provider（无 model 参数且会话无默认模型）→ 放行
		console.warn("[quota-gate] 无法解析目标 provider，放行");
		return;
	}
	if (fuzzyRisk) {
		auditPolicy(ctx, {
			policy: "quota-gate",
			action: "warn",
			reasonCode: "fuzzy-model-resolve",
			target: provider,
		});
	}

	const adapter = QUOTA_ADAPTERS[provider];
	if (!adapter) {
		// D5：无适配器 provider（opencode-go 等）→ 放行 + 每会话提示一次
		if (!notifiedProviders.has(provider)) {
			notifiedProviders.add(provider);
			try {
				ctx.ui.notify(
					`[quota-gate] ${displayName(provider)} 暂未接入额度查询，本次派发已放行`,
					"info",
				);
			} catch {
				// UI 不可用时静默
			}
		}
		console.log(`[quota-gate] ${provider} 无适配器，放行`);
		return;
	}

	// 缓存命中/并发去重/失败兜底都在 getQuota 内
	const status = await getQuota(provider, adapter, ctx);
	console.log(`[quota-gate] ${provider}：${status.kind} | ${status.summary}`);

	if (status.kind === "ok" || status.kind === "unknown") return; // 放行（健康/查询异常已另行记账，零 policy.decision）

	// low/exhausted → 阻断，reason 喂回主线程 LLM 供其换模型重试
	auditPolicy(ctx, {
		policy: "quota-gate",
		action: "block",
		reasonCode: status.kind === "exhausted" ? "quota-exhausted" : "quota-low",
		target: provider,
	});
	return {
		block: true,
		reason: buildBlockReason(provider, status, { fuzzyRisk, defaultProvider }),
	};
}

// ---------- provider 解析（D2，与 AGENTS.md 硬规则对齐） ----------

function resolveProvider(
	input: Record<string, unknown>,
	ctx: GateCtx,
): { provider: string; fuzzyRisk: boolean; defaultProvider?: string } {
	const defaultProvider = ctx.model?.provider;
	const modelArg = typeof input.model === "string" ? input.model.trim() : "";

	// 1. 省略 model → 会话默认 provider
	if (!modelArg) return { provider: defaultProvider ?? "", fuzzyRisk: false, defaultProvider };

	// 2. provider/modelId 全称 → 直取 provider
	const slash = modelArg.indexOf("/");
	if (slash !== -1) {
		const provider = modelArg.slice(0, slash).trim() || defaultProvider || "";
		return { provider, fuzzyRisk: false, defaultProvider };
	}

	// 3. 裸 modelId → 按 pi 的字母序规则解析（getAvailable 筛 id 匹配，provider 名排序取第一个）
	const available = ctx.modelRegistry.getAvailable();
	const providerSet = new Set<string>();
	for (const m of available) {
		if (m.id === modelArg || m.id.includes(modelArg)) providerSet.add(m.provider);
	}
	let provider: string;
	if (providerSet.size === 0) {
		provider = defaultProvider ?? "";
	} else {
		provider = [...providerSet].sort()[0];
	}
	// 解析结果 ≠ 默认供应商 → 记 warning（AGENTS.md 的 fuzzy 名坑）
	const fuzzyRisk = provider !== defaultProvider;
	if (fuzzyRisk) {
		console.warn(
			`[quota-gate] model="${modelArg}" 为裸模型名，按字母序解析到 ${provider}（默认 ${defaultProvider ?? "无"}），建议显式写 provider/modelId 全称`,
		);
	}
	return { provider, fuzzyRisk, defaultProvider };
}

// ---------- 额度查询（缓存 + in-flight 去重 + fail-open） ----------

async function getQuota(provider: string, adapter: QuotaAdapter, ctx: GateCtx): Promise<QuotaStatus> {
	// 缓存命中 → 零网络开销
	const now = Date.now();
	const hit = cache.get(provider);
	if (hit && now - hit.fetchedAt < TTL_MS) return hit.status;

	// 同 provider 并发查询共享 in-flight Promise
	const pending = inflight.get(provider);
	if (pending) return pending;

	const p = (async (): Promise<QuotaStatus> => {
		try {
			// D2：认证一律走 modelRegistry.getProviderAuth，不读 auth.json
			const authResult = await ctx.modelRegistry.getProviderAuth(provider);
			const apiKey = authResult?.auth?.apiKey;
			if (!apiKey) {
				console.warn(`[quota-gate] ${provider} 未配置 apiKey，判定 unknown 放行`);
				return { kind: "unknown", summary: "未配置 apiKey" };
			}
			const signal = buildSignal(ctx.signal);
			const status = await adapter(apiKey, signal);
			cache.set(provider, { status, fetchedAt: Date.now() });
			return status;
		} catch (err) {
			// 查询失败/超时/被反爬 → unknown，fail-open 不阻断
			console.warn(`[quota-gate] ${provider} 额度查询失败，fail-open 放行：${String(err)}`);
			auditPolicy(ctx, {
				policy: "quota-gate",
				action: "fail-open",
				reasonCode: "quota-query-failed",
				target: provider,
			});
			// custom_message 落盘留痕（不静默）：谁在何时因什么失败被放行，会话里可查
		try {
			piRef?.sendMessage(
				{
					customType: "quota-gate-warning",
					content: `⚠️ [quota-gate] ${displayName(provider)} 额度查询失败已放行（fail-open）。失败原因：${String(err)}`,
					display: true,
				},
				{ deliverAs: "steer" },
			);
			} catch (sendErr) {
				// sendMessage 不可用时仅 console 留痕，不影响放行
				console.warn(`[quota-gate] custom_message 落盘失败：${String(sendErr)}`);
			}
			return { kind: "unknown", summary: "额度查询失败" };
		} finally {
			inflight.delete(provider);
		}
	})();
	inflight.set(provider, p);
	return p;
}

/** 5s 超时 + 会话取消信号合并 */
function buildSignal(sessionSignal?: AbortSignal): AbortSignal {
	const timeout = AbortSignal.timeout(REQUEST_TIMEOUT_MS);
	if (!sessionSignal) return timeout;
	try {
		return AbortSignal.any([timeout, sessionSignal]);
	} catch {
		return timeout;
	}
}

/** GET JSON 的公共守卫：非 2xx / 空响应 / 非 JSON 一律抛错（由调用方兜成 unknown） */
async function fetchJson(
	url: string,
	headers: Record<string, string>,
	signal: AbortSignal,
): Promise<unknown> {
	const resp = await fetch(url, { method: "GET", headers, signal, redirect: "follow" });
	if (!resp.ok) throw new Error(`HTTP ${resp.status} ${resp.statusText}`);
	const text = await resp.text();
	if (!text) throw new Error("空响应");
	return JSON.parse(text);
}

// ---------- 三个适配器（字段名经 2026-08 实测确认） ----------

const QUOTA_ADAPTERS: Record<string, QuotaAdapter> = {
	// 实测返回：data.limits[] 里只有 type==="TOKENS_LIMIT" 的条目才是 LLM 额度窗口
	// （5h 窗口；带周限制的新套餐会多一条周窗口，按 nextResetTime 升序区分）；
	// type==="TIME_LIMIT" 是 MCP 工具用量（percentage=100 也只是 MCP 见底），与 LLM 派发无关，一律忽略。
	// 老套餐（lite 等）无周限制，仅一条 TOKENS_LIMIT。
	"zai-coding-cn": async (apiKey, signal) => {
		const data = (await fetchJson(
			"https://open.bigmodel.cn/api/monitor/usage/quota/limit",
			{
				Authorization: apiKey, // GLM 端点不加 Bearer
				"User-Agent": BROWSER_UA, // 防反爬
				Accept: "application/json",
			},
			signal,
		)) as { data?: { limits?: unknown[] } };
		const limits = data?.data?.limits;
		if (!Array.isArray(limits)) throw new Error("GLM 响应缺少 data.limits");

		// percentage 实测有数字也有字符串，统一 Number 转换；nextResetTime 缺失（窗口未激活）排最后
		const tokens = (limits as Array<Record<string, unknown>>)
			.filter((l) => l && l.type === "TOKENS_LIMIT" && Number.isFinite(Number(l.percentage)))
			.map((l) => ({
				pct: Number(l.percentage),
				reset: typeof l.nextResetTime === "number" ? l.nextResetTime : undefined,
			}))
			.sort((a, b) => (a.reset ?? Infinity) - (b.reset ?? Infinity));
		if (tokens.length === 0) throw new Error("GLM 响应无 TOKENS_LIMIT 条目");

		const short = tokens[0]; // 最近重置 → 5h 窗口
		const weekly = tokens[1]; // 较远重置 → 每周（老套餐无此条）
		return judgeWindows(
			{
				label: "5h窗口",
				remainPct: 100 - short.pct,
				resetTime: short.reset,
			},
			weekly
				? {
						label: "每周",
						remainPct: 100 - weekly.pct,
						resetTime: weekly.reset,
					}
				: undefined,
		);
	},

	// 实测返回：顶层 usage = 周额度（limit/used/remaining/resetTime，字符串数字，resetTime 为 ISO 字符串）；
	// limits[] 为细分窗口，其中 window.duration === 300（300 分钟 = 5h）的 detail 是 5h 窗口。
	"kimi-coding": async (apiKey, signal) => {
		const data = (await fetchJson(
			"https://api.kimi.com/coding/v1/usages",
			{
				Authorization: `Bearer ${apiKey}`,
				"User-Agent": BROWSER_UA,
				Accept: "application/json",
			},
			signal,
		)) as { usage?: unknown; limits?: unknown[] };

		// 周额度（顶层 usage）
		const weekly = parseCountBucket((data?.usage as Record<string, unknown>) ?? undefined, "每周");
		// 5h 窗口（limits[] 里 window.duration=300 的条目，找不到则取第一条）
		const limits = Array.isArray(data?.limits) ? data.limits : [];
		let shortEntry: Record<string, unknown> | undefined;
		for (const l of limits as Array<Record<string, unknown>>) {
			const win = (l?.window as Record<string, unknown>) ?? {};
			if (win?.duration === 300) {
				shortEntry = l;
				break;
			}
		}
		if (!shortEntry) shortEntry = limits[0] as Record<string, unknown> | undefined;
		const short = parseCountBucket(shortEntry?.detail as Record<string, unknown>, "5h窗口");
		if (!short && !weekly) throw new Error("Kimi 响应缺少额度字段");
		return judgeWindows(short, weekly);
	},

	// 实测返回：is_available: boolean；balance_infos[0].total_balance 为字符串金额（如 "42.63"）
	deepseek: async (apiKey, signal) => {
		const data = (await fetchJson(
			"https://api.deepseek.com/user/balance",
			{
				Authorization: `Bearer ${apiKey}`,
				"User-Agent": BROWSER_UA,
				Accept: "application/json",
			},
			signal,
		)) as { is_available?: boolean; balance_infos?: unknown[] };

		if (data?.is_available === false) {
			return { kind: "exhausted", summary: "账户不可用（is_available=false）" };
		}
		const infos = Array.isArray(data?.balance_infos) ? data.balance_infos : [];
		const total = Number((infos[0] as Record<string, unknown>)?.total_balance);
		if (!Number.isFinite(total)) throw new Error("DeepSeek 响应缺少 total_balance");
		const summary = `余额 ¥${total.toFixed(2)}（阈值 ¥${MIN_BALANCE}）`;
		if (total < MIN_BALANCE) return { kind: "exhausted", summary };
		return { kind: "ok", summary };
	},
};

/** Kimi 的用量桶解析：limit/used/remaining 是字符串数字 */
function parseCountBucket(
	detail: Record<string, unknown> | undefined,
	label: string,
): WindowInfo | undefined {
	if (!detail) return undefined;
	const limit = Number(detail.limit);
	const remaining = Number(detail.remaining);
	if (!Number.isFinite(limit) || limit <= 0 || !Number.isFinite(remaining)) return undefined;
	return {
		label,
		remainPct: (remaining / limit) * 100,
		resetTime: typeof detail.resetTime === "string" ? detail.resetTime : undefined,
	};
}

// ---------- 统一判定（D3） ----------

/**
 * 双窗口判定：任一窗口剩余 < 阈值即介入；双见底 = exhausted，单见底 = low。
 * 重置提示优先指向真正见底的窗口（等它重置才有意义）。
 */
function judgeWindows(short: WindowInfo | undefined, weekly: WindowInfo | undefined): QuotaStatus {
	const shortBelow = short ? short.remainPct < WINDOW_PCT : false;
	const weeklyBelow = weekly ? weekly.remainPct < WINDOW_PCT : false;

	const parts: string[] = [];
	if (short) parts.push(`${short.label}剩 ${fmtPct(short.remainPct)}%`);
	if (weekly) parts.push(`${weekly.label}剩 ${fmtPct(weekly.remainPct)}%`);
	const summary = parts.join("，");

	let resetHint: string | undefined;
	if (shortBelow && short?.resetTime != null) {
		resetHint = formatReset(short.label, short.resetTime);
	} else if (!shortBelow && weeklyBelow && weekly?.resetTime != null) {
		resetHint = formatReset(weekly.label, weekly.resetTime);
	} else if (short?.resetTime != null) {
		resetHint = formatReset(short.label, short.resetTime);
	}

	if (shortBelow && weeklyBelow) return { kind: "exhausted", summary, resetHint };
	if (shortBelow || weeklyBelow) return { kind: "low", summary, resetHint };
	return { kind: "ok", summary, resetHint };
}

// ---------- 阻断 reason（中文，含 provider/剩余/重置/建议） ----------

function buildBlockReason(
	provider: string,
	status: QuotaStatus,
	extra: { fuzzyRisk: boolean; defaultProvider?: string },
): string {
	const lines = [
		`${displayName(provider)} 额度不足，本次 Agent 派发已阻断。`,
		`剩余情况：${status.summary}（阈值：窗口剩余 ${WINDOW_PCT}%，DeepSeek 余额 ¥${MIN_BALANCE}）`,
	];
	if (status.resetHint) lines.push(`重置时间：${status.resetHint}`);
	lines.push(
		status.kind === "exhausted"
			? "建议：换用其他有额度的 provider 重试（如 deepseek/deepseek-v4-flash），或等待额度重置后再派发。"
			: "建议：等待额度窗口重置后重试，或换用其他有额度的 provider。",
	);
	if (extra.fuzzyRisk) {
		lines.push(
			`注意：model 参数为裸模型名，已按字母序解析到 ${displayName(provider)}` +
				`（会话默认 ${displayName(extra.defaultProvider ?? "无")}），fuzzy 写法可能落到非预期供应商，建议显式写 provider/modelId 全称。`,
		);
	}
	return lines.join("\n");
}

// ---------- 小工具 ----------

/** policy.decision 旁路记账（change: harden-harness-policy-and-spill D2/D3）：
 *  无 cwd/sessionId 上下文不记；记账异常忽略，绝不影响裁决。target 仅 provider 短摘要 */
function auditPolicy(ctx: GateCtx, decision: PolicyDecisionInput): void {
	try {
		const sessionId = ctx.sessionManager?.getSessionId?.();
		if (!ctx.cwd || !sessionId) return;
		logPolicyDecision(ctx.cwd, { sessionId, ...decision }); // change=undefined → 自动检测活跃 change
	} catch (err) {
		console.warn(`[quota-gate] policy.decision 记账异常（旁路忽略）：${String(err)}`);
	}
}

/** provider 展示名 */
const DISPLAY_NAMES: Record<string, string> = {
	"zai-coding-cn": "智谱 GLM（zai-coding-cn）",
	"kimi-coding": "Kimi（kimi-coding）",
	deepseek: "DeepSeek（deepseek）",
};
function displayName(provider: string): string {
	return DISPLAY_NAMES[provider] ?? provider;
}

/** 环境变量数值解析：非法值回退默认 */
function envNum(name: string, def: number): number {
	const v = Number(process.env[name]);
	return Number.isFinite(v) && v > 0 ? v : def;
}

/** 百分比展示：<10 保留一位小数，避免 0.4% 显示成 0% 误读为见底 */
function fmtPct(x: number): string {
	const v = Math.max(0, x);
	return v < 10 ? v.toFixed(1) : String(Math.round(v));
}

/** 重置时间人话化：近 24h 给相对分钟数，跨天给日期 */
function formatReset(label: string, t: string | number): string {
	const d = new Date(t);
	if (Number.isNaN(d.getTime())) return `${label}重置时间未知`;
	const diff = d.getTime() - Date.now();
	const abs = Math.abs(diff);
	const time = d.toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit", hour12: false });
	const date = d.toLocaleDateString("zh-CN", { month: "2-digit", day: "2-digit" });
	if (abs < 24 * 3600 * 1000) {
		const when =
			diff >= 0 ? `约 ${Math.max(1, Math.round(abs / 60000))} 分钟后重置` : "已过重置时间";
		return `${label} ${date} ${time} ${when}`;
	}
	return `${label} ${date} ${time} 重置`;
}
