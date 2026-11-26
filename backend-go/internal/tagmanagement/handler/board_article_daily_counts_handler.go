package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	tagmodels "syntopica-backend/internal/tagmanagement/models"
)

// 版块文章按日计数（archive-time-chart-range）：GET /semantic-boards/:id/article-daily-counts
//
// 口径不变量（口径漂移的唯一防线，改动前先读这三条）：
//  1. 成员口径与 getBoardArticles 主查询**逐字一致**：同一 boardTagIDs 取法
//     （topic_tag_board_labels.semantic_board_id，**不过滤标签 status**——列表主查询
//     也不过滤）、同 feeds 内连接、同 DATE(pub_date) 归日、COUNT(DISTINCT a.id) 去重
//     （一篇文章命中多个版块标签只计 1；列表侧用 SELECT DISTINCT，等价）。
//  2. 固定口径聚合：MUST NOT 接受或应用 feed_id / auxiliary_label_id / report_status /
//     show_direction_mismatch / 分页 / 日期窗口——传了也不解析、不返回 400（给未来留余地，
//     不是契约）。方向校验未通过的文章（列表行内默认隐藏其标签）**仍计入**当日 count，
//     以与归档列表「当天篇数」对齐。
//  3. 只读：全流程只 SELECT，不写库、不触发打标或匹配。

// boardArticleDailyCountDTO 是单日计数行（date=自然日，count=当日命中文章数）。
type boardArticleDailyCountDTO struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

// boardArticleDailyCountsDTO 是按日计数端点的响应体。
// days 为**稀疏**数组（只含 count>0 的日期，按 date 升序）；缺日由调用方按已知 0 处理，
// 端点 MUST NOT 补齐全区间零值。
type boardArticleDailyCountsDTO struct {
	EarliestDate *string                     `json:"earliest_date"`
	LatestDate   *string                     `json:"latest_date"`
	Days         []boardArticleDailyCountDTO `json:"days"`
}

// boardArticleDailyCountsSQL 是按日聚合的单条查询（一次扫描出全历史）。
//
// 日期列用 CAST(... AS VARCHAR(10)) 落成 10 字符文本：Postgres 的 DATE 列与 SQLite 的
// DATE() 表达式返回类型不同，统一转文本后两端拿到同一字符串口径（避免驱动差异）。
// GROUP BY 1 / ORDER BY 1 用输出序号，两端语义一致（Postgres 要求 select 表达式与分组
// 表达式逐字一致，用序号避开该限制）。
//
// `pub_date IS NOT NULL`：articles.pub_date 是可空列（存量行的回填是 2026-06-11 才加的，
// 不可证明全表非空）。NULL 分组会扫进 Day string 触发
// "converting NULL to string is unsupported" → 整个端点 500；而按日曲线本也无法给
// 无日期文章定位到某一天，直接排除（同仓先例 service/board/tag_clustering.go 的
// `AND a.pub_date IS NOT NULL`）。
const boardArticleDailyCountsSQL = `
SELECT CAST(DATE(a.pub_date) AS VARCHAR(10)) AS day, COUNT(DISTINCT a.id) AS cnt
FROM articles a
JOIN article_topic_tags att ON att.article_id = a.id AND att.topic_tag_id IN (?)
JOIN feeds f ON f.id = a.feed_id
WHERE a.pub_date IS NOT NULL
GROUP BY 1
ORDER BY 1`

// boardDailyCountRow 是聚合一行的扫描目标。
type boardDailyCountRow struct {
	Day   string `gorm:"column:day"`
	Count int    `gorm:"column:cnt"`
}

// loadBoardTagIDs 取版块挂载的 topic tag id 集合（成员口径的唯一实现）。
//
// 与 getBoardArticles 主查询共用同一份取法：**不过滤标签 status**（列表主查询也不
// 过滤——历史 disabled 标签命中的文章仍算版块成员），供按日计数端点复用，避免两处漂移。
func (h *semanticBoardHandler) loadBoardTagIDs(ctx context.Context, boardID uint) ([]uint, error) {
	var boardTagIDs []uint
	if err := h.db.WithContext(ctx).Model(&tagmodels.TopicTagBoardLabel{}).
		Select("topic_tag_id").
		Where("semantic_board_id = ?", boardID).
		Pluck("topic_tag_id", &boardTagIDs).Error; err != nil {
		return nil, err
	}
	return boardTagIDs, nil
}

// getBoardArticleDailyCounts 返回该版块命中文章按自然日的聚合数（只读）。
//
// 空标签集合（版块无标签 / 版块不存在）直接返回 {earliest_date:null, latest_date:null,
// days:[]} 且 HTTP 200——与 /articles 的空列表口径一致，不额外区分 404。
func (h *semanticBoardHandler) getBoardArticleDailyCounts(c *gin.Context) {
	boardID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	ctx := c.Request.Context()

	boardTagIDs, err := h.loadBoardTagIDs(ctx, boardID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	if len(boardTagIDs) == 0 {
		respondOK(c, boardArticleDailyCountsDTO{
			EarliestDate: nil,
			LatestDate:   nil,
			Days:         []boardArticleDailyCountDTO{},
		})
		return
	}

	var rows []boardDailyCountRow
	if err := h.db.WithContext(ctx).Raw(boardArticleDailyCountsSQL, boardTagIDs).Scan(&rows).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}

	days := make([]boardArticleDailyCountDTO, 0, len(rows))
	for _, row := range rows {
		days = append(days, boardArticleDailyCountDTO{Date: row.Day, Count: row.Count})
	}
	result := boardArticleDailyCountsDTO{Days: days}
	// earliest/latest 由结果首末行派生，不另发 MIN/MAX 查询（避免二次全扫）。
	if len(days) > 0 {
		earliest := days[0].Date
		latest := days[len(days)-1].Date
		result.EarliestDate = &earliest
		result.LatestDate = &latest
	}
	respondOK(c, result)
}
