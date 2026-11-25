package auxlabel

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
	"syntopica-backend/internal/tagmanagement/service/core"
)

// --- fix-aux-label-idle-tx-cache design D1：两阶段拆分白盒 ---

// recordingBatchEmbedder 记录每次批量调用的输入与模式，供调用次数断言。
type recordingBatchEmbedder struct {
	mu     sync.Mutex
	calls  [][]string
	modes  []AuxiliaryLabelEmbeddingMode
	failer bool
}

func (e *recordingBatchEmbedder) embed(ctx context.Context, inputs []string, mode AuxiliaryLabelEmbeddingMode) ([]string, [][]float64, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls = append(e.calls, inputs)
	e.modes = append(e.modes, mode)
	if e.failer {
		return nil, nil, errors.New("batch embedder forced failure")
	}
	pgVectors := make([]string, len(inputs))
	vectors := make([][]float64, len(inputs))
	for i := range inputs {
		padded := testutil.PadVector([]float64{1, 0, 0}, testutil.TestEmbeddingDim)
		pgVectors[i], vectors[i] = core.FloatsToPgVector(padded), padded
	}
	return pgVectors, vectors, nil
}

// 主链路步1：attach 3 个全新标签 → 批量 embedder 恰好 2 次调用
// （merge 批 1 + storage 批 1），非逐标签 6 次。
func TestAttachTwoPhaseBatchEmbedderExactlyTwoCalls(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	tag := models.TopicTag{Label: "两阶段标签", Slug: "two-phase", Category: "event", Status: "active"}
	require.NoError(t, db.Create(&tag).Error)
	batch := &recordingBatchEmbedder{}
	service := NewAuxiliaryLabelService(db, (&recordingAuxiliaryEmbedder{}).embed)
	service.SetBatchEmbedder(batch.embed)

	labels := []core.AuxiliaryLabel{
		{Label: "批量标签甲", Description: "描述一"},
		{Label: "批量标签乙", Description: "描述二"},
		{Label: "批量标签丙", Description: "描述三"},
	}
	require.NoError(t, service.AttachAuxiliaryLabels(context.Background(), tag.ID, labels))

	batch.mu.Lock()
	defer batch.mu.Unlock()
	require.Len(t, batch.calls, 2, "3 new labels must cost exactly 2 batch calls (merge + storage), not per-label calls")
	require.Equal(t, AuxiliaryLabelEmbeddingModeMerge, batch.modes[0])
	require.Equal(t, AuxiliaryLabelEmbeddingModeStorage, batch.modes[1])
	require.Equal(t, []string{"批量标签甲", "批量标签乙", "批量标签丙"}, batch.calls[0])
	require.Equal(t, []string{"批量标签甲: 描述一", "批量标签乙: 描述二", "批量标签丙: 描述三"}, batch.calls[1])

	var created int64
	require.NoError(t, db.Model(&models.SemanticLabel{}).Where("label_type = ?", "auxiliary").Count(&created).Error)
	require.Equal(t, int64(3), created)
	var links int64
	require.NoError(t, db.Model(&tagmodels.TopicTagSemanticLabel{}).Where("topic_tag_id = ?", tag.ID).Count(&links).Error)
	require.Equal(t, int64(3), links)
}

// 主链路步2：阶段一（L1 匹配 / L2 召回）与阶段二（纯写事务）都不得在事务开着时调用
// embedder。判定方式（fix-aux-label-dedup-ann D6 后调整）：阶段一多了一个只读召回
// 事务，旧探针「首个 BEGIN 之后一律算违规」的单调近似会把「召回事务关闭后的 storage
// 批嵌入」误报成违规。改为在每次 embedder 调用时刻查 pg_stat_activity——有任何
// idle in transaction 会话即违规（事务开着等外部调用正是 idle-in-transaction 病灶）。
//
// 探针条件用 state LIKE 'idle in transaction%'：回滚后的会话停在
// 'idle in transaction (aborted)'，精确等值匹配会漏掉它。
func TestAttachTwoPhaseNoEmbedderInsideTransaction(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	tag := models.TopicTag{Label: "探针标签", Slug: "probe", Category: "event", Status: "active"}
	require.NoError(t, db.Create(&tag).Error)

	var mu sync.Mutex
	var violations []string
	// noteEmbedderCall 记录「此刻是否有事务未提交」（判定「embedder 在事务内」）。
	noteEmbedderCall := func(name string) {
		t.Helper()
		var open int64
		require.NoError(t, db.Raw(`SELECT count(*) FROM pg_stat_activity WHERE state LIKE 'idle in transaction%'`).Scan(&open).Error)
		if open > 0 {
			mu.Lock()
			violations = append(violations, name)
			mu.Unlock()
		}
	}

	batch := func(ctx context.Context, inputs []string, mode AuxiliaryLabelEmbeddingMode) ([]string, [][]float64, error) {
		noteEmbedderCall("batch-embedder")
		return (&recordingBatchEmbedder{}).embed(ctx, inputs, mode)
	}
	service := NewAuxiliaryLabelService(db, (&recordingAuxiliaryEmbedder{}).embed)
	service.SetBatchEmbedder(batch)

	require.NoError(t, service.AttachAuxiliaryLabels(context.Background(), tag.ID, []core.AuxiliaryLabel{
		{Label: "事务外甲", Description: "先算向量"},
		{Label: "事务外乙", Description: "再进事务"},
	}))
	require.Empty(t, violations, "no embedder call may happen while a transaction is open (idle-in-transaction root cause)")

	// 探针自检：事务开着时调用确实会被记录（防止探针失灵导致的假绿）。
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		noteEmbedderCall("self-check-in-tx")
		return nil
	}))
	require.Contains(t, violations, "self-check-in-tx")
}

