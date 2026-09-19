package core

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/models"
)

// ---------- 1.1 stripNoise ----------

func TestStripNoiseRemovesImages(t *testing.T) {
	body := "前言文字。\n![专辑封面](https://cdn.example.com/cover.png)\n中段文字。\n![](https://cdn.example.com/photo.JPG)\n结尾文字。"

	clean := stripNoise(body)

	require.NotContains(t, clean, "cdn.example.com")
	require.NotContains(t, clean, "![")
	require.Contains(t, clean, "前言文字。")
	require.Contains(t, clean, "中段文字。")
	require.Contains(t, clean, "结尾文字。")
}

func TestStripNoiseKeepsLinkTextOnly(t *testing.T) {
	body := "开头 [文档](https://docs.example.com/a) 中间 [另一个](https://docs.example.com/b) 结尾"

	clean := stripNoise(body)

	require.Equal(t, "开头 文档 中间 另一个 结尾", clean)
	require.NotContains(t, clean, "docs.example.com")
	require.NotContains(t, clean, "[")
	require.NotContains(t, clean, "]")
}

func TestStripNoiseDissolvesNestedImageLink(t *testing.T) {
	body := "[![封面图](https://img.example.com/cover.png)](https://post.example.com/1)"

	clean := stripNoise(body)

	require.Equal(t, "", clean) // 嵌套整体消解，图片 alt 也不保留
}

func TestStripNoiseKeepsCleanTextUntouched(t *testing.T) {
	body := "纯文本正文，没有链接与图片。"

	require.Equal(t, body, stripNoise(body))
}

// ---------- 1.2 splitByHeadings ----------

func TestSplitByHeadingsSplitsH2DigestWithIntro(t *testing.T) {
	md := "# 周刊\n\n引言一句话导读。\n\n## 栏目一\n内容一。\n\n## 栏目二\n内容二。\n\n## 栏目三\n内容三。"

	sections := splitByHeadings(md)

	// design：认 #~###### 标题行——h1「周刊」也是切分边界单独成段，共 4 段。
	require.Len(t, sections, 4)
	require.Equal(t, []string{"周刊", "栏目一", "栏目二", "栏目三"}, sectionTitles(sections))
	// h1 与首个 h2 之间的前言并入第一段，保住导读信息。
	require.Equal(t, "引言一句话导读。", sections[0].Content)
	require.Equal(t, "内容一。", sections[1].Content)
	require.Equal(t, "内容二。", sections[2].Content)
	require.Equal(t, "内容三。", sections[3].Content)
}

func TestSplitByHeadingsSplitsH3OnlyBody(t *testing.T) {
	md := "### 开发\n开发正文。\n\n### 设计\n设计正文。"

	sections := splitByHeadings(md)

	require.Len(t, sections, 2)
	require.Equal(t, []string{"开发", "设计"}, sectionTitles(sections))
	require.Equal(t, "开发正文。", sections[0].Content)
	require.Equal(t, "设计正文。", sections[1].Content)
}

func TestSplitByHeadingsAggregatesNarrativeIntoSingleSection(t *testing.T) {
	md := "第一段正文。\n\n第二段正文。\n\n第三段正文。"

	sections := splitByHeadings(md)

	require.Len(t, sections, 1)
	require.Equal(t, "", sections[0].Title)
	require.Equal(t, md, sections[0].Content)
}

func TestSplitByHeadingsHandlesAdjacentHeadings(t *testing.T) {
	md := "## 标题A\n## 标题B\n正文内容。"

	sections := splitByHeadings(md)

	require.Len(t, sections, 2)
	require.Equal(t, "标题A", sections[0].Title)
	require.Empty(t, sections[0].Content)
	require.Equal(t, "标题B", sections[1].Title)
	require.Equal(t, "正文内容。", sections[1].Content)
}

// ---------- 1.3 truncateAtSentenceBoundary ----------

