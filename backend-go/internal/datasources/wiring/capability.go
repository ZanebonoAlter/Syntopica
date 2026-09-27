// 发现侧能力前置（board-signal-reports design §10.1，tasks 3.7 发现半边）：
// detect system prompt 注入四源 catalog 元数据 + 硬限制，使 research_question
// 落在四源可回答范围。每源行一律从 datasources.Catalog() 渲染——单一事实来
// 源，绝不手抄第二份（研究阶段 toolsDesc 的 appendCatalogMetadata 同源）。
// 发现阶段不因此取数：注入的是知识，不是授权。
package wiring

import (
	"strings"

	"syntopica-backend/internal/datasources"
)

// discoveryHardLimitNote states, for the detect stage, what none of the four
// sources can answer plus the discovery no-fetch invariant (design §10.1 硬
// 限制：无价格/裂解价差/运价/政策数据、EIA 仅美国；tasks 3.7：发现阶段自身
// 不取数).
const discoveryHardLimitNote = "\n【硬限制】四个数据源均无价格序列、裂解价差、运价、政策文件类数据；EIA 仅美国（不得表述为全球）。发现阶段只做选题，不调用任何数据源取数。"

// SourceCapabilityText renders the four-source catalog (code | coverage |
// frequency | typical lag | unit policy) plus the ensemble hard limits into a
// prompt-ready block. Per-source lines are derived from datasources.Catalog()
// at call time; callers treat empty/nil catalog as "no capability block".
// Injected into the SignalDetector by dataenrichment/wire.go (the service
// package must not import datasources/wiring).
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