// txBeginCounter 包裹 ConnPool，统计事务开启次数并区分只读 / 读写。
//
// GORM 的 db.Transaction() 直接走 ConnPool.BeginTx（不经过任何回调处理器），所以
// 「某个阶段确实开过事务」只能用连接池层的探针钉住（review R-3）。
type txBeginCounter struct {
	gorm.ConnPool
	mu        sync.Mutex
	begins    int
	readOnly  int
	readWrite int
}

func (p *txBeginCounter) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	tx, err := p.ConnPool.(gorm.TxBeginner).BeginTx(ctx, opts)
	if err == nil {
		p.mu.Lock()
		p.begins++
		if opts != nil && opts.ReadOnly {
			p.readOnly++
		} else {
			p.readWrite++
		}
		p.mu.Unlock()
	}
	return tx, err
}

func (p *txBeginCounter) counts() (begins, readOnly, readWrite int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.begins, p.readOnly, p.readWrite
}

// 主链路步2b：阶段二**确实开过事务**（review R-3 补回的断言）。
//
// attach 的事务拓扑固定为两个：阶段一的 L2 召回只读事务（ReadOnly: true） +
// 阶段二的纯写事务。若阶段二不再开事务，GORM 会退化为「每条语句一个隐式事务」
// （本仓未开 SkipDefaultTransaction），读写事务数会从 1 涨到语句数——所以
// 断言「读写事务恰好 1 个」同时钉住了阶段二的原子性。
func TestAttachTwoPhaseOpensRecallAndWriteTransactions(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	tag := models.TopicTag{Label: "事务拓扑", Slug: "tx-shape", Category: "event", Status: "active"}
	require.NoError(t, db.Create(&tag).Error)

	// 独立连接句柄：包裹它的连接池才能观测到 attach 自己开的事务。
	probeDB := testutil.NewIsolatedDB(t)
	counter := &txBeginCounter{ConnPool: probeDB.ConnPool}
	probeDB.ConnPool = counter
	probeDB.Statement.ConnPool = counter

	service := NewAuxiliaryLabelService(probeDB, (&recordingAuxiliaryEmbedder{}).embed)
	service.SetBatchEmbedder((&recordingBatchEmbedder{}).embed)

	require.NoError(t, service.AttachAuxiliaryLabels(context.Background(), tag.ID, []core.AuxiliaryLabel{
		{Label: "拓扑甲", Description: "走召回"},
		{Label: "拓扑乙", Description: "走召回"},
	}))

	begins, readOnly, readWrite := counter.counts()
	require.Equal(t, 2, begins, "phase one opens the read-only recall transaction, phase two opens the write transaction")
	require.Equal(t, 1, readOnly, "the L2 recall must run in exactly one read-only transaction")
	require.Equal(t, 1, readWrite, "phase two must wrap its writes in exactly one read-write transaction")
}

