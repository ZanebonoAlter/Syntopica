package boardhealth

import (
	"context"

	"gorm.io/gorm"

	tagmodels "syntopica-backend/internal/tagmanagement/models"
)

// UncapturedCandidates computes the board's factual label profile diff
// (事实画像 diff): auxiliary labels carried by tags ALREADY matched to the
// board (topic_tag_board_labels) but absent from the board composition,
// aggregated by member tag count and article coverage. Excludes disabled
// labels (status != 'active' on the join) and already-mounted labels (NOT
// EXISTS on board_composition). Pure read; ordered by member tags desc,
// capped at maxUncapturedCandidates (design D4: top 50).
//
// Evidence semantics: MemberTags = distinct matched tags carrying the label;
// MemberArticles = distinct articles referencing those tags
// (article_topic_tags). COUNT(DISTINCT) on both sides keeps the 1:N join
// honest.
func UncapturedCandidates(ctx context.Context, db *gorm.DB, boardID uint) ([]tagmodels.BoardHealthCandidate, error) {
	var rows []tagmodels.BoardHealthCandidate
	err := db.WithContext(ctx).Raw(`
		SELECT ttsl.semantic_label_id AS aux_id,
		       l.label AS label,
		       COUNT(DISTINCT ttsl.topic_tag_id) AS member_tags,
		       COUNT(DISTINCT att.article_id) AS member_articles
		FROM topic_tag_board_labels ttbl
		JOIN topic_tag_semantic_labels ttsl ON ttsl.topic_tag_id = ttbl.topic_tag_id
		JOIN semantic_labels l
		     ON l.id = ttsl.semantic_label_id
		    AND l.label_type = 'auxiliary'
		    AND l.status = 'active'
		LEFT JOIN article_topic_tags att ON att.topic_tag_id = ttsl.topic_tag_id
		WHERE ttbl.semantic_board_id = ?
		  AND NOT EXISTS (
			SELECT 1 FROM board_composition bc
			WHERE bc.board_id = ? AND bc.auxiliary_label_id = ttsl.semantic_label_id
		  )
		GROUP BY ttsl.semantic_label_id, l.label
		ORDER BY member_tags DESC, member_articles DESC, aux_id ASC
		LIMIT ?`, boardID, boardID, maxUncapturedCandidates).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}
