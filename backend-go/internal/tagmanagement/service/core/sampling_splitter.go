package core

import (
	"regexp"
	"strings"
)

// 采样切分器（long-form-sampled-tagging）：为 mono 打标输入在 4000 runes
// 预算内拼出全文代表。与 aggregate 路径的 splitSections 职责不同——那边是
// "每栏目一次调用"的专用件（带 dropIntro/mergeShort/splitLong 后处理），这边
// 只要"宽松标题切 + 空行兜底 + 拼预算"，两者互不依赖。

const (
	// narrativeSectionThreshold: 标题段数不超过该值视为叙事型，走头中尾采样。
	narrativeSectionThreshold = 3
	// minSectionSampleRunes: 文集型采样每段正文保底预算。
	minSectionSampleRunes = 120
	// sentenceFallbackWindow: 句界回退窗口（runes），窗口内无句末标点则硬切。
	sentenceFallbackWindow = 50
	// ellipsisMark: 采样省略标记（design D4：裸省略号，不改 extractor 系统提示）。
	ellipsisMark = "……"
	// otherSectionsPrefix: 段数过多退化时标题清单行的前缀。
	otherSectionsPrefix = "其他栏目："
)

var (
	// 图片语法整体删除（alt 不保留）；行内链接仅保留 text。先图后链，
	// 使 [![alt](url1)](url2) 这类嵌套整体消解。
	imageNoiseRe  = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)
	linkNoiseRe   = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	headingLineRe = regexp.MustCompile(`^#{1,6}\s+(.+)$`)
)

// stripNoise 剥离 markdown 图片与链接噪声（spec：噪声不得占用采样预算）。
func stripNoise(body string) string {
	body = imageNoiseRe.ReplaceAllString(body, "")
	body = linkNoiseRe.ReplaceAllString(body, "$1")
	return body
}

// splitByHeadings 认 `#`~`######` 标题行切段（宽松切分：标题层级不统一时
// h1/h3 混用都能切）。标题前的前言并入第一段保住导读信息；无标题时整篇
// 聚合为单一无标题段（叙事型，配合 narrativeSectionThreshold 走头中尾）。
// 代码块 fence 内 `#` 注释行的误判是已知限制（design D3，打标输入带代码场景少）。
func splitByHeadings(body string) []Section {
	var sections []Section
	var current *Section
	var intro strings.Builder
	for _, line := range strings.Split(body, "\n") {
		m := headingLineRe.FindStringSubmatch(strings.TrimRight(line, "\r"))
		if m != nil {
			if current != nil {
				current.Content = strings.TrimSpace(current.Content)
				sections = append(sections, *current)
			}
			current = &Section{Title: strings.TrimSpace(m[1])}
			continue
		}
		if current != nil {
			current.Content += line + "\n"
		} else {
			intro.WriteString(line)
			intro.WriteString("\n")
		}
	}
	if current != nil {
		current.Content = strings.TrimSpace(current.Content)
		sections = append(sections, *current)
	}
	if len(sections) == 0 {
		content := strings.TrimSpace(body)
		if content == "" {
			return nil
		}
		return []Section{{Title: "", Content: content}}
	}
	if head := strings.TrimSpace(intro.String()); head != "" {
		sections[0].Content = strings.TrimSpace(head + "\n\n" + sections[0].Content)
	}
	return sections
}

// truncateAtSentenceBoundary 将 text 截断到 n runes：从切点回退找句末标点
// （。！？换行），窗口 sentenceFallbackWindow 内无句界则硬切。
func truncateAtSentenceBoundary(text string, n int) string {
	r := []rune(text)
	if len(r) <= n {
		return text
	}
	limit := n - sentenceFallbackWindow
	if limit < 0 {
		limit = 0
	}
	for i := n - 1; i >= limit; i-- {
		switch r[i] {
		case '。', '！', '？', '\n':
			return string(r[:i+1])
		}
	}
	return string(r[:n])
}

