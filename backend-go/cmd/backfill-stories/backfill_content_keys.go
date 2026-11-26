package main

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/textutil"
	"syntopica-backend/internal/topicgraph/repository"
)

// contentKeyBatchSize 限定一轮扫描 + 写入的行数（单批事务的规模）。
// 树莓派上 500 行一批足够快，又不会让单个事务长时间占着表锁。
const contentKeyBatchSize = 500

type contentKeyStats struct {
	// Scanned 扫过的 signal 行数（含文章已删、join 不到的行不算在内）。
	Scanned int64
	// Updated 内容身份键与现值不同、因而被写（dry-run 时为「将被写」）的行数。
	Updated int64
	// Stories 重算过的事件线数（证据重写 + 参与者计数回落）。
	Stories int
}

// contentKeyRow 是回填扫描的一行：signal 主键 + 原文 link + 现值 content_key。
type contentKeyRow struct {
	StoryID    uint    `gorm:"column:story_id"`
	ArticleID  uint    `gorm:"column:article_id"`
	Link       string  `gorm:"column:link"`
	ContentKey *string `gorm:"column:content_key"`
}

// backfillContentKeys 回填 story_signals.content_key（fix-duplicate-article-heat
// design D5）：按 articles.link 规范化后写入内容身份键，并在回填收尾把受影响
// 事件线的参与者计数重算一遍。
//
// 分批扫描（每批 ≤500 行）、只更新与现值不同的行 → 幂等（重跑第二次
// updated=0）；dry-run 只统计不写库。
//
// 收尾重算（回填的另一半）：只写 content_key 不重算 participant_count 时，
// 来源数仍按旧值显示——同一篇内容被多个 feed 各抓一份仍算多个来源
// （同一 link 的副本不再合并）。所以回填后逐条重算受影响事件线：
// RecomputeSignalsForStory 按事件线的 thread 并集重写证据（content_key
// 同源产出）并重算 participant_count（幂等）。
//
// 另有一笔存量欠账：更早的回填版本只写 content_key、不重算计数，计数仍停
// 在按 feed 的旧值上。这里按同一口径（内容身份优先、feed 兜底）比对「存储的
// participant_count」与「按证据重算的值」，对不上的事件线一并重算——否则
// 第二次运行 updated=0 却把虚高的来源数留在库里。
//
// 与 --rebuild-signals 正交：rebuild 整线重写 signals（自然带上 content_key），
// 本回填负责 rebuild 范围之外的历史行。
func backfillContentKeys(ctx context.Context, db *gorm.DB, opts commandOptions) (contentKeyStats, error) {
	stats := contentKeyStats{}
	db = db.WithContext(ctx)
	repo := repository.NewTopicGraphRepository(db)

	affected, err := backfillContentKeyRows(ctx, db, opts, &stats)
	if err != nil {
		return stats, err
	}
	stale, err := staleParticipantStories(db, repo)
	if err != nil {
		return stats, err
	}

	// 重算清单 = 本次回填写过内容身份键的事件线 + 计数与证据对不上的存量线。
	// 顺序稳定（先 affected 后 stale），同一事件线只算一次。
	targets := make([]uint, 0, len(affected)+len(stale))
	seen := make(map[uint]bool, len(affected)+len(stale))
	for _, ids := range [][]uint{affected, stale} {
		for _, id := range ids {
			if seen[id] {
				continue
			}
			seen[id] = true
			targets = append(targets, id)
		}
	}

	var failures []error
	for _, storyID := range targets {
		if opts.DryRun {
			stats.Stories++
			continue
		}
		if err := repo.RecomputeSignalsForStory(storyID); err != nil {
			// 单条失败不打断整轮回填：记 warn 后继续，最后聚合返回。
			logging.Warnf("backfill content keys: 重算事件线 %d 失败: %v", storyID, err)
			failures = append(failures, fmt.Errorf("recompute story %d: %w", storyID, err))
			continue
		}
		stats.Stories++
	}
	if len(failures) > 0 {
		return stats, fmt.Errorf("backfill content keys: %d 条事件线重算失败: %w", len(failures), errors.Join(failures...))
	}
	return stats, nil
}

