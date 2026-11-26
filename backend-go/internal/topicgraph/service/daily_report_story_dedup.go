package service

import (
	"fmt"
	"time"

	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/topicgraph/repository"
)

// Cross-period story dedup — add-event-story-heat tasks 6.1-6.3 (batch 4a).
//
// Runs AFTER attribution, synchronously before online highlights (or inside
// the compatibility detached runner): every attributed thread is compared with
// the story's body-rendered history. 展示层 ONLY —— 不删行、不删 signals、
// 不改 story_id、热度永不读 display_tier（红线：降级不改变热量）。
//
// 判定口径（specs/daily-report-system 跨期重复降级 / design §7）：
//   - 重述 → flash：overlap = |current ∩ history| / |current| ≥ 0.6 且
//     未出现新 feed（currentFeeds ⊆ historicalFeeds）。current 为空 →
//     overlap 记 0（无证据不算重述，宁进正文不误降级）。
//   - 进展 → body：is_development=true（当事方新动作优先级最高）、或
//     overlap < 0.6、或出现新 feed，三者任一成立。
//   - 不参与 → 保持 body：story_id 为空（未归组/回填未及）、scope=
//     composite（综合稿无事件身份）、story_manual=true（人工指派不受
//     自动去重干预）。
//
// 历史口径：同 story 最近 7 天（含当天、不含当前 report）display_tier=
// 'body' 的 threads 的 related_article_ids 并集——只有真进过正文的才算
// 历史；本期 flash 线不进历史（下期重述按当时正文集合重新对比）。feed
// 集合包含归档文章——归档只是轮转出 feed 活跃窗口，不是撤回（sourcestats
// 红线：窗口查询不得按 archived 过滤）。
//
// 幂等：verdict 是当前集合的纯函数——重复执行结果一致；已 flash 的若
// 条件变化（如新 feed 补入）会在下次重算时恢复 body。
// 已在 body 且判 body 的行不写库（避免无谓 UPDATE）。

const (
	// storyDedupWindowDays 历史窗口：最近 7 天（含当天，不含当前 report）。
	storyDedupWindowDays = 7
	// storyDedupOverlapThreshold 重述阈值：|current ∩ history| / |current|。
	storyDedupOverlapThreshold = 0.6
)

// StoryDedupResult counts the verdicts of one dedup run. Flash/Body count
// PARTICIPATING threads only; Skipped counts story_id-NULL / composite /
// manual threads (they keep body and are never written). Hidden is a
// reserved tier — the automatic verdict never sets it, the counter stays 0
// until a future manual-hide path exists.
type StoryDedupResult struct {
	Body    int
	Flash   int
	Hidden  int
	Skipped int
}

