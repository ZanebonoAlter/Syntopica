package service

import (
	"fmt"
	"math/big"
	"strings"
	"time"
)

// ── 受限计算解释器（board-signal-reports design §5，tasks 3.5）────────────────
//
// signal_research loop 的 calculate 动作落到这里。这不是通用表达式引擎：
//   - 仅三个白名单算子 difference / percent_change / mean；
//   - 输入只能是本次研究已取得且通过 cutoff 的【原始观测】（按 observation_ref
//     引用），不支持表达式链 / forward / 循环引用，计算结果不能作输入（CA-7）；
//   - 兼容校验保守起步：difference 允许「同一系列不同时点」或「同源同期间的
//     批准兼容流量对（imports−exports）」；percent_change / mean 要求同源同
//     系列同单位；任何跨源换算、混单位、库存减流量一律拒绝（CA-6）；
//   - 值由代码用规范十进制计算，half-up（half away from zero）最多 4 位小数、
//     去末尾 0；模型永远不能提交 value（value 字段在 policy 层即拦）。
//   - 任一输入观测缺失值 → status=missing + 原因，绝不填 0（CA-5）。
//
// 纯逻辑、无 DB、无 LLM：可独立表驱动测试（CA-1..7）。

// 白名单算子。
const (
	SignalCalcOpDifference    = "difference"
	SignalCalcOpPercentChange = "percent_change"
	SignalCalcOpMean          = "mean"
)

// 计算结果状态：ok=成功；missing=请求合法但输入缺值（缺失结果+原因，不填 0）；
// rejected=请求合法但违反兼容白名单（拒绝原因落账本）。请求形状非法（未知
// op/缺 question/带 value/悬空重复引用的形状问题）在 policy 层即 blocked，
// 不会到达本解释器。
const (
	SignalCalcStatusOK       = "ok"
	SignalCalcStatusMissing  = "missing"
	SignalCalcStatusRejected = "rejected"
)

// SignalCalcPrecision 是派生值的固定小数位上限（half-up 后去末尾 0）。
const SignalCalcPrecision = 4

// SignalCalcObservation is one cutoff-passed raw observation as seen by the
// calculator. Ref is the observation_ref the model cites ("c1:o2"); SeriesID
// is the source-normalized series identity; Flow carries the source flow
// dimension (approved traffic pairs are checked against it).
type SignalCalcObservation struct {
	Ref      string   // 观测引用，如 "c1:o2"
	CallID   string   // 所属取数调用 call_id
	Tool     string   // 来源工具名（=源）
	SeriesID string   // 源内系列身份（同系列判定）
	Unit     string   // 原生单位（不跨源换算）
	Period   string   // 数据期（源格式原样）
	Flow     string   // 流量维度（批准流量对判定）；无流量维度的源为空
	Value    *float64 // nil = 缺失观测（缺失标记/null，绝不转 0）
}

// SignalCalculationRequest 镜像模型提交的 calculate 动作载荷。
type SignalCalculationRequest struct {
	Op       string
	Inputs   []string
	Question string
}

// SignalCalculationResult 是 appendix.calculations[] 的一行（design §5：
// {op, inputs, expression, value, unit, precision, status, computed_at}）。
// Value 是规范十进制字符串（避免 JSON 浮点表示漂移）；原始观测不经派生舍入，
// 按源精度展示（appendix 观测行保留源值）。
type SignalCalculationResult struct {
	CalcID     string   `json:"calc_id"`
	Op         string   `json:"op"`
	Inputs     []string `json:"inputs"`
	Expression string   `json:"expression"`
	Value      string   `json:"value,omitempty"`
	Unit       string   `json:"unit,omitempty"`
	Precision  int      `json:"precision"`
	Status     string   `json:"status"`
	Reason     string   `json:"reason,omitempty"`
	ComputedAt string   `json:"computed_at"`
}

