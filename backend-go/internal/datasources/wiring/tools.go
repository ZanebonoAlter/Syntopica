package wiring

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"syntopica-backend/internal/dataenrichment/service"
	"syntopica-backend/internal/datasources"
	"syntopica-backend/internal/datasources/sources"
)

// ResearchTools adapts the four source fetchers into agent-loop Tool shape
// (dataenrichment/service.Tool). Execute output is a JSON string; typed
// errors serialize as {"error_code","message","detail"} so the agent loop
// sees structured failures, matching the unified error taxonomy.
func ResearchTools(eia EIAFetcher, jodi JODIFetcher, wdi WDIFetcher, comtrade ComtradeFetcher) []*service.Tool {
	tools := []*service.Tool{
		{
			Name:        "eia_wpsr_table1",
			Description: "美国 EIA 周度石油状况报告 Table 1（仅美国，周度）。取原油库存（stocks：商业不含SPR/SPR/含SPR总量，单位 MMbbl）或供需（supply：国内产量/原油进口/原油出口，单位 Mb/d）；缺省返回当周+上周成对观测值与周结束日期；可选 weeks 显式取最近 N 周序列（扁平逐周观测）。不得表述为全球数据；结构漂移会显式报 SCHEMA_CHANGED。",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"section": map[string]any{"type": "string", "enum": []string{"stocks", "supply"}},
					"weeks": map[string]any{
						"type":        "integer",
						"minimum":     1,
						"maximum":     sources.EIAMaxWeeks,
						"description": "可选：显式传入时返回最近 N 周扁平序列（1~12，含当周倒推，来自官方归档版次）；缺省保持当周+上周成对返回",
					},
				},
				"required": []string{"section"},
			},
			Execute: func(ctx context.Context, args map[string]any) (string, error) {
				section, _ := args["section"].(string)
				weeks, hasWeeks, err := optionalIntArg(args, "weeks", 1, sources.EIAMaxWeeks, "eia_wpsr")
				if err != nil {
					return marshalResult(nil, err)
				}
				var window []int
				if hasWeeks {
					window = []int{weeks}
				}
				res, err := eia.Fetch(ctx, section, window...)
				return marshalResult(res, err)
			},
		},
		{
			Name:        "jodi_oil_primary",
			Description: "JODI 全球原油月度数据库（96 经济体含中日韩美；product 固定 CRUDEOIL）。flow: production/imports/exports/closing_stocks；单位 KBD（流量）/KBBL（库存）不换算；month 缺省取最新期（即使 null 不回退）；可选 years 显式取最近 N 个日历年全部月份（与 month 互斥）；TIME_PERIOD 是数据期（滞后约 1.5-2 个月）。",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"geo":   map[string]any{"type": "string", "description": "两个大写字母（ISO 风格），如 US/CN/JP/KR/SA"},
					"flow":  map[string]any{"type": "string", "enum": []string{"production", "imports", "exports", "closing_stocks"}},
					"unit":  map[string]any{"type": "string", "enum": []string{"KBD", "KBBL"}, "description": "缺省按 flow 取默认（流量 KBD、库存 KBBL）"},
					"month": map[string]any{"type": "string", "description": "YYYY-MM；缺省取该文件最新期"},
					"years": map[string]any{
						"type":        "integer",
						"minimum":     1,
						"maximum":     sources.JODIMaxYears,
						"description": "可选：显式传入时返回最近 N 个日历年全部可得月份（1~5）；与 month 互斥；两者都不传取最新期",
					},
				},
				"required": []string{"geo", "flow"},
			},
			Execute: func(ctx context.Context, args map[string]any) (string, error) {
				geo, _ := args["geo"].(string)
				flow, _ := args["flow"].(string)
				unit, _ := args["unit"].(string)
				month, _ := args["month"].(string)
				years, hasYears, err := optionalIntArg(args, "years", 1, sources.JODIMaxYears, "jodi_oil_primary")
				if err != nil {
					return marshalResult(nil, err)
				}
				var window []int
				if hasYears {
					window = []int{years}
				}
				res, err := jodi.Fetch(ctx, geo, flow, unit, month, window...)
				return marshalResult(res, err)
			},
		},
		{
			Name:        "wb_wdi",
			Description: "世界银行 WDI 宏观年度指标（匿名）。指标点分大写码（如 NE.EXP.GNFS.ZS 出口占GDP、TM.VAL.FUEL.ZS.UN 燃料进口占商品进口%），国家 ISO3（CHN/JPN/KOR...），年份范围 1960..当前。返回各国各年观测值（null 表示无观测）与源端 lastupdated。",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"indicator": map[string]any{"type": "string", "description": "点分大写指标码，如 NE.EXP.GNFS.ZS"},
					"countries": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "ISO3 列表，如 [\"CHN\",\"JPN\",\"KOR\"]"},
					"from_year": map[string]any{"type": "integer"},
					"to_year":   map[string]any{"type": "integer"},
				},
				"required": []string{"indicator", "countries", "from_year", "to_year"},
			},
			Execute: func(ctx context.Context, args map[string]any) (string, error) {
				indicator, _ := args["indicator"].(string)
				var countries []string
				if raw, ok := args["countries"].([]any); ok {
					for _, c := range raw {
						if s, ok := c.(string); ok {
							countries = append(countries, s)
						}
					}
				}
				res, err := wdi.Fetch(ctx, indicator, countries, int(argNumber(args, "from_year")), int(argNumber(args, "to_year")))
				return marshalResult(res, err)
			},
		},
		{
			Name:        "un_comtrade_trade",
			Description: "UN Comtrade HS 商品贸易双边数据（全球 reporter×partner；需订阅 key）。netWgt/qty 单位 kg、value_usd 美元；partner_code=0 且 is_aggregate=true 为 World 合计行。月度滞后约 4 个月+（时效敏感月度流量请用 jodi_oil_primary）；M49 码：中国156/日本392/韩国410/沙特682；原油 HS2709。",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"freq_code": map[string]any{"type": "string", "enum": []string{"A", "M"}},
					"reporter":  map[string]any{"type": "integer", "description": "M49 reporter 码"},
					"partner":   map[string]any{"type": "integer", "description": "M49 partner 码；缺省返回全部（含 World 合计行）"},
					"cmd_code":  map[string]any{"type": "string", "description": "HS 编码，如 2709"},
					"flow_code": map[string]any{"type": "string", "enum": []string{"M", "X"}, "description": "M 进口 / X 出口"},
					"period":    map[string]any{"type": "string", "description": "A→YYYY；M→YYYYMM"},
				},
				"required": []string{"freq_code", "reporter", "cmd_code", "flow_code", "period"},
			},
			Execute: func(ctx context.Context, args map[string]any) (string, error) {
				p := sources.ComtradeParams{
					FreqCode: str(args, "freq_code"),
					Reporter: int(argNumber(args, "reporter")),
					CmdCode:  str(args, "cmd_code"),
					FlowCode: str(args, "flow_code"),
					Period:   str(args, "period"),
				}
				if v, ok := args["partner"]; ok && v != nil {
					code := int(argNumber(args, "partner"))
					p.Partner = &code
				}
				res, err := comtrade.Fetch(ctx, p)
				return marshalResult(res, err)
			},
		},
	}
	appendCatalogMetadata(tools)
	return tools
}

