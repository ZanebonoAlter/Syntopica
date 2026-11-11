package main

// Blind-annotation worksheet export (design D12 / R-11): the worksheet is
// generated from the FROZEN samples and contains ONLY the article text and
// the mention context — never candidate descriptions, never system outputs,
// never existing entity IDs. Disputed items are pre-listed here and counted.

import (
	"encoding/csv"
	"fmt"
	"io"
)

// WorksheetRow is one annotation line.
type WorksheetRow struct {
	SampleID     string
	ArticleTitle string
	ContextText  string
	Mention      string
	Disputed     bool
}

// ExportWorksheet writes the blind worksheet as CSV. Columns:
// sample_id, article_title, context_text, mention, disputed(0/1), gold_entity(EMPTY — annotator fills).
// No candidate/system columns exist by construction.
func ExportWorksheet(w io.Writer, samples []EvalSample) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"sample_id", "article_title", "context_text", "mention", "disputed", "gold_entity"}); err != nil {
		return err
	}
	for _, s := range samples {
		if err := cw.Write([]string{s.SampleID, s.ArticleTitle, s.ContextText, s.Mention, bool01(s.Disputed), ""}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// CountDisputed reports the pre-listed disputed items (报告须给出数量).
func CountDisputed(samples []EvalSample) int {
	n := 0
	for _, s := range samples {
		if s.Disputed {
			n++
		}
	}
	return n
}

func bool01(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// FormatDisputedReport renders the disputed-item disclosure line.
func FormatDisputedReport(samples []EvalSample) string {
	return fmt.Sprintf("争议样本预列 %d 条（运行前冻结，不计身份分）", CountDisputed(samples))
}