func TestTruncateAtSentenceBoundaryFallsBackToSentenceEnd(t *testing.T) {
	text := strings.Repeat("前", 30) + "。这句是完整句。" + strings.Repeat("后", 100)

	// 切点 40 落在"后"填充区，50 runes 回退窗口内命中句号 → 输出 38 runes。
	truncated := truncateAtSentenceBoundary(text, 40)

	require.Equal(t, strings.Repeat("前", 30)+"。这句是完整句。", truncated)
	require.Len(t, []rune(truncated), 38)
}

func TestTruncateAtSentenceBoundaryKeepsTrailingPunctuation(t *testing.T) {
	text := strings.Repeat("甲", 99) + "。后面还有"

	require.Equal(t, strings.Repeat("甲", 99)+"。", truncateAtSentenceBoundary(text, 100))
}

func TestTruncateAtSentenceBoundaryHardCutsWithoutPunctuation(t *testing.T) {
	text := strings.Repeat("项", 200) // 无 。！？\n

	truncated := truncateAtSentenceBoundary(text, 100)

	require.Equal(t, strings.Repeat("项", 100), truncated)
	require.Len(t, []rune(truncated), 100)
}

// ---------- 1.4 sampleSections ----------

func TestSampleSectionsKeepsAllTitlesWithinBudget(t *testing.T) {
	var sections []Section
	for i := 0; i < 15; i++ {
		sections = append(sections, Section{Title: fmt.Sprintf("栏目%02d", i), Content: strings.Repeat("正", 300)})
	}

	out := sampleSections(sections, 4000)

	runes := []rune(out)
	require.LessOrEqual(t, len(runes), 4200)
	for i := 0; i < 15; i++ {
		require.Contains(t, out, fmt.Sprintf("栏目%02d", i))
	}
	require.Contains(t, out, ellipsisMark)
}

func TestSampleSectionsDegradesToTitleListWhenTooManySections(t *testing.T) {
	// 标题必须足够长（12 runes）才能让保底分配超预算触发退化；短标题不会触发。
	var sections []Section
	for i := 0; i < 30; i++ {
		sections = append(sections, Section{
			Title:   fmt.Sprintf("栏目「重点推荐专题」%02d", i),
			Content: fmt.Sprintf("第%02d段独特开头。", i) + strings.Repeat("正", 491),
		})
	}

	out := sampleSections(sections, 4000)

	runes := []rune(out)
	require.LessOrEqual(t, len(runes), 4200)
	require.Contains(t, out, otherSectionsPrefix)
	require.Contains(t, out, "第00段独特开头。")    // 首段正文在场
	require.NotContains(t, out, "第29段独特开头。") // 末段正文不在场
	require.Contains(t, out, "栏目「重点推荐专题」29") // 末段标题只出现在清单行
}

func TestSampleSectionsKeepsShortDigestIntact(t *testing.T) {
	var sections []Section
	for i := 0; i < 8; i++ {
		sections = append(sections, Section{Title: fmt.Sprintf("常规%02d", i), Content: strings.Repeat("文", 300)})
	}

	out := sampleSections(sections, 4000)

	runes := []rune(out)
	require.LessOrEqual(t, len(runes), 4200)
	for i := 0; i < 8; i++ {
		require.Contains(t, out, fmt.Sprintf("常规%02d", i))
	}
}

// ---------- 1.5 sampleHeadMidTail（标记段落法，不用实现公式自证） ----------

func TestSampleHeadMidTailMarksDisjointWindows(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 20; i++ {
		b.WriteString(fmt.Sprintf("《段%02d开始》", i))
		b.WriteString(strings.Repeat("填", 493)) // 每段精确 500 runes
	}
	body := b.String()
	require.Equal(t, 10000, len([]rune(body)))

	out := sampleHeadMidTail(body, 4000)

	runes := []rune(out)
	require.Equal(t, 4004, len(runes)) // 2000 + "……" + 1000 + "……" + 1000
	require.Equal(t, string([]rune(body)[:2000]), string(runes[:2000]))
	require.Equal(t, string([]rune(body)[9000:]), string(runes[3004:]))
	require.Contains(t, out, "《段10开始》")    // 中段采样窗正中 [5000:6000)
	require.NotContains(t, out, "《段05开始》") // 头/中之间的空洞 [2000:5000)
	require.NotContains(t, out, "《段15开始》") // 中/尾之间的空洞 [6000:9000)
}