// toolCatalogCodes aligns the fixed ResearchTools construction order
// (eia/jodi/wdi/comtrade, pinned by TestResearchToolsToolSurfaceUnchanged) to
// catalog codes, so descriptions are enriched from datasources.Definition —
// the single metadata source, never a hand-copied second set.
var toolCatalogCodes = [...]string{"eia_wpsr", "jodi_oil_primary", "wb_wdi", "un_comtrade"}

// researchHardLimitNote states what none of the four sources can answer
// (design §10.1): the first real signal report burned its whole budget
// re-trying diesel/crack-spread/freight questions no source covers.
const researchHardLimitNote = "\n【硬限制】四个数据源均无价格序列、裂解价差、运价、政策文本类数据；超出覆盖的研究问题不要反复换参重试，直接如实声明无覆盖。"

// comtradeMeasuredCoverageNote declares the subscription-tier reality measured
// 2026-09-23 (CN returns rows; US/JP count=0 for any period): count=0 means
// that period is not published for the reporter, NOT a failed call.
const comtradeMeasuredCoverageNote = "\n【实测覆盖限制】订阅档仅部分 reporter 有数据（实测中国有数，美国/日本任意期 count=0）；count=0 表示该期未发布，不是调用失败，不要据此判定源故障或换参空转。"

// appendCatalogMetadata enriches each tool description with its catalog
// metadata (coverage/frequency/typical_lag/unit_policy) plus the ensemble
// hard limits and the Comtrade measured-coverage caveat, so research
// questions land inside what the sources can actually answer (design §10.1
// 研究阶段 toolsDesc 附 catalog 覆盖与滞后).
func appendCatalogMetadata(tools []*service.Tool) {
	if len(tools) != len(toolCatalogCodes) {
		return
	}
	for i, tool := range tools {
		d, ok := datasources.Definition(toolCatalogCodes[i])
		if !ok {
			continue
		}
		tool.Description += "\n【目录元数据】覆盖：" + d.Coverage +
			"｜频率：" + d.Frequency +
			"｜典型滞后：" + d.TypicalLag +
			"｜单位纪律：" + d.UnitPolicy
		if toolCatalogCodes[i] == "un_comtrade" {
			tool.Description += comtradeMeasuredCoverageNote
		}
		tool.Description += researchHardLimitNote
	}
}

