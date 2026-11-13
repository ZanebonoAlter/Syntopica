package repository

import (
	"encoding/json"

	"syntopica-backend/internal/platform/textutil"
)

// FoldThreadArticleCopies 把线索的 related_article_ids 里「同一篇内容的重复
// 副本」折叠掉，只留首个出现（保序）。
//
// 折叠键 = 文章内容身份 = textutil.NormalizeArticleLink(link)（去 fragment /
// 去 query / scheme+host 小写）——与 flow/daily-report 红线 49 的内容身份键
// 是同一实现（同一归一规则，不另立口径）。红线 49 的现存服务面只有「簇内
// 文章数」与 related-events 两处既有统计；story 线（story_signals /
// story_heat）已按红线 43 整体退役，不再服务内容身份。同一个 id 在数组里
// 出现两次也算重复引用，保留首个。
//
// 保守口径（design D2）：只认「归一 link 相同」这一条可判定信号——同标题不同
// URL 的多源转载无法判定为同一内容，一律保留。归一 link 为空（link 空/纯空白）
// 或该 id 在 link 映射里查不到时内容身份不可判定 → 原样保留，不折叠、不删元素。
//
// 纯函数、无 IO：link 由调用方批量取好（design D4/D5），单测不需要数据库。
// 入参切片不被修改，返回新切片——调用方手里的原值仍是落库口径（红线 45）。
func FoldThreadArticleCopies(threads []DailyReportThread, linkByArticleID map[uint]string) []DailyReportThread {
	if len(threads) == 0 {
		return threads
	}
	folded := make([]DailyReportThread, len(threads))
	copy(folded, threads)
	for i := range folded {
		folded[i].RelatedArticleIDs = foldArticleIDList(folded[i].RelatedArticleIDs, linkByArticleID)
	}
	return folded
}

// foldArticleIDList 折叠单条线索的 related_article_ids（jsonb 数组）。
//
// 解析失败 / 不是数组 / 历史 null 标量 → 原样返回（不报错、不删元素）。
// 没有任何副本被折叠时也原样返回字节，避免无意义地重写 jsonb 文本。
func foldArticleIDList(raw JSON, linkByArticleID map[uint]string) JSON {
	ids, ok := decodeArticleIDArray(raw)
	if !ok {
		return raw
	}
	kept := make([]uint, 0, len(ids))
	seenIDs := make(map[uint]bool, len(ids))
	seenKeys := make(map[string]bool, len(ids))
	folded := false
	for _, id := range ids {
		// 同一个 id 重复出现 = 同一行文章被引用两次，保留首个。
		if seenIDs[id] {
			folded = true
			continue
		}
		key, decidable := contentIdentityKey(id, linkByArticleID)
		if decidable && seenKeys[key] {
			// 同内容副本（跨 feed 各抓一份 / 归一后同 link）：丢弃副本，不改指 keeper。
			folded = true
			continue
		}
		seenIDs[id] = true
		if decidable {
			seenKeys[key] = true
		}
		kept = append(kept, id)
	}
	if !folded {
		return raw
	}
	encoded, err := json.Marshal(kept)
	if err != nil {
		return raw
	}
	return JSON(encoded)
}

// decodeArticleIDArray 解析 related_article_ids（jsonb 数组）。解析失败、不是
// 数组、或历史行里的 JSON null 标量时返回 ok=false，调用方原样保留。
func decodeArticleIDArray(raw JSON) ([]uint, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	var ids []uint
	if err := json.Unmarshal(raw, &ids); err != nil {
		return nil, false
	}
	if ids == nil {
		return nil, false
	}
	return ids, true
}

// contentIdentityKey 返回文章 id 的内容身份键。ok=false 表示身份不可判定
// （link 查不到或归一后为空），此时该 id 不参与折叠、原样保留。
func contentIdentityKey(id uint, linkByArticleID map[uint]string) (string, bool) {
	link, found := linkByArticleID[id]
	if !found {
		return "", false
	}
	key := textutil.NormalizeArticleLink(link)
	if key == "" {
		return "", false
	}
	return key, true
}
