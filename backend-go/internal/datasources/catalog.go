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
	{
		Code:        "yahoo_finance",
		Name:        "Yahoo Finance 日度行情（期货/商品/股指/汇率/加密）",
		Provider:    "Yahoo Finance",
		HomepageURL: "https://finance.yahoo.com/",
		Coverage:    "核定 15 symbol：能源期货 CL=F/BZ=F/HO=F/RB=F/NG=F、贵金属 GC=F/SI=F、股指 ^GSPC/^IXIC/^DJI/^VIX、汇率 JPY=X/EUR=X/USDCNY=X、加密 BTC-USD；枚举外拒绝",
		Topics:      []string{"prices", "energy", "macro"},
		Frequency:   "daily",
		TypicalLag:  "T+0（收盘后当日可得）；非官方接口无 SLA，结构漂移显式失败",
		UnitPolicy:  "OHLC 原计价货币（meta.currency 透传），不换算、不跨币种合并；null 收盘价如实缺失不插补",
		RequiresKey: false,
		// 非官方 chart API（2026-10-02 实测匿名可用；同类 stooq 已被反爬干掉，
		// SCHEMA_CHANGED/SOURCE_UNAVAILABLE 语义天然兜底，FRED 日度现货作备份）。
		HostAllowlist: []string{"query1.finance.yahoo.com"},
	},
	{
		Code:          "fred",
		Name:          "FRED 圣路易斯联储数据库（宏观/价格序列）",
		Provider:      "Federal Reserve Bank of St. Louis",
		HomepageURL:   "https://fred.stlouisfed.org/",
		Coverage:      "80 万+ 序列（格式即边界，非枚举）：DCOILWTICO/DCOILBRENTEU 日度 WTI/Brent 现货、SOFR、美债、DXY 等宏观全谱",
		Topics:        []string{"prices", "macro", "rates"},
		Frequency:     "daily",
		TypicalLag:    "日度序列 T+0~T+1（随源端更新）",
		UnitPolicy:    "序列原生单位与频率原样透传（units/frequency 字段），缺失观测（.）为 null 不转 0，不换算不推断",
		RequiresKey:   true,
		ConfigKeyName: "FRED_API_KEY",
		HostAllowlist: []string{"api.stlouisfed.org"},
	},
	{
		Code:          "gdelt",
		Name:          "GDELT DOC 新闻事件热度时间线",
		Provider:      "The GDELT Project",
		HomepageURL:   "https://www.gdeltproject.org/",
		Coverage:      "全球新闻聚合（timelinevol 声量热度 / timelinetone 情绪基调）；只给带时间戳的结构化量，不给新闻正文或观点文本",
		Topics:        []string{"events", "news-volume"},
		Frequency:     "daily",
		TypicalLag:    "小时级（准实时新闻聚合）",
		UnitPolicy:    "无量纲热度/情绪指数，随源端定义；非事实核查、非新闻检索，不得用于直接断言事实",
		RequiresKey:   false,
		HostAllowlist: []string{"api.gdeltproject.org"},
	},
}