// backfillContentKeyRows 分批回填 story_signals.content_key，返回本轮被改写
// 过的事件线 id（去重、保序）。
func backfillContentKeyRows(ctx context.Context, db *gorm.DB, opts commandOptions, stats *contentKeyStats) ([]uint, error) {
	db = db.WithContext(ctx)
	affected := make([]uint, 0, 64)
	seen := make(map[uint]bool, 64)
	// keyset 分页：按 (story_id, article_id) 递增推进，不用 OFFSET（大表上
	// OFFSET 会越翻越慢）。
	var lastStory, lastArticle uint
	for {
		var rows []contentKeyRow
		if err := db.Raw(`
			SELECT s.story_id, s.article_id, a.link, s.content_key
			FROM story_signals s
			JOIN articles a ON a.id = s.article_id
			WHERE s.story_id > ? OR (s.story_id = ? AND s.article_id > ?)
			ORDER BY s.story_id, s.article_id
			LIMIT ?`, lastStory, lastStory, lastArticle, contentKeyBatchSize).Scan(&rows).Error; err != nil {
			return affected, fmt.Errorf("scan story_signals content keys: %w", err)
		}
		if len(rows) == 0 {
			break
		}
		stats.Scanned += int64(len(rows))
		type contentKeyUpdate struct {
			storyID    uint
			articleID  uint
			contentKey string
		}
		updates := make([]contentKeyUpdate, 0, len(rows))
		for _, row := range rows {
			want := textutil.NormalizeArticleLink(row.Link)
			// 现值为 NULL 与空串都算「尚未回填」：只有与目标值不同的行才写。
			if row.ContentKey != nil && *row.ContentKey == want {
				continue
			}
			updates = append(updates, contentKeyUpdate{storyID: row.StoryID, articleID: row.ArticleID, contentKey: want})
		}
		if len(updates) > 0 && !opts.DryRun {
			if err := db.Transaction(func(tx *gorm.DB) error {
				for _, u := range updates {
					if err := tx.Exec(`UPDATE story_signals SET content_key = ?
						WHERE story_id = ? AND article_id = ?`, u.contentKey, u.storyID, u.articleID).Error; err != nil {
						return fmt.Errorf("update story_signals (%d, %d): %w", u.storyID, u.articleID, err)
					}
				}
				return nil
			}); err != nil {
				return affected, err
			}
		}
		stats.Updated += int64(len(updates))
		for _, u := range updates {
			if seen[u.storyID] {
				continue
			}
			seen[u.storyID] = true
			affected = append(affected, u.storyID)
		}
		lastStory, lastArticle = rows[len(rows)-1].StoryID, rows[len(rows)-1].ArticleID
	}
	return affected, nil
}

// staleParticipantStories 找出「存储的 participant_count 与按证据重算的值
// 对不上」的事件线。
//
// 这笔欠账来自更早的回填版本：只写 content_key、不重算计数，来源数仍停在
// 按 feed 的旧值上（同一 link 的多个 feed 副本各算一个来源）。比对口径直接
// 用仓储层的同一份实现（内容身份优先、feed 兜底），不在本包另写一份公式。
//
// 扫描按 id 分批推进（不把全表事件线一次读进内存）；比对为只读，dry-run
// 也能跑。
func staleParticipantStories(db *gorm.DB, repo *repository.TopicGraphRepository) ([]uint, error) {
	type storyCountRow struct {
		StoryID uint  `gorm:"column:story_id"`
		Stored  int64 `gorm:"column:stored"`
	}
	var stale []uint
	var lastID uint
	for {
		var rows []storyCountRow
		if err := db.Raw(`
			SELECT b.id AS story_id, b.participant_count AS stored
			FROM board_stories b
			WHERE b.id > ?
			ORDER BY b.id
			LIMIT ?`, lastID, contentKeyBatchSize).Scan(&rows).Error; err != nil {
			return nil, fmt.Errorf("scan story participant counts: %w", err)
		}
		if len(rows) == 0 {
			break
		}
		for _, row := range rows {
			want, err := repo.StoryParticipantCount(row.StoryID)
			if err != nil {
				return nil, fmt.Errorf("recompute story %d participants: %w", row.StoryID, err)
			}
			if want != row.Stored {
				stale = append(stale, row.StoryID)
			}
		}
		lastID = rows[len(rows)-1].StoryID
	}
	return stale, nil
}