// researchCatalogCodeByTool aligns the fixed ResearchTools names to catalog
// codes (the same order as toolCatalogCodes; pinned by
// TestResearchToolsToolSurfaceUnchanged so the two cannot drift silently).
var researchCatalogCodeByTool = map[string]string{
	"eia_wpsr_table1":   "eia_wpsr",
	"jodi_oil_primary":  "jodi_oil_primary",
	"wb_wdi":            "wb_wdi",
	"un_comtrade_trade": "un_comtrade",
}

// UnavailableNoticeProbe returns the LIVE availability probe injected into the
// research service (design §10.4：可用性判断一律走 live resolver，不靠启动
// 快照——UI 保存 key 后研究 prompt 即时生效，不必重启). Empty string =
// available; non-empty = the notice the research clone appends to that tool's
// description. The judgment mirrors datasources.StatusFor (RequiresKey + empty
// key via the same resolver), so catalog status and prompt wording cannot
// drift apart. The anchor is shared with the research clone's strip step
// (service.SignalUnavailableNoticeAnchor) so a stale startup-baked notice is
// replaced, not duplicated.
func UnavailableNoticeProbe(resolve datasources.KeyResolver) func(toolName string) string {
	return func(toolName string) string {
		if resolve == nil {
			return ""
		}
		code, ok := researchCatalogCodeByTool[toolName]
		if !ok {
			return ""
		}
		d, ok := datasources.Definition(code)
		if !ok || !d.RequiresKey {
			return ""
		}
		if strings.TrimSpace(resolve(d.ConfigKeyName)) == "" {
			return service.SignalUnavailableNoticeAnchor + d.ConfigKeyName + "】调用会返回配置缺失错误帧；不要调用本工具，相关研究问题改由其他源回答或如实声明无覆盖。"
		}
		return ""
	}
}

// Narrow fetcher interfaces so tools.go depends on behavior, not concrete
// structs (and tests can stub). The trailing variadic carries the OPTIONAL
// explicit history window (EIA weeks 1..12 / JODI years 1..5); existing
// callers that omit it keep the old behavior byte-identically.
type (
	EIAFetcher interface {
		Fetch(ctx context.Context, section string, weeks ...int) (map[string]any, error)
	}
	JODIFetcher interface {
		Fetch(ctx context.Context, geo, flow, unit, month string, years ...int) (map[string]any, error)
	}
	WDIFetcher interface {
		Fetch(ctx context.Context, indicator string, countries []string, from, to int) (map[string]any, error)
	}
	ComtradeFetcher interface {
		Fetch(ctx context.Context, p sources.ComtradeParams) (map[string]any, error)
	}
)

// optionalIntArg extracts an optional integer tool argument. Absent/nil →
// (0,false,nil). Present but not an integral JSON number in [min,max] →
// INVALID_ARGUMENT naming the source (fractional / string / bool / out-of-range
// values are rejected BEFORE any network call — the source re-validates).

func str(args map[string]any, key string) string {
	switch v := args[key].(type) {
	case string:
		return v
	case float64:
		return fmt.Sprintf("%v", v)
	default:
		return ""
	}
}

func optionalIntArg(args map[string]any, key string, min, max int, source string) (int, bool, error) {
	v, present := args[key]
	if !present || v == nil {
		return 0, false, nil
	}
	num, ok := v.(float64) // JSON numbers decode as float64
	if !ok || num != float64(int(num)) {
		return 0, false, datasources.InvalidArg(source,
			fmt.Sprintf("%s 必须是 %d~%d 的整数，收到 %v", key, min, max, v))
	}
	n := int(num)
	if n < min || n > max {
		return 0, false, datasources.InvalidArg(source,
			fmt.Sprintf("%s 必须是 %d~%d 的整数，收到 %d", key, min, max, n))
	}
	return n, true, nil
}

func argNumber(args map[string]any, key string) float64 {
	switch v := args[key].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case string:
		var f float64
		_, _ = fmt.Sscanf(v, "%g", &f)
		return f
	default:
		return 0
	}
}

// marshalResult serializes a fetch result; typed errors become structured
// JSON (error_code/message/detail) instead of opaque strings.
func marshalResult(res map[string]any, err error) (string, error) {
	if err != nil {
		payload := map[string]any{"error": err.Error()}
		if se, ok := datasources.AsSourceError(err); ok {
			payload = map[string]any{
				"error_code": string(se.Kind),
				"message":    se.Message,
				"detail":     se.Detail,
			}
		}
		b, _ := json.Marshal(payload)
		return string(b), nil // tool ran; the failure is part of its output
	}
	b, err := json.Marshal(res)
	if err != nil {
		return "", fmt.Errorf("marshal result: %w", err)
	}
	return string(b), nil
}

var _ = strings.TrimSpace
var _ = time.Now