// applyStoryDedupForReport recomputes the display tier of every dedup-
// participating thread of the report and batch-writes the verdicts.
func applyStoryDedupForReport(reportID uint, periodDate time.Time) (StoryDedupResult, error) {
	result := StoryDedupResult{}
	repo := repository.Repo

	threads, err := repo.ListReportThreadsForDedup(reportID)
	if err != nil {
		return result, fmt.Errorf("story-dedup: load threads: %w", err)
	}
	participants := make([]repository.DailyReportThread, 0, len(threads))
	for _, thread := range threads {
		if thread.StoryID == nil || thread.Scope == repository.ThreadScopeComposite || thread.StoryManual {
			result.Skipped++ // 未归组/综合稿/人工指派：保持 body，不查历史
			continue
		}
		participants = append(participants, thread)
	}
	if len(participants) == 0 {
		return result, nil
	}

	// 历史集合：同 story 最近 7 天 body threads（不含当前 report）。
	storyIDs := make([]uint, 0, len(participants))
	seenStory := make(map[uint]bool, len(participants))
	articleIDs := make([]uint, 0, 64)
	for i := range participants {
		id := *participants[i].StoryID
		if !seenStory[id] {
			seenStory[id] = true
			storyIDs = append(storyIDs, id)
		}
		articleIDs = appendUintSet(articleIDs, jsonUintIDsExported(participants[i].RelatedArticleIDs))
	}
	day := repository.NormalizeReportDate(periodDate)
	// [day-6, day+1) = day-6..day 恰好 7 个日历日（含当天，不含当前
	// report）——storyDedupWindowDays 含当天计数，起点须减 (n-1) 而非 n。
	history, err := repo.ListStoryBodyHistoryThreads(storyIDs, reportID,
		day.AddDate(0, 0, -(storyDedupWindowDays-1)), day.AddDate(0, 0, 1))
	if err != nil {
		return result, fmt.Errorf("story-dedup: load history: %w", err)
	}
	for i := range history {
		if history[i].StoryID == nil {
			continue
		}
		articleIDs = appendUintSet(articleIDs, jsonUintIDsExported(history[i].RelatedArticleIDs))
	}

	// feed 映射一次查全（当前 + 历史文章，含归档——归档=轮转非撤回，
	// ArticleFeeds 不按 archived 过滤）。
	feeds, err := repo.ArticleFeeds(articleIDs)
	if err != nil {
		return result, fmt.Errorf("story-dedup: load article feeds: %w", err)
	}

	histArticles := make(map[uint]map[uint]bool, len(storyIDs))
	histFeeds := make(map[uint]map[uint]bool, len(storyIDs))
	for i := range history {
		storyID := *history[i].StoryID
		articles, ok := histArticles[storyID]
		if !ok {
			articles = make(map[uint]bool)
			histArticles[storyID] = articles
			histFeeds[storyID] = make(map[uint]bool)
		}
		for _, id := range jsonUintIDsExported(history[i].RelatedArticleIDs) {
			articles[id] = true
			if feedID, visible := feeds[id]; visible {
				histFeeds[storyID][feedID] = true
			}
		}
	}

	toFlash := make([]uint, 0)
	toBody := make([]uint, 0)
	for i := range participants {
		thread := participants[i]
		current := make(map[uint]bool)
		for _, id := range jsonUintIDsExported(thread.RelatedArticleIDs) {
			current[id] = true
		}
		currentFeeds := make(map[uint]bool, len(current))
		for id := range current {
			if feedID, visible := feeds[id]; visible {
				currentFeeds[feedID] = true
			}
		}

		// overlap 口径：|current ∩ history| / |current|；current 为空记 0。
		overlap := 0.0
		if len(current) > 0 {
			shared := 0
			for id := range current {
				if histArticles[*thread.StoryID][id] {
					shared++
				}
			}
			overlap = float64(shared) / float64(len(current))
		}
		newFeed := false
		for feedID := range currentFeeds {
			if !histFeeds[*thread.StoryID][feedID] {
				newFeed = true
				break
			}
		}

		tier := repository.ThreadDisplayTierBody
		if !thread.IsDevelopment && overlap >= storyDedupOverlapThreshold && !newFeed {
			tier = repository.ThreadDisplayTierFlash
		}
		switch tier {
		case repository.ThreadDisplayTierFlash:
			result.Flash++
		default:
			result.Body++
		}
		if tier != thread.DisplayTier {
			if tier == repository.ThreadDisplayTierFlash {
				toFlash = append(toFlash, thread.ID)
			} else {
				toBody = append(toBody, thread.ID)
			}
		}
	}

	if err := repo.SetThreadsDisplayTier(toFlash, repository.ThreadDisplayTierFlash); err != nil {
		return result, fmt.Errorf("story-dedup: write flash verdicts: %w", err)
	}
	if err := repo.SetThreadsDisplayTier(toBody, repository.ThreadDisplayTierBody); err != nil {
		return result, fmt.Errorf("story-dedup: write body verdicts: %w", err)
	}
	return result, nil
}

// appendUintSet appends ids not yet present (keeps the union list deduped so
// ArticleFeeds gets one compact IN query).
func appendUintSet(list []uint, ids []uint) []uint {
	seen := make(map[uint]bool, len(list))
	for _, id := range list {
		seen[id] = true
	}
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			list = append(list, id)
		}
	}
	return list
}

// runStoryDedupSafe is the wiring half invoked by attributeStoriesSafe right
// after attribution: same detached goroutine, same recover contract. Failure
// is a warning only — the report result is never affected (display layer).
func runStoryDedupSafe(boardID, reportID uint, periodDate time.Time) {
	if result, err := applyStoryDedupForReport(reportID, periodDate); err != nil {
		logging.Warnf("story-dedup: dedup for board %d report %d failed: %v", boardID, reportID, err)
	} else {
		logging.Infof("story-dedup: report %d body=%d flash=%d skipped=%d", reportID, result.Body, result.Flash, result.Skipped)
	}
}