// sampleSections 文集型采样：标题全保 + 正文预算均分（每段保底
// minSectionSampleRunes）；保底分配超出总预算时尾部段落退化为
// "其他栏目：标题清单"行（spec Scenario: 段数过多退化为标题清单）。
// 段短于均分预算全保，被截断的段落尾带省略标记。
func sampleSections(sections []Section, budget int) string {
	n := len(sections)
	if n == 0 {
		return ""
	}
	titleTotal := 0
	for _, s := range sections {
		if s.Title != "" {
			titleTotal += len([]rune(s.Title)) + 1 // 标题行 + 换行
		}
	}
	perSection := (budget - titleTotal - 2*(n-1)) / n // 段间 "\n\n" 开销
	if perSection < minSectionSampleRunes {
		return sampleSectionsWithFallback(sections, budget)
	}

	var b strings.Builder
	for i, s := range sections {
		if i > 0 {
			b.WriteString("\n\n")
		}
		if s.Title != "" {
			b.WriteString(s.Title)
			b.WriteString("\n")
		}
		trunc := truncateAtSentenceBoundary(s.Content, perSection)
		b.WriteString(trunc)
		if len([]rune(trunc)) < len([]rune(s.Content)) {
			b.WriteString(ellipsisMark)
		}
	}
	return b.String()
}

// sampleSectionsWithFallback 保底分配超出总预算的退化路径：前 K 段按保底
// 预算采样、标题照写，其余段标题归入一行清单（不保留正文）。从 K=N-1 向下
// 尝试，取预算内可容纳的最大 K；极端兜底（标题总量本身超预算）只留清单行。
func sampleSectionsWithFallback(sections []Section, budget int) string {
	for keep := len(sections) - 1; keep >= 0; keep-- {
		out := buildFallbackSample(sections, keep)
		if len([]rune(out)) <= budget {
			return out
		}
	}
	var titles strings.Builder
	for i, s := range sections {
		if i > 0 {
			titles.WriteString("、")
		}
		titles.WriteString(s.Title)
	}
	return truncateAtSentenceBoundary(otherSectionsPrefix+titles.String(), budget)
}

func buildFallbackSample(sections []Section, keep int) string {
	var b strings.Builder
	for i := 0; i < keep; i++ {
		if i > 0 {
			b.WriteString("\n\n")
		}
		s := sections[i]
		if s.Title != "" {
			b.WriteString(s.Title)
			b.WriteString("\n")
		}
		trunc := truncateAtSentenceBoundary(s.Content, minSectionSampleRunes)
		b.WriteString(trunc)
		if len([]rune(trunc)) < len([]rune(s.Content)) {
			b.WriteString(ellipsisMark)
		}
	}
	if rest := sections[keep:]; len(rest) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(otherSectionsPrefix)
		for i, s := range rest {
			if i > 0 {
				b.WriteString("、")
			}
			b.WriteString(s.Title)
		}
	}
	return b.String()
}

// sampleHeadMidTail 叙事型采样：头/中/尾约 2:1:1（budget=4000 → 2000/1000/1000），
// 段间省略标记。中段放在头尾之间的剩余区间正中，保证三段来源区间互不重叠。
func sampleHeadMidTail(body string, budget int) string {
	r := []rune(body)
	if len(r) <= budget {
		return body
	}
	headLen := budget / 2
	tailLen := budget / 4
	midLen := budget - headLen - tailLen
	midSpace := (len(r) - tailLen) - headLen // 头尾之间可放中段的空间
	if midLen > midSpace {
		midLen = midSpace
	}
	midStart := headLen + (midSpace-midLen)/2
	var b strings.Builder
	b.WriteString(string(r[:headLen]))
	b.WriteString(ellipsisMark)
	b.WriteString(string(r[midStart : midStart+midLen]))
	b.WriteString(ellipsisMark)
	b.WriteString(string(r[len(r)-tailLen:]))
	return b.String()
}

// sampleTaggingSummary 打标正文采样入口：剥噪声 → 未超预算原样返回（86%
// 短文零变化）→ 按段数分流文集均分采样 / 叙事头中尾采样。
func sampleTaggingSummary(body string, budget int) string {
	clean := stripNoise(body)
	if len([]rune(clean)) <= budget {
		return clean
	}
	sections := splitByHeadings(clean)
	if len(sections) <= narrativeSectionThreshold {
		return sampleHeadMidTail(clean, budget)
	}
	return sampleSections(sections, budget)
}
