package board

import (
	"fmt"
	"strings"
)

// renderExpandProfilePrompt 渲染版块画像（扩充两路共用 prompt 头，design D4）：
// 版块名 + 描述 + 现有构成标签（组合标签带标记）+ 近期内容标题。
func renderExpandProfilePrompt(profile *boardExpandProfile) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "【目标版块】%s（ID=%d）", profile.BoardLabel, profile.BoardID)
	if d := strings.TrimSpace(profile.BoardDescription); d != "" {
		builder.WriteString("：" + d)
	}
	builder.WriteString("\n")
	if len(profile.Composition) > 0 {
		builder.WriteString("现有构成：\n")
		for _, comp := range profile.Composition {
			marker := ""
			if comp.LabelType == "composite" {
				marker = "（组合）"
			}
			fmt.Fprintf(&builder, "  - %s%s\n", comp.Label, marker)
		}
	}
	if len(profile.RecentSections) > 0 {
		builder.WriteString("近期内容：\n")
		for _, title := range profile.RecentSections {
			fmt.Fprintf(&builder, "  · %s\n", title)
		}
	}
	return builder.String()
}

// buildExpandAuxPrompt 渲染扩充×单标签二分类裁决 prompt（spec: 版块画像上下文
// 与二分类裁决）。LLM 只判断「候选属于该版块吗」，不比较版块、不指定目标。
func buildExpandAuxPrompt(profile *boardExpandProfile, candidates []expandAuxCandidate) string {
	var builder strings.Builder
	builder.WriteString("你是一个语义板块分析助手。\n\n")
	builder.WriteString(renderExpandProfilePrompt(profile))
	builder.WriteString("\n任务：逐个判断下列候选辅助标签是否属于该目标版块。\n")
	builder.WriteString("决策空间只有两种：merge_into_existing（属于该版块，将挂载进该版块）或 skip（不属于）。\n")
	builder.WriteString("判断原则：\n")
	builder.WriteString("- 标签语义与版块主题一致（指向同一主题域）→ merge_into_existing\n")
	builder.WriteString("- 标签与版块主题无关、或过于泛化不足以归属 → skip\n")
	builder.WriteString("- 仅输出明确属于的候选，模糊的不输出（宁缺毋滥）；每条建议的 auxiliary_label_ids 只能取自候选列表\n\n")
	builder.WriteString("返回 JSON 格式：{\"suggestions\": [{\"decision\": \"merge_into_existing|skip\", \"board_label\": \"\", \"description\": \"\", \"auxiliary_label_ids\": [id1], \"reason\": \"判断理由\"}]}\n\n")
	builder.WriteString("候选辅助标签：\n")
	for _, c := range candidates {
		evidence := make([]string, 0, 2)
		if c.SimDist != nil {
			evidence = append(evidence, fmt.Sprintf("与版块语义距离 %.4f", *c.SimDist))
		}
		if c.Cooccur > 0 {
			evidence = append(evidence, fmt.Sprintf("与构成标签共现 %d 篇", c.Cooccur))
		}
		fmt.Fprintf(&builder, "  - ID=%d: %s（引用次数=%d，%s）\n", c.ID, c.Label, c.RefCount, strings.Join(evidence, "，"))
	}
	return builder.String()
}

// buildExpandCompositePrompt 渲染扩充×组合二分类裁决 prompt：判断共现组合
// 「值得创建组合标签且属于目标版块」（spec: 扩充方向的组合建议携带目标——
// target 由服务端注入，LLM 输出不含目标字段）。
func buildExpandCompositePrompt(profile *boardExpandProfile, candidates []ComposeCandidate, labels map[uint]string) string {
	var builder strings.Builder
	builder.WriteString("你是一个语义标签分析助手。\n\n")
	builder.WriteString(renderExpandProfilePrompt(profile))
	builder.WriteString("\n任务：判断下列高频共现标签组合是否值得创建为组合标签并挂载进该目标版块。\n")
	builder.WriteString("决策空间只有两种：compose（值得：创建组合标签并挂载进该版块）或 skip（不值得）。\n")
	builder.WriteString("判断原则：\n")
	builder.WriteString("- 组合有明确指向性主题（如「美国国债」+「收益率」=「美债收益率」）且与该版块主题一致 → compose（board_label=组合标签名、description=组合含义一句话、auxiliary_label_ids=组件 ID 列表）\n")
	builder.WriteString("- 组合无明确指向语义（如「日本」+「市场」）、或组合主题与该版块无关 → skip\n\n")
	builder.WriteString("返回 JSON 格式：{\"suggestions\": [{\"decision\": \"compose|skip\", \"board_label\": \"组合标签名\", \"description\": \"组合含义一句话\", \"auxiliary_label_ids\": [id1, id2], \"reason\": \"判断理由\"}]}\n\n")
	builder.WriteString("候选组合（与该版块相关，按共现文章数降序）：\n")
	for i, candidate := range candidates {
		parts := make([]string, 0, len(candidate.ComponentIDs))
		for _, id := range candidate.ComponentIDs {
			label := labels[id]
			if label == "" {
				label = fmt.Sprintf("#%d", id)
			}
			parts = append(parts, fmt.Sprintf("%s(ID:%d)", label, id))
		}
		fmt.Fprintf(&builder, "%d. [%s] 共现文章数 %d\n", i+1, strings.Join(parts, " × "), candidate.Cooccurrence)
	}
	return builder.String()
}
