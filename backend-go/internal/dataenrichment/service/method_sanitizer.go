package service

// 方法内容修辞清洗（M7.7）：确定性行级过滤，把方法卡中的固定修辞指令行
// （固定金句/套话、强制「不是…而是…」对仗句式、模仿具体作者/人格/口吻/
// 情绪/立场）挡在 hypothesize prompt 之外。设计约束：
//
//   - 只删「高置信」的指令行：主题词 + 指令信号 双条件命中才删，宁漏勿错；
//   - 已知 trade-off（接受）：一条修辞指令被拆成多行（或换行稀释）时，
//     单行可能不再同时满足主题词+信号双条件而漏过过滤。接受这种漏过、
//     不为此放宽匹配——漏网内容由下游 prompt 纪律兜底（方法卡在
//     hypothesize prompt 中只是过程约束、不是事实来源，见
//     boardHypothesizePrompt 硬性纪律 5），误删证据纪律行的代价更高；
//   - 否定性边界说明（禁止/不要/避免使用固定句式…）一律保留，绝不反向误杀；
//   - 中性/克制的语气要求（审慎/严谨/平稳地陈述证据、客观文风、不带情绪）
//     保留——它们是证据纪律而非修辞模仿；persona 规则只过滤「模仿/扮演
//     具体作者人物口吻」或「强制情绪化风格」的高置信指令；
//   - 不调用任何 LLM；被删原文绝不重复进 trace/prompt，只留 reason 码与行数。
//
// 落地点：AssembleSelectedAnalysisMethods（先清洗，再按清洗后正文做整卡预算）。

import (
	"regexp"
	"strings"
)

// 单行过滤原因码（机器可读）。与 DroppedAnalysisMethod.Reason 的整卡码
// "content_noncompliant" 不同，这里是行级码。
const (
	methodFilterFixedPhrase   = "fixed_phrase"    // 固定金句/套话/固定开场收尾指令
	methodFilterForcedPattern = "forced_pattern"  // 强制「不是…而是…」对仗句式指令
	methodFilterPersona       = "persona_mimicry" // 模仿具体作者/人格/口吻/情绪/立场指令
)

var (
	// 行首 markdown/列表前缀：匹配前剥掉，被保留的行仍按原文注入。
	methodLinePrefixRE = regexp.MustCompile(`^(?:[#>*+\-•·]+\s*|\d+[.)、]\s*|[a-zA-Z][.)]\s*)+`)

	// 否定性边界守卫：行虽命中删除模式，但含禁止/劝阻/警示语义 → 保留
	// （「禁止使用固定金句」「避免模仿作者口吻」是合规说明，不是修辞）。
	methodNegationGuardRE = regexp.MustCompile(`禁止|不得|不要|不能|切勿|严禁|避免|无需|无须|忌|绝不|警惕|防止|杜绝|拒绝|不带|反例|(?i:never|do not|don't|avoid|must not|refrain)`)

	// 中性语气守卫：客观/克制的表达纪律不是修辞模仿 → 保留。
	methodNeutralToneGuardRE = regexp.MustCompile(`(?:中性|客观|冷静|克制|平实|朴素|事实性?)(?:的)?(?:语气|口吻|风格|文风|笔调|表达)|不带(?:任何)?(?:情绪|立场|判断|色彩)`)
)

// methodRhetoricMatcher 是一条行级修辞规则：topic（修辞主题）+ 可选
// signal（写作风格元语言信号）双条件命中才删，避免把普通证据步骤误删。
type methodRhetoricMatcher struct {
	reason string
	topic  *regexp.Regexp
	signal *regexp.Regexp // nil = topic 自足（模仿动词本身即指令）
}