// ---------- 2.2 端到端夹具三路径 ----------

func TestSampleTaggingSummarySspaiWeeklyFixture(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "fixture_sspai_weekly.md"))
	require.NoError(t, err)
	body := string(raw)

	clean := stripNoise(body)
	require.Greater(t, len([]rune(clean)), 4000)

	out := sampleTaggingSummary(body, maxSummaryRunesForTagging)

	runes := []rune(out)
	require.LessOrEqual(t, len(runes), 4200)
	// spec 只承诺完整语法删除：夹具中 3 处孤立 "!["（残缺标记，非合法图片语法）
	// 允许以 2 runes 残留；URL 与完整链接/图片语法必须不残留。
	require.NotContains(t, out, "](http")
	require.NotContains(t, out, "https://")
	require.NotContains(t, out, ".JPG")
	require.Contains(t, out, ellipsisMark)

	headingRe := regexp.MustCompile(`(?m)^#{2,3} (.+)$`)
	matches := headingRe.FindAllStringSubmatch(string(raw), -1)
	require.Len(t, matches, 10)
	seen := make(map[string]struct{}, len(matches))
	for _, m := range matches {
		seen[m[1]] = struct{}{}
	}
	for title := range seen {
		require.Contains(t, out, title)
	}
}

func TestSampleTaggingSummaryNarrativeFixture(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "fixture_narrative.md"))
	require.NoError(t, err)
	body := string(raw)

	clean := stripNoise(body)
	require.Greater(t, len([]rune(clean)), 4000)
	require.Len(t, splitByHeadings(clean), 1) // 无标题 → 叙事型单段

	out := sampleTaggingSummary(body, maxSummaryRunesForTagging)

	runes := []rune(out)
	require.LessOrEqual(t, len(runes), 4200)
	require.Equal(t, string([]rune(clean)[:2000]), string(runes[:2000]))
	require.Contains(t, out, ellipsisMark)
}

func TestSampleTaggingSummaryShortRSSFixture(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "fixture_short_rss.md"))
	require.NoError(t, err)
	body := string(raw)

	clean := stripNoise(body)
	require.Less(t, len([]rune(clean)), 4000)

	out := sampleTaggingSummary(body, maxSummaryRunesForTagging)

	require.Equal(t, clean, out) // 未超预算零变化
	require.NotContains(t, out, ellipsisMark)
}

// ---------- 2.3 buildArticleSummary 短文回归 ----------

func TestBuildArticleSummaryShortInputPassesThrough(t *testing.T) {
	body := bodyOf("这是一篇短文的正文内容。", 3000)
	article := models.Article{AIContentSummary: body}

	summary := buildArticleSummary(article)

	require.Equal(t, body, summary)
	require.NotContains(t, summary, ellipsisMark)
}

func TestBuildArticleSummaryFallsBackToFirecrawlContent(t *testing.T) {
	body := bodyOf("Firecrawl 抓取的正文内容。", 3000)
	article := models.Article{FirecrawlContent: body}

	summary := buildArticleSummary(article)

	require.Equal(t, body, summary)
}

func TestBuildArticleSummaryStripsNoiseFromShortInput(t *testing.T) {
	body := "开头 [点这里](https://link.example.com/go) 中段 ![](https://img.example.com/1.png) 结尾。"
	article := models.Article{AIContentSummary: body}

	summary := buildArticleSummary(article)

	require.Equal(t, stripNoise(body), summary)
	require.NotContains(t, summary, "https://")
}
