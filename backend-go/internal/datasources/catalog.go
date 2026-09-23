package datasources

// DataSourceDefinition is the single source of truth for a data source's
// static catalog attributes (design decision 5): the Go constant feeds the
// data_sources table seed AND derives the tool description. The table stores
// a snapshot plus runtime state; it never diverges because startup re-upserts.
type DataSourceDefinition struct {
	Code          string   `json:"code"`     // eia_wpsr / jodi_oil_primary / wb_wdi / un_comtrade
	Name          string   `json:"name"`     // human-readable name (zh)
	Provider      string   `json:"provider"` // EIA / JODI / World Bank / UN Comtrade
	HomepageURL   string   `json:"homepage_url"`
	Coverage      string   `json:"coverage"`    // e.g. "美国（周度）"
	Topics        []string `json:"topics"`      // crude-oil / trade / macro
	Frequency     string   `json:"frequency"`   // weekly / monthly / annual
	TypicalLag    string   `json:"typical_lag"` // human-readable lag description
	UnitPolicy    string   `json:"unit_policy"` // unit discipline (never cross-convert)
	RequiresKey   bool     `json:"requires_key"`
	ConfigKeyName string   `json:"config_key_name,omitempty"` // env/config name when RequiresKey
	HostAllowlist []string `json:"-"`                         // official HTTPS hosts (fetch layer)
}

// Catalog returns all registered data source definitions.
func Catalog() []DataSourceDefinition {
	return append([]DataSourceDefinition(nil), catalog...)
}

// Definition looks one source up by code.
func Definition(code string) (DataSourceDefinition, bool) {
	for _, d := range catalog {
		if d.Code == code {
			return d, true
		}
	}
	return DataSourceDefinition{}, false
}

var catalog = []DataSourceDefinition{
	{
		Code:        "eia_wpsr",
		Name:        "EIA 周度石油状况报告（WPSR Table 1）",
		Provider:    "U.S. Energy Information Administration",
		HomepageURL: "https://ir.eia.gov/wpsr/table1.csv",
		Coverage:    "美国（仅美国数据，不得表述为全球）",
		Topics:      []string{"crude-oil", "stocks", "supply"},
		Frequency:   "weekly",
		TypicalLag:  "约 3-5 天（周三发布上周五截止数据）",
		UnitPolicy:  "stocks=MMbbl、supply=Mb/d（依 WPSR 官方表定义，不从数值猜测，不跨单位换算）",
		RequiresKey: false,
		// www.eia.gov hosts the archived-edition CSVs the explicit weeks>2
		// window path builds (sources/eia.go eiaArchiveBase). Missing it made
		// every weeks>2 call die in the fetch gate (result id=19 step 15:
		// SOURCE_UNAVAILABLE「目标 host 不在核定白名单内」).
		HostAllowlist: []string{"ir.eia.gov", "www.eia.gov"},
	},
	{
		Code:          "jodi_oil_primary",
		Name:          "JODI Oil Primary（世界原油月度数据库）",
		Provider:      "Joint Organisations Data Initiative",
		HomepageURL:   "https://www.jodidata.org/oil/database/data-downloads.aspx",
		Coverage:      "96 个经济体（含中日韩美），原油产品",
		Topics:        []string{"crude-oil", "production", "trade", "stocks"},
		Frequency:     "monthly",
		TypicalLag:    "约 1.5-2 个月（TIME_PERIOD 为数据期，非发布期）",
		UnitPolicy:    "KBD/KBBL 原始单位保留，不跨单位换算；库存+流量单位组合拒绝",
		RequiresKey:   false,
		HostAllowlist: []string{"www.jodidata.org"},
	},
	{
		Code:          "wb_wdi",
		Name:          "世界银行 WDI（世界发展指标）",
		Provider:      "World Bank",
		HomepageURL:   "https://api.worldbank.org/v2/",
		Coverage:      "全球（中日韩全覆盖），宏观年度指标",
		Topics:        []string{"macro", "trade"},
		Frequency:     "annual",
		TypicalLag:    "年度数据，随源端 lastupdated 更新",
		UnitPolicy:    "指标原生单位（% / 美元等），按指标定义，不换算",
		RequiresKey:   false,
		HostAllowlist: []string{"api.worldbank.org"},
	},
	{
		Code:          "un_comtrade",
		Name:          "UN Comtrade 商品贸易（HS）",
		Provider:      "United Nations Statistics Division",
		HomepageURL:   "https://comtradeapi.un.org/",
		Coverage:      "全球双边贸易（中日韩等全部 reporter，含伙伴国拆分）",
		Topics:        []string{"trade", "crude-oil"},
		Frequency:     "monthly+annual",
		TypicalLag:    "月度约 4 个月+（中国更长）；年度完整。时效敏感月度流量请用 JODI",
		UnitPolicy:    "netWgt/qty=kg、primaryValue=USD；M49 国家码；不跨源合并",
		RequiresKey:   true,
		ConfigKeyName: "COMTRADE_API_KEY",
		HostAllowlist: []string{"comtradeapi.un.org"},
	},
}
