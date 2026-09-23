package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"syntopica-backend/internal/datasources"
)

const comtradeBaseURL = "https://comtradeapi.un.org/data/v1"

// Comtrade fetches UN Comtrade HS commodity trade data. Requires the free
// subscription key (see docs/configuration「研究数据源」; obtained via
// comtradedeveloper.un.org → Products → Free APIs).
type Comtrade struct {
	fetch   *datasources.Fetcher
	cache   *datasources.TTLCache
	baseURL string
	keyFn   func() string
}

// NewComtrade builds the fetcher. keyFn is resolved on every Fetch (dynamic:
// UI-configured key takes effect without restart); returning "" leaves the
// source usable only
// for the explicit missing-key error path (spec「key 缺失时显式报配置缺失」).
func NewComtrade(f *datasources.Fetcher, c *datasources.TTLCache, keyFn func() string) *Comtrade {
	return &Comtrade{fetch: f, cache: c, baseURL: comtradeBaseURL, keyFn: keyFn}
}

// ComtradeParams is one trade query. Partner==nil returns the World aggregate
// row plus all partner rows (partnerCode=0 marks the World aggregate);
// Partner=&code narrows to one partner.
type ComtradeParams struct {
	FreqCode string // "A" | "M"
	Reporter int    // M49 reporter code, e.g. 156 CN / 392 JP / 410 KR
	Partner  *int   // optional M49 partner; nil = all
	CmdCode  string // HS code, e.g. "2709" crude oil
	FlowCode string // "M" imports | "X" exports
	Period   string // "YYYY" (annual) | "YYYYMM" (monthly)
}

