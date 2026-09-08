package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"

	"syntopica-backend/internal/dataenrichment/repository"
)

const maxSelectedAnalysisMethods = 2

// AnalysisMethodRef is the immutable identity persisted by the future
// investigation integration. The actual snapshot wiring remains in task 4.x.
type AnalysisMethodRef struct {
	ID          uint   `json:"id"`
	Title       string `json:"title"`
	ContentHash string `json:"content_hash"`
}

type DroppedAnalysisMethod struct {
	ID     uint   `json:"id"`
	Title  string `json:"title"`
	Reason string `json:"reason"`
}

// AnalysisMethodCardTrace is the per-card assembly audit line (M7.7 留痕):
// 原始身份 + 原始 Content 字节 hash、修辞清洗行数与原因码（只有计数与
// 机器码，被过滤原文绝不落 trace）、注入/舍弃结果、实际注入的清洗后正文。
// 未来 4.6 把它随 investigation input_snapshot 一起固化。
type AnalysisMethodCardTrace struct {
	ID              uint     `json:"id"`
	Title           string   `json:"title"`
	ContentHash     string   `json:"content_hash"`               // 原始 Content 字节 hash（清洗前）
	FilteredLines   int      `json:"filtered_lines"`             // 被剔除的修辞指令行数
	ReasonCodes     []string `json:"reason_codes,omitempty"`     // fixed_phrase | forced_pattern | persona_mimicry
	Injected        bool     `json:"injected"`                   // 是否实际注入 hypothesize prompt
	DroppedReason   string   `json:"dropped_reason,omitempty"`   // content_noncompliant | budget_exceeded | selection_limit
	InjectedContent string   `json:"injected_content,omitempty"` // 实际注入正文（清洗后；被过滤原文不在此处）
}

type AnalysisMethodAssembly struct {
	Prompt     string                    `json:"prompt"`
	MethodRefs []AnalysisMethodRef       `json:"method_refs"`
	Dropped    []DroppedAnalysisMethod   `json:"dropped"`
	Cards      []AnalysisMethodCardTrace `json:"cards"`
}

// AnalysisMethodContentHash returns a stable SHA-256 over the exact UTF-8
// bytes stored in Content. It intentionally performs no trimming or newline
// normalization so snapshots can be verified byte-for-byte.
func AnalysisMethodContentHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// AssembleSelectedAnalysisMethods is a deterministic post-selection helper.
// Input order is relevance order from the future task 2.4 selector. It performs
// no LLM call and no selection. Per card it first runs the M7.7 rhetoric
// sanitizer (line-level, high-confidence only), then enforces the 0-2 limit
// and a rune budget computed over the CLEANED content, dropping whole cards
// rather than slicing their text:
//
//   - 清洗后无任何正文（全部非空行均为高置信修辞指令，或原始 Content
//     即空白/全空白）→ 整卡舍弃（content_noncompliant），无 method_ref；
//   - 部分过滤 → 注入清洗后正文，method_ref 的 content_hash 仍取原始 Content 字节
//     （快照可回放对库里原文做 byte-for-byte 校验）；
//   - 每张卡（注入或舍弃）都留 Cards trace，被过滤原文只以行数+原因码出现。
func AssembleSelectedAnalysisMethods(methods []repository.AnalysisMethod, runeBudget int) AnalysisMethodAssembly {
	out := AnalysisMethodAssembly{
		MethodRefs: []AnalysisMethodRef{},
		Dropped:    []DroppedAnalysisMethod{},
		Cards:      []AnalysisMethodCardTrace{},
	}
	var prompt strings.Builder
	used := 0
	for i, method := range methods {
		title := strings.TrimSpace(method.Title)
		if title == "" {
			title = method.Name
		}
		cleaned, stats := sanitizeAnalysisMethodContent(method.Content)
		card := AnalysisMethodCardTrace{
			ID:            method.ID,
			Title:         title,
			ContentHash:   AnalysisMethodContentHash(method.Content),
			FilteredLines: stats.FilteredLines,
			ReasonCodes:   stats.ReasonCodes(),
		}
		if strings.TrimSpace(cleaned) == "" {
			// 清洗后无任何正文 → 整卡 content_noncompliant：两类情形一视同仁——
			// ① 全部非空行均为高置信修辞指令（FilteredLines>0）；② 原始
			// Content 本身即空白/全空白（FilteredLines=0，空卡无任何可注入
			// 正文，只注入标题没有意义）。不注入、无 method_ref，只留 trace。
			card.DroppedReason = "content_noncompliant"
			out.Dropped = append(out.Dropped, DroppedAnalysisMethod{ID: method.ID, Title: title, Reason: "content_noncompliant"})
			out.Cards = append(out.Cards, card)
			continue
		}
		if i >= maxSelectedAnalysisMethods {
			card.DroppedReason = "selection_limit"
			out.Dropped = append(out.Dropped, DroppedAnalysisMethod{ID: method.ID, Title: title, Reason: "selection_limit"})
			out.Cards = append(out.Cards, card)
			continue
		}
		// 预算按清洗后整卡正文计算；超限仍整卡舍弃。runeBudget<=0 是「无预
		// 算」语义：所有卡一律 budget_exceeded 全弃，0 ref / 0 prompt——调用
		// 方传 0 或负数即声明本次不注入任何方法（行为保持不变）。
		entry := fmt.Sprintf("## %s\n%s\n", title, cleaned)
		entryRunes := utf8.RuneCountInString(entry)
		if runeBudget <= 0 || used+entryRunes > runeBudget {
			card.DroppedReason = "budget_exceeded"
			out.Dropped = append(out.Dropped, DroppedAnalysisMethod{ID: method.ID, Title: title, Reason: "budget_exceeded"})
			out.Cards = append(out.Cards, card)
			continue
		}
		prompt.WriteString(entry)
		used += entryRunes
		card.Injected = true
		card.InjectedContent = cleaned
		out.Cards = append(out.Cards, card)
		out.MethodRefs = append(out.MethodRefs, AnalysisMethodRef{
			ID: method.ID, Title: title, ContentHash: AnalysisMethodContentHash(method.Content),
		})
	}
	out.Prompt = prompt.String()
	return out
}