var methodRhetoricMatchers = []methodRhetoricMatcher{
	{
		// 强制「不是…而是…」句式：省略号占位模板 / 引号内模板句（含中文弯引号
		// “ ”）/ 紧邻对仗，且行内出现句式/写成/措辞等明确修辞/模板指令信号。
		// signal 刻意不含「表述/框架/开头/结尾/风格」这类宽泛词：它们大量出现
		// 在普通证据纪律里（「结论表述中需区分…」「用因果框架检验…」），
		// 单靠它们触发会误删纪律行。
		reason: methodFilterForcedPattern,
		topic: regexp.MustCompile(`不是\s*(?:…+|[.。]{2,}|·+)\s*而是` +
			`|["'“”「『][^"'“”」』]{0,30}不是[^"'“”」』]{1,40}，?\s*而是[^"'“”」』]{0,40}["'“”」』]` +
			`|不是[^，。；！？!?]{1,24}而是`),
		signal: regexp.MustCompile(`句式|格式|模板|写成|改写|措辞|表达为|金句|修辞|对仗|升华|点睛|口吻`),
	},
	{
		// 英文对仗句式指令："not X but Y" + pattern/construction/write 等元语言。
		reason: methodFilterForcedPattern,
		topic:  regexp.MustCompile(`(?i)\bnot\b[^.\n]{1,30}\bbut\b`),
		signal: regexp.MustCompile(`(?i)pattern|construction|template|trope|reframe|frame|\bwrit(?:e|es|ing)\b|\bphras(?:e|es|ing)\b|\bword(?:ing|ed)?\b`),
	},
	{
		// 固定金句/套话指令：主题名词 + 使用/插入/结尾/每段等使用信号。
		reason: methodFilterFixedPhrase,
		topic:  regexp.MustCompile(`金句|名句|警句|格言|套话|口头禅|万能句|固定句|模板句|点睛之笔|收尾句|开场白|招牌句|标志性(?:句子|表达)`),
		signal: regexp.MustCompile(`使用|采用|套用|插入|加入|放在|置于|准备|储备|备好|牢记|背下|背诵|固定|每[段篇节章]|各段|结尾|收尾|开头|开场|点题|升华|点睛|反复|一律|统一|都要`),
	},
	{
		// 英文固定句指令：signature phrase/catchphrase/punchline + 使用信号。
		reason: methodFilterFixedPhrase,
		topic:  regexp.MustCompile(`(?i)\b(?:signature (?:phrase|sentence|line)|catch-?phrase|punch ?line|memorable (?:line|quote|closing)|fixed (?:sentence|phrase|wording)|template sentence|stock phrase)\b`),
		signal: regexp.MustCompile(`(?i)\b(?:use|uses|using|used|insert|add|place|put|end with|open with|start with|prepare|stock(?:ed|ing)?)\b|\balways\b|\bevery\b`),
	},
	{
		// 模仿具体作者/人格/口吻/情绪/立场，只过滤高置信指令：
		// ① 模仿/扮演动词 + 人格/风格名词；② 「以/用…口吻」仅当锚定具体
		// 人物/角色身份（书名、作者、记者、基金经理等）才算 persona——普通
		// 语气纪律（审慎/严谨/平稳地陈述证据）不是模仿，保留；③ 情绪化/
		// 立场化风格强制。
		reason: methodFilterPersona,
		topic: regexp.MustCompile(
			`(?:模仿|扮演|化身|假扮|装作)[^，。；\n]{0,16}(?:作者|人格|人设|口吻|语气|文风|腔调|笔调|风格|声音|声线|行文|写法|笔法|叙事者)` +
				`|(?:以|用|保持|带着|充满|采用)[^，。；\n]{0,12}(?:《[^》]{1,24}》|作者|作家|记者|主持人|评论员|分析师|基金经理|交易员|辩手|说书人|小说家|专栏作家|某人|某某)[^，。；\n]{0,8}的?(?:口吻|语气|腔调|笔调|文风|人格|人设|声线)` +
				`|(?:写作|行文|叙述|表达|措辞|文章)(?:时|中|上)?(?:必须|要|应|须|力求|尽量)?(?:保持|带着|充满|体现|贯穿)[^，。；\n]{0,12}(?:情绪|愤怒|讽刺|冷嘲|嘲讽|戏谑|煽动|悲情|激昂|悲愤|立场)` +
				`|(?:立场|情绪)(?:鲜明|强硬|对立|激烈|化)?地?(?:写作|行文|表达|陈述)`),
	},
	{
		// 英文人格模仿：write in the style of / mimic the voice / persona of。
		// adopt 不在模仿动词之列——"Always adopt a neutral and objective
		// tone" 是普通语气纪律，必须保留。
		reason: methodFilterPersona,
		topic: regexp.MustCompile(`(?i)(?:\b(?:writ(?:e|es|ing)|speak|narrate|talk)\b[^.\n]{0,40}\bin the (?:style|voice|tone|manner) of\b` +
			`|\b(?:mimic|imitate|emulate|channel|impersonate)\b[^.\n]{0,30}\b(?:style|voice|tone|persona|character|manner)\b` +
			`|\bpersona of\b|\bin the voice of\b)`),
	},
}

// methodLineFilterReason returns the filter reason code for one line, or ""
// when the line must survive. High-confidence only: topic(+signal) both hit;
// negation / neutral-tone guards win over any hit（不能反向误杀）。
func methodLineFilterReason(rawLine string) string {
	line := strings.TrimSpace(methodLinePrefixRE.ReplaceAllString(strings.TrimSpace(rawLine), ""))
	if line == "" {
		return ""
	}
	for _, m := range methodRhetoricMatchers {
		if !m.topic.MatchString(line) {
			continue
		}
		if m.signal != nil && !m.signal.MatchString(line) {
			continue
		}
		if methodNegationGuardRE.MatchString(line) || methodNeutralToneGuardRE.MatchString(line) {
			return "" // 否定性边界/中性纪律说明：保留
		}
		return m.reason
	}
	return ""
}

// methodSanitizeStats summarizes one card's line filtering. 原始被删文本不落
// 结构体（防泄漏），只有行数与原因码计数。
type methodSanitizeStats struct {
	FilteredLines int
	ReasonCounts  map[string]int
}

// ReasonCodes returns the distinct reason codes in a deterministic order.
func (s methodSanitizeStats) ReasonCodes() []string {
	codes := []string{}
	for _, c := range []string{methodFilterFixedPhrase, methodFilterForcedPattern, methodFilterPersona} {
		if s.ReasonCounts[c] > 0 {
			codes = append(codes, c)
		}
	}
	return codes
}

// sanitizeAnalysisMethodContent drops high-confidence rhetorical instruction
// lines from a method card and returns the cleaned content plus stats.
// Deterministic（无 LLM）；普通证据步骤与否定性边界说明原样保留。
func sanitizeAnalysisMethodContent(content string) (string, methodSanitizeStats) {
	stats := methodSanitizeStats{ReasonCounts: map[string]int{}}
	if strings.TrimSpace(content) == "" {
		return content, stats
	}
	lines := strings.Split(content, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		reason := methodLineFilterReason(line)
		if reason != "" {
			stats.FilteredLines++
			stats.ReasonCounts[reason]++
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n"), stats
}