// 主链路步9：全 L1 命中的 attach（既有标签）→ 零 embedder 调用、零缓存重拉。
func TestAttachAllL1HitsZeroEmbedderCalls(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	tag := models.TopicTag{Label: "复用标签", Slug: "reuse", Category: "event", Status: "active"}
	require.NoError(t, db.Create(&tag).Error)
	existing := models.SemanticLabel{Label: "既有甲", Slug: "jiyou-a", LabelType: "auxiliary", Status: "active"}
	require.NoError(t, db.Create(&existing).Error)
	batch := &recordingBatchEmbedder{}
	service := NewAuxiliaryLabelService(db, (&recordingAuxiliaryEmbedder{}).embed)
	service.SetBatchEmbedder(batch.embed)

	require.NoError(t, service.AttachAuxiliaryLabels(context.Background(), tag.ID, []core.AuxiliaryLabel{
		{Label: "既有甲", Description: "L1 slug 命中"},
	}))

	batch.mu.Lock()
	require.Empty(t, batch.calls, "L1-hit attach must not call the embedder at all")
	batch.mu.Unlock()
	var labels int64
	require.NoError(t, db.Model(&models.SemanticLabel{}).Where("label_type = ?", "auxiliary").Count(&labels).Error)
	require.Equal(t, int64(1), labels)
}

// 变体·批内重复 label：同 slug / 同 normalizeKey 的重复输入共享首个决策——
// 一行、一条 link、ref_count 恒 1（旧串行路径靠缓存回灌 L1 复命中达到同一结果）。
func TestAttachDuplicateLabelsInBatchShareOutcome(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	tag := models.TopicTag{Label: "重复标签", Slug: "dup", Category: "event", Status: "active"}
	require.NoError(t, db.Create(&tag).Error)
	batch := &recordingBatchEmbedder{}
	service := NewAuxiliaryLabelService(db, (&recordingAuxiliaryEmbedder{}).embed)
	service.SetBatchEmbedder(batch.embed)

	// 「SK 重复」与「SK重复」slug 不同（Slugify 保留单空格）但 normalizeKey 相同；
	// 第三个是完全相同的文本。
	require.NoError(t, service.AttachAuxiliaryLabels(context.Background(), tag.ID, []core.AuxiliaryLabel{
		{Label: "SK 重复", Description: ""},
		{Label: "SK重复", Description: ""},
		{Label: "SK 重复", Description: ""},
	}))

	batch.mu.Lock()
	require.Len(t, batch.calls, 2, "dedup happens in phase one; only the first occurrence reaches the embedder")
	require.Len(t, batch.calls[0], 1, "duplicate labels must be collapsed before the batch call")
	batch.mu.Unlock()

	var labels []models.SemanticLabel
	require.NoError(t, db.Where("label_type = ?", "auxiliary").Find(&labels).Error)
	require.Len(t, labels, 1, "batch-internal duplicates must share one row")
	var links int64
	require.NoError(t, db.Model(&tagmodels.TopicTagSemanticLabel{}).Where("topic_tag_id = ?", tag.ID).Count(&links).Error)
	require.Equal(t, int64(1), links)
	require.Equal(t, 1, labels[0].RefCount, "OnConflict DoNothing keeps ref_count at 1 for duplicates")
}

// 白盒·批量 embedder 失败：错误冒泡、无半成品写入。
func TestAttachBatchEmbedderFailureBubblesClean(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	tag := models.TopicTag{Label: "失败标签", Slug: "fail", Category: "event", Status: "active"}
	require.NoError(t, db.Create(&tag).Error)
	batch := &recordingBatchEmbedder{failer: true}
	service := NewAuxiliaryLabelService(db, (&recordingAuxiliaryEmbedder{}).embed)
	service.SetBatchEmbedder(batch.embed)

	err := service.AttachAuxiliaryLabels(context.Background(), tag.ID, []core.AuxiliaryLabel{
		{Label: "失败甲", Description: ""},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "batch embedder forced failure")

	var labels int64
	require.NoError(t, db.Model(&models.SemanticLabel{}).Where("label_type = ?", "auxiliary").Count(&labels).Error)
	require.Zero(t, labels, "phase-one failure must leave zero rows behind")
	var links int64
	require.NoError(t, db.Model(&tagmodels.TopicTagSemanticLabel{}).Where("topic_tag_id = ?", tag.ID).Count(&links).Error)
	require.Zero(t, links)
}

// 边界·批大小切片：chunkStrings 是纯函数，直接断言切分形状。
func TestChunkStringsSplitsBySize(t *testing.T) {
	require.Equal(t, [][]string{{"a"}}, chunkStrings([]string{"a"}, 32))
	five := []string{"1", "2", "3", "4", "5"}
	require.Equal(t, [][]string{five}, chunkStrings(five, 5))
	require.Equal(t, [][]string{{"1", "2", "3"}, {"4", "5"}}, chunkStrings(five, 3))
	require.Equal(t, [][]string{{"1"}, {"2"}, {"3"}, {"4"}, {"5"}}, chunkStrings(five, 1))
}