// Fetch executes one query. Units follow the field-calibrated contract:
// netWgt/qty are kg (qtyUnitCode 8), primaryValue is USD (docs/research
// explore-findings 2026-09-19). Monthly data lags ~4 months; empty data is a
// legitimate no_data, never an error.
func (c *Comtrade) Fetch(ctx context.Context, p ComtradeParams) (map[string]any, error) {
	apiKey := strings.TrimSpace(c.keyFn())
	if apiKey == "" {
		return nil, datasources.UnavailableDetail("un_comtrade",
			"缺少 UN Comtrade 订阅 key", "请在环境变量 COMTRADE_API_KEY（或 config.yaml comtrade.api_key）配置后重启；key 于 comtradedeveloper.un.org 订阅 Free APIs 产品获取")
	}
	if p.FreqCode != "A" && p.FreqCode != "M" {
		return nil, datasources.InvalidArg("un_comtrade", `freqCode 必须是 "A" 或 "M"，收到 `+fmt.Sprint(p.FreqCode))
	}
	if p.FlowCode != "M" && p.FlowCode != "X" {
		return nil, datasources.InvalidArg("un_comtrade", `flowCode 必须是 "M"（进口）或 "X"（出口）`)
	}
	if p.Reporter < 1 || p.Reporter > 999 {
		return nil, datasources.InvalidArg("un_comtrade",
			fmt.Sprintf("reporter 必须是 M49 国家码（如 156 中国/392 日本/410 韩国/682 沙特），收到 %d", p.Reporter))
	}
	if p.Partner != nil && (*p.Partner < 0 || *p.Partner > 999) {
		return nil, datasources.InvalidArg("un_comtrade",
			fmt.Sprintf("partner 必须是 M49 国家码或 0（World），收到 %d", *p.Partner))
	}
	if !isDigits(p.CmdCode) || len(p.CmdCode) < 2 || len(p.CmdCode) > 6 {
		return nil, datasources.InvalidArg("un_comtrade",
			fmt.Sprintf("cmdCode 必须是 2-6 位 HS 编码（如 2709 原油），收到 %q", p.CmdCode))
	}
	periodOK := (p.FreqCode == "A" && len(p.Period) == 4) || (p.FreqCode == "M" && len(p.Period) == 6)
	if !periodOK || !isDigits(p.Period) {
		return nil, datasources.InvalidArg("un_comtrade",
			fmt.Sprintf("period 必须与 freqCode 匹配（A=YYYY，M=YYYYMM），收到 %q", p.Period))
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s/get/C/%s/HS?reporterCode=%d&period=%s&cmdCode=%s&flowCode=%s",
		c.baseURL, p.FreqCode, p.Reporter, p.Period, p.CmdCode, p.FlowCode)
	if p.Partner != nil {
		fmt.Fprintf(&sb, "&partnerCode=%d", *p.Partner)
	}
	url := sb.String()

	key := fmt.Sprintf("comtrade:%s", url)
	var doc datasources.FetchedDoc
	if cached, ok := c.cache.Get(key); ok {
		doc = cached
	} else {
		fetched, err := c.fetch.Get(ctx, url, map[string]string{
			"Ocp-Apim-Subscription-Key": apiKey,
			"User-Agent":                "syntopica-datasources/0.1 (read-only public data)",
		})
		if err != nil {
			// 429 rate limit surfaces as SOURCE_UNAVAILABLE carrying the
			// upstream status (fetch layer UnavailableStatus).
			return nil, err
		}
		doc = *fetched
		c.cache.Put(key, doc)
	}

	var payload struct {
		Count int             `json:"count"`
		Error json.RawMessage `json:"error"`
		Data  []struct {
			ReporterCode int      `json:"reporterCode"`
			PartnerCode  int      `json:"partnerCode"`
			Period       string   `json:"period"`
			FreqCode     string   `json:"freqCode"`
			FlowCode     string   `json:"flowCode"`
			CmdCode      string   `json:"cmdCode"`
			QtyUnitCode  int      `json:"qtyUnitCode"`
			Qty          *float64 `json:"qty"`
			NetWgt       *float64 `json:"netWgt"`
			PrimaryValue *float64 `json:"primaryValue"`
			IsAggregate  bool     `json:"isAggregate"`
		} `json:"data"`
	}
	if err := json.Unmarshal(doc.Payload, &payload); err != nil {
		c.cache.Evict(key)
		return nil, datasources.SchemaChangedDetail("un_comtrade", "Comtrade 响应解析失败", err.Error())
	}
	if len(payload.Error) > 0 && string(payload.Error) != `""` {
		return nil, datasources.UnavailableDetail("un_comtrade",
			"Comtrade 源端返回错误", strings.TrimSpace(string(payload.Error)))
	}

	type obs = map[string]any
	observations := make([]obs, 0, len(payload.Data))
	for _, d := range payload.Data {
		o := obs{
			"reporter_code": d.ReporterCode,
			"partner_code":  d.PartnerCode,
			"period":        d.Period,
			"freq_code":     d.FreqCode,
			"flow_code":     d.FlowCode,
			"cmd_code":      d.CmdCode,
			"qty_unit_code": d.QtyUnitCode,
			"qty_kg":        d.Qty,
			"net_wgt_kg":    d.NetWgt,
			"value_usd":     d.PrimaryValue,
			"is_aggregate":  d.IsAggregate,
		}
		observations = append(observations, o)
	}

	return map[string]any{
		"source":        "UN Comtrade Plus API (HS commodity trade)",
		"url":           doc.URL,
		"retrieved_at":  doc.RetrievedAt.Format(time.RFC3339),
		"source_sha256": doc.SHA256,
		"from_cache":    doc.FromCache,
		"count":         payload.Count,
		"no_data":       len(observations) == 0,
		"observations":  observations,
		"notes": []string{
			"netWgt/qty 单位为千克（qtyUnitCode=8），primaryValue 单位为美元。",
			"partner_code=0 且 is_aggregate=true 为 World 合计行；分伙伴行按 partner_code 区分。",
			"月度数据滞后约 4 个月以上（中国更长）；count=0 表示该期未发布，是合法无数据而非错误。",
			"免费档限速 500 次/天（2026-09 官方口径）；本服务带 TTL 缓存，缓存命中不消耗配额。",
		},
	}, nil
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
