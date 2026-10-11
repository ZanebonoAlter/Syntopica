// Package wiring 发现侧能力前置（board-signal-reports design §10.1，tasks 3.7
// 发现半边）：detect system prompt 注入七源 catalog 元数据 + 能力边界，使
// research_question 落在可回答范围。每源行一律从 datasources.Catalog() 渲
// 染——单一事实来源，绝不手抄第二份（研究阶段 toolsDesc 的
// appendCatalogMetadata 同源）。发现阶段不因此取数：注入的是知识，不是授权。
package wiring

import (
	"strings"

	"syntopica-backend/internal/datasources"
)

// discoveryHardLimitNote states, for the detect stage, the ensemble
// capability boundaries (add-fast-frequency-datasources：价格/事件源已接入，
// 旧「均无价格」废止，按类划界) plus the discovery no-fetch invariant
// (design §10.1 硬限制；tasks 3.7：发现阶段自身不取数).
const discoveryHardLimitNote = "\n【能力边界】价格/行情类数据只有 yahoo_finance 与 fred（日度序列，可算价差/涨跌幅）；事件层只有 gdelt 的结构化热度/情绪走势（非事实核查、无新闻正文）；量类源（eia_wpsr/jodi_oil_primary/wb_wdi/un_comtrade）不含价格；EIA 仅美国（不得表述为全球）；任何源均无运价与政策文本原文。发现阶段只做选题，不调用任何数据源取数。"

// SourceCapabilityText renders the seven-source catalog (code | coverage |
// frequency | typical lag | unit policy) plus the ensemble capability
// boundaries into a prompt-ready block. Per-source lines are derived from
// datasources.Catalog() at call time; callers treat empty/nil catalog as
// "no capability block". Injected into the SignalDetector by
// dataenrichment/wire.go (the service package must not import
// datasources/wiring).
func SourceCapabilityText() string {
	var b strings.Builder
	b.WriteString("【可用数据源目录】研究问题只能落在以下数据源的覆盖范围内：")
	for _, d := range datasources.Catalog() {
		b.WriteString("\n- " + d.Code +
			" | " + d.Coverage +
			" | " + d.Frequency +
			" | " + d.TypicalLag +
			" | " + d.UnitPolicy)
	}
	b.WriteString(discoveryHardLimitNote)
	b.WriteString(comtradeMeasuredCoverageNote)
	return b.String()
}