// runSignalCalculation executes one whitelisted calculation against the run's
// observation index. calcID 由代码分配（k1、k2…）；now 只喂 computed_at。
// 返回值永远非 nil——missing/rejected 也是落账本的结果。
func runSignalCalculation(index map[string]*SignalCalcObservation, req SignalCalculationRequest, calcID string, now time.Time) *SignalCalculationResult {
	res := &SignalCalculationResult{
		CalcID:     calcID,
		Op:         req.Op,
		Inputs:     append([]string{}, req.Inputs...),
		Precision:  SignalCalcPrecision,
		Status:     SignalCalcStatusOK,
		ComputedAt: now.UTC().Format(time.RFC3339),
	}
	reject := func(reason string) *SignalCalculationResult {
		res.Status = SignalCalcStatusRejected
		res.Reason = reason
		return res
	}
	missing := func(reason string) *SignalCalculationResult {
		res.Status = SignalCalcStatusMissing
		res.Reason = reason
		return res
	}
	resolve := func(ref string) (*SignalCalcObservation, error) {
		obs, ok := index[ref]
		if !ok || obs == nil {
			return nil, fmt.Errorf("引用 %s 不在本次已取得的观测中（只能引用已通过 cutoff 的原始观测）", ref)
		}
		return obs, nil
	}

	switch req.Op {
	case SignalCalcOpDifference:
		if len(req.Inputs) != 2 {
			return reject("difference 恰好需要两个输入观测（a - b）")
		}
		a, err := resolve(req.Inputs[0])
		if err != nil {
			return reject(err.Error())
		}
		b, err := resolve(req.Inputs[1])
		if err != nil {
			return reject(err.Error())
		}
		res.Expression = fmt.Sprintf("%s - %s", a.Ref, b.Ref)
		if a.Tool != b.Tool {
			return reject(fmt.Sprintf("跨源相减被拒绝：%s（%s）与 %s（%s）来自不同数据源，单位与口径不可换算", a.Ref, a.Tool, b.Ref, b.Tool))
		}
		if a.Unit != b.Unit {
			return reject(fmt.Sprintf("混单位相减被拒绝：%s（%s）与 %s（%s）原生单位不同", a.Ref, a.Unit, b.Ref, b.Unit))
		}
		switch {
		case a.SeriesID == b.SeriesID && a.Period == b.Period:
			return reject(fmt.Sprintf("%s 与 %s 是同一系列同一期间，差值无意义；同一系列请取不同时点", a.Ref, b.Ref))
		case a.SeriesID == b.SeriesID:
			// 同一系列不同时点：批准。
		case a.Period == b.Period && approvedSignalFlowPair(a.Flow, b.Flow):
			// 同源同期间的批准兼容流量对（imports−exports 净流量）：批准。
		case a.Period == b.Period:
			return reject(fmt.Sprintf("%s（%s）与 %s（%s）同期间但不是批准的兼容流量对；同期间只允许 imports−exports 之差", a.Ref, a.Flow, b.Ref, b.Flow))
		default:
			return reject(fmt.Sprintf("%s 与 %s 既非同一系列的不同时点，也非同期间的批准流量对", a.Ref, b.Ref))
		}
		if a.Value == nil || b.Value == nil {
			return missing(fmt.Sprintf("输入缺失观测值：%s（%s）或 %s（%s）为缺失标记，不填 0", a.Ref, a.Period, b.Ref, b.Period))
		}
		r := new(big.Rat).Sub(bigRatFromFloat(*a.Value), bigRatFromFloat(*b.Value))
		res.Value = formatSignalDecimal(r)
		res.Unit = a.Unit
		return res

	case SignalCalcOpPercentChange:
		if len(req.Inputs) != 2 {
			return reject("percent_change 恰好需要两个输入观测（current, base）")
		}
		cur, err := resolve(req.Inputs[0])
		if err != nil {
			return reject(err.Error())
		}
		base, err := resolve(req.Inputs[1])
		if err != nil {
			return reject(err.Error())
		}
		res.Expression = fmt.Sprintf("(%s - %s) / %s * 100", cur.Ref, base.Ref, base.Ref)
		if cur.Tool != base.Tool {
			return reject(fmt.Sprintf("跨源百分比被拒绝：%s（%s）与 %s（%s）来自不同数据源", cur.Ref, cur.Tool, base.Ref, base.Tool))
		}
		if cur.SeriesID != base.SeriesID {
			return reject(fmt.Sprintf("percent_change 要求同一系列（同源同指标）： %s 与 %s 系列不同", cur.Ref, base.Ref))
		}
		if cur.Unit != base.Unit {
			return reject(fmt.Sprintf("混单位被拒绝：%s（%s）与 %s（%s）原生单位不同", cur.Ref, cur.Unit, base.Ref, base.Unit))
		}
		if base.Value == nil || cur.Value == nil {
			return missing(fmt.Sprintf("输入缺失观测值：%s（%s）或 %s（%s）为缺失标记，不填 0", cur.Ref, cur.Period, base.Ref, base.Period))
		}
		if *base.Value <= 0 {
			return reject(fmt.Sprintf("基数 %s 的值 %s 非正，百分比无意义，已拒绝（不生成无穷大或误导百分比）", base.Ref, formatSignalDecimal(bigRatFromFloat(*base.Value))))
		}
		curRat := bigRatFromFloat(*cur.Value)
		baseRat := bigRatFromFloat(*base.Value)
		r := new(big.Rat).Sub(curRat, baseRat)
		r.Quo(r, baseRat)
		r.Mul(r, big.NewRat(100, 1))
		res.Value = formatSignalDecimal(r)
		res.Unit = "%"
		return res

	case SignalCalcOpMean:
		if len(req.Inputs) < 2 {
			return reject("mean 需要至少两个输入观测（明确期间集合）")
		}
		obs := make([]*SignalCalcObservation, 0, len(req.Inputs))
		for _, ref := range req.Inputs {
			o, err := resolve(ref)
			if err != nil {
				return reject(err.Error())
			}
			obs = append(obs, o)
		}
		res.Expression = "mean(" + strings.Join(req.Inputs, ", ") + ")"
		first := obs[0]
		for _, o := range obs[1:] {
			if o.Tool != first.Tool {
				return reject(fmt.Sprintf("跨源均值被拒绝：%s（%s）与 %s（%s）来自不同数据源", first.Ref, first.Tool, o.Ref, o.Tool))
			}
			if o.SeriesID != first.SeriesID {
				return reject(fmt.Sprintf("mean 要求同一系列（同源同指标）： %s 与 %s 系列不同", first.Ref, o.Ref))
			}
			if o.Unit != first.Unit {
				return reject(fmt.Sprintf("混单位被拒绝：%s（%s）与 %s（%s）原生单位不同", first.Ref, first.Unit, o.Ref, o.Unit))
			}
		}
		seenPeriods := map[string]bool{}
		for _, o := range obs {
			if seenPeriods[o.Period] {
				return reject(fmt.Sprintf("期间集合含重复期间 %s（%s）；mean 只接受明确的、互异的期间集合", o.Period, o.Ref))
			}
			seenPeriods[o.Period] = true
		}
		var missingRefs []string
		sum := new(big.Rat)
		for _, o := range obs {
			if o.Value == nil {
				missingRefs = append(missingRefs, fmt.Sprintf("%s（%s）", o.Ref, o.Period))
				continue
			}
			sum.Add(sum, bigRatFromFloat(*o.Value))
		}
		if len(missingRefs) > 0 {
			// 缺期不称完整窗口：缺失成员使整个均值缺失，不做「可用值均值」。
			return missing(fmt.Sprintf("mean 窗口缺期：%s 无观测值，不称完整窗口", strings.Join(missingRefs, "、")))
		}
		res.Value = formatSignalDecimal(new(big.Rat).Quo(sum, new(big.Rat).SetInt64(int64(len(obs)))))
		res.Unit = first.Unit
		return res

	default:
		return reject(fmt.Sprintf("未知算子 %q；白名单只有 difference / percent_change / mean", req.Op))
	}
}

