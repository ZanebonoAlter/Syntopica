package repository

import (
	"context"

	"gorm.io/gorm"
)

// Repo is the package-level repository singleton.
var Repo *TopicGraphRepository

// InitRepository creates and stores the global repository using the given DB connection.
// Must be called once at startup before any handlers use Repo.
func InitRepository(db *gorm.DB) {
	Repo = NewTopicGraphRepository(db)
}

// TopicGraphRepository provides data access methods for topic graph, daily reports,
// and related entities. It replaces direct database.DB access.
type TopicGraphRepository struct {
	db *gorm.DB
	// rebuildRelations rebuilds a board's relation graph inside the caller's
	// transaction. It is a field (not a package global) so a test can inject a
	// failing rebuild and assert that the whole membership-change transaction
	// rolls back (lane-manual-correction「关系重建失败整体回滚」).
	rebuildRelations func(tx *gorm.DB, boardID uint) error
	// laneDynamics overrides the lane-dynamics fetch. It is a field (not a
	// package global) so a test can inject a failure and assert the radar
	// overview's「泳道动态计算失败」降级行（该路径此前没有降级标记，会把
	// 降级渲染成「0 关注度」的业务结论）.
	//
	// nil = 走默认实现（GetBoardLaneDynamics）。默认实现**不**在构造期绑成
	// 闭包：雷达路径要让请求 ctx 跟着查询走（预算到期即取消在飞的查询，见
	// withContext），而 ctx 挂在 db 句柄上，构造期闭包拿不到当次 ctx。注入覆盖
	// 保持原签名（boardID, days）——既有测试一行都不用改。
	laneDynamics func(boardID uint, days int) (*LaneDynamicsResponse, error)
}

// NewTopicGraphRepository creates a new repository with the given DB connection.
func NewTopicGraphRepository(db *gorm.DB) *TopicGraphRepository {
	repo := &TopicGraphRepository{db: db, rebuildRelations: RebuildBoardRelations}
	return repo
}

// withContext returns a shallow copy of the repository whose database handle
// carries ctx. Every query built from that handle inherits the context, so a
// cancelled / expired request stops its in-flight queries instead of letting
// them run to completion while still holding a pooled connection —— 雷达总览
// 5 秒预算的关键：预算到期后在飞的查询立刻取消、连接马上归还池子。
func (r *TopicGraphRepository) withContext(ctx context.Context) *TopicGraphRepository {
	if ctx == nil {
		return r
	}
	clone := *r
	clone.db = r.db.WithContext(ctx)
	return &clone
}

// DB returns the underlying *gorm.DB for ad-hoc queries.
func (r *TopicGraphRepository) DB() *gorm.DB {
	return r.db
}
