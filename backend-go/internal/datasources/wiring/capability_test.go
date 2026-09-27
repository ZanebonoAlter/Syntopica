package wiring

import (
	"strings"
	"testing"

	"syntopica-backend/internal/datasources"
)

// 发现侧能力文本（board-signal-reports tasks 3.7 发现半边）内容契约：每源
// 行四字段（code | Coverage | Frequency | TypicalLag | UnitPolicy）全部从
// datasources.Catalog() 派生——单一事实来源，改 catalog 必然改渲染，出现
// 手抄第二份的漂移直接红。硬限制句钉住三件事：四源无价格/裂解价差/运价/
// 政策数据、EIA 仅美国、发现阶段不取数。
func TestSourceCapabilityTextDerivesFromCatalog(t *testing.T) {
	text := SourceCapabilityText()
	catalog := datasources.Catalog()
	if len(catalog) == 0 {
		t.Fatalf("catalog must not be empty")
	}
	for _, d := range catalog {
		line := d.Code + " | " + d.Coverage + " | " + d.Frequency + " | " + d.TypicalLag + " | " + d.UnitPolicy
		if !strings.Contains(text, line) {
			t.Fatalf("capability text missing catalog line for %s:\nwant line: %q\ntext: %q", d.Code, line, text)
		}
	}
	for _, code := range []string{"eia_wpsr", "jodi_oil_primary", "wb_wdi", "un_comtrade"} {
		if !strings.Contains(text, code) {
			t.Fatalf("capability text missing source %s", code)
		}
	}
	for _, phrase := range []string{
		"裂解价差", "运价", "政策",
		"EIA 仅美国",
		"发现阶段", "不调用任何数据源取数",
	} {
		if !strings.Contains(text, phrase) {
			t.Fatalf("capability text missing hard-limit phrase %q", phrase)
		}
	}
	// Comtrade 订阅档实测覆盖限制（design §10.1：不把无覆盖包装成可查）。
	if !strings.Contains(text, comtradeMeasuredCoverageNote) {
		t.Fatalf("capability text must carry the comtrade measured-coverage caveat")
	}
}