// approvedSignalFlowPair reports whether (a,b) is an approved compatible
// traffic pair for same-period differences: imports−exports in either order
// (net trade). Conservative starting whitelist per design §5 — stocks never
// qualify (库存减流量一律拒绝), production has no approved partner.
func approvedSignalFlowPair(a, b string) bool {
	la, lb := strings.ToLower(strings.TrimSpace(a)), strings.ToLower(strings.TrimSpace(b))
	if la == "" || lb == "" || la == lb {
		return false
	}
	return (la == "imports" && lb == "exports") || (la == "exports" && lb == "imports")
}

// bigRatFromFloat converts a float64 observation value to an exact rational
// (SetFloat64 keeps the exact binary value; all arithmetic stays rational
// until the single final rounding).
func bigRatFromFloat(f float64) *big.Rat {
	return new(big.Rat).SetFloat64(f)
}

// formatSignalDecimal renders an exact rational as a canonical decimal string:
// half-up (half away from zero) at SignalCalcPrecision digits, trailing zeros
// trimmed, no scientific notation, no "-0".
func formatSignalDecimal(r *big.Rat) string {
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(SignalCalcPrecision), nil)
	num := new(big.Int).Mul(r.Num(), scale)
	den := r.Denom()
	neg := num.Sign() < 0
	q, rem := new(big.Int).QuoRem(new(big.Int).Abs(num), den, new(big.Int))
	twice := new(big.Int).Lsh(rem, 1)
	if twice.Cmp(den) >= 0 {
		q.Add(q, big.NewInt(1))
	}
	s := formatScaledSignalInt(q, SignalCalcPrecision)
	if neg && q.Sign() != 0 {
		s = "-" + s
	}
	return s
}

// formatScaledSignalInt inserts a decimal point `precision` digits from the
// right and trims trailing zeros / a bare trailing dot.
func formatScaledSignalInt(q *big.Int, precision int) string {
	s := q.String()
	if precision <= 0 {
		return s
	}
	if len(s) <= precision {
		s = strings.Repeat("0", precision-len(s)+1) + s
	}
	out := s[:len(s)-precision] + "." + s[len(s)-precision:]
	out = strings.TrimRight(out, "0")
	return strings.TrimSuffix(out, ".")
}
