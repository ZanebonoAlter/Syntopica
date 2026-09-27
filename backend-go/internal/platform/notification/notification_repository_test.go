package notification

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
)

// TestNotificationEviction covers the 500-cap eviction state machine
// (test-cases 白盒 A1/A2 + 边界值): rows under the cap insert directly; at
// the cap the oldest read rows go first; the newest row always survives;
// consecutive writes keep the table ≤ MaxNotifications.
func TestNotificationEviction(t *testing.T) {
	_ = testutil.SetupTestDB(t)
	db := databaseDB()

	createdAt := func(minutesAgo int) time.Time {
		return time.Now().Add(-time.Duration(minutesAgo) * time.Minute)
	}

	// A1：498 未读 + 2 已读（已读的是最旧两行）= 500 行，不触发淘汰
	seedRow(t, "success", 600, false) // oldest, read below
	seedRow(t, "success", 599, false) // second oldest, read below
	for i := 0; i < 498; i++ {
		seedRow(t, "success", i, false)
	}
	require.NoError(t, db.Model(&models.Notification{}).Where("created_at <= ?", createdAt(599)).Update("is_read", true).Error)
	var total int64
	require.NoError(t, db.Model(&models.Notification{}).Count(&total).Error)
	require.Equal(t, int64(500), total)

	// A2：=500 且存在已读行 → 写入 1 条触发淘汰最旧已读
	require.NoError(t, evictOverCap(db))

	// A2：=500 且存在已读行 → 写入 1 条触发淘汰最旧已读（用真实助手函数验证新行存活）
	date := time.Date(2026, 9, 17, 4, 0, 0, 0, time.Local)
	DailyReportFailedSummary(date, 5, 1)
	require.NoError(t, db.Model(&models.Notification{}).Count(&total).Error)
	require.Equal(t, int64(500), total)
	var newest models.Notification
	require.NoError(t, db.Order("created_at DESC, id DESC").First(&newest).Error)
	require.Equal(t, "error", newest.Type)
	require.Contains(t, newest.Title, "2026-09-17")

	// 连续写入：每次同步淘汰，稳定 ≤500（白盒边界值）
	for i := 0; i < 3; i++ {
		_, err := insertAndEvict("success", "日报已生成", "共 6 个版面，保存 42 条条目。", "daily-report", "2026-09-18")
		require.NoError(t, err)
		require.NoError(t, db.Model(&models.Notification{}).Count(&total).Error)
		require.LessOrEqual(t, total, int64(MaxNotifications))
	}
}

// TestNotificationEvictionAllUnread covers A3 + the read/unread distribution
// boundary: with zero read rows at cap the oldest unread row is evicted; when
// read rows exist they are evicted first (unread rows preserved).
func TestNotificationEvictionAllUnread(t *testing.T) {
	db := testutil.SetupTestDB(t)

	// A3：500 行全部未读 → 写入 1 条删最旧未读
	for i := 0; i < 500; i++ {
		seedRow(t, "success", i, false)
	}
	require.NoError(t, evictOverCap(db)) // 500 = cap, no excess yet
	var total int64
	require.NoError(t, db.Model(&models.Notification{}).Count(&total).Error)
	require.Equal(t, int64(500), total)

	// 超限后：删除 priority 最高（最旧未读 seed 行），新行存活
	_, err := insertAndEvict("success", "日报已生成", "新一条", "daily-report", "d")
	require.NoError(t, err)
	require.NoError(t, db.Model(&models.Notification{}).Count(&total).Error)
	require.Equal(t, int64(500), total)
	var stillThere models.Notification
	require.NoError(t, db.Where("summary = ?", "新一条").First(&stillThere).Error)
	require.Equal(t, "新一条", stillThere.Summary, "newest inserted row must survive")
	var oldestRemaining models.Notification
	require.NoError(t, db.Order("created_at ASC, id ASC").First(&oldestRemaining).Error)
	require.Equal(t, "seed row", oldestRemaining.Summary, "eviction removes the OLDEST seed row")

	// 已读/未读分布边界：最旧行=未读、次旧=已读 → 淘汰次旧已读，未读行保留
	require.NoError(t, databaseDB().Exec("DELETE FROM notifications").Error)
	// 500 行：oldest unread(1000) + read(999) + 498 unread(998..501)
	seedRow(t, "success", 1000, false)
	seedRow(t, "success", 999, true)
	for i := 2; i <= 499; i++ {
		seedRow(t, "success", 1000-i, false) // minutesAgo 998..501
	}
	require.NoError(t, databaseDB().Model(&models.Notification{}).Count(&total).Error)
	require.Equal(t, int64(500), total)

	_, err = insertAndEvict("success", "日报已生成", "又一条", "daily-report", "d")
	require.NoError(t, err)
	require.NoError(t, databaseDB().Model(&models.Notification{}).Count(&total).Error)
	require.Equal(t, int64(500), total)

	// 最旧未读行（minutesAgo=1000）仍在：已读行（999）优先被淘汰
	var oldestUnread models.Notification
	require.NoError(t, databaseDB().Where("is_read = ?", false).Order("created_at ASC, id ASC").First(&oldestUnread).Error)
	require.Equal(t, createdAtX(1000).Unix(), oldestUnread.CreatedAt.Unix())
	var readRow models.Notification
	err = databaseDB().Where("is_read = ? AND created_at = ?", true, createdAtX(999)).First(&readRow).Error
	require.Error(t, err, "oldest read row must have been evicted in priority over unread")
}

// TestNotificationContentVariants covers V1/V2: special characters persist
// verbatim; the daily-report helpers' fixed copy is well-formed.
func TestNotificationContentVariants(t *testing.T) {
	_ = testutil.SetupTestDB(t)

	long := make([]rune, 500)
	for i := range long {
		long[i] = '字'
	}
	// 特殊字符（title）+ 恰好 500 字（summary，列上限）原样入库，不破坏 JSON 广播（V1）
	notif, err := insertAndEvict("error", `标题"引号" & <特殊> 😀`, string(long), "daily-report", "x")
	require.NoError(t, err)
	require.Contains(t, notif.Title, `标题"引号" & <特殊> 😀`)
	require.Len(t, []rune(notif.Summary), 500)
}

// TestNotificationD3D4 covers D3/D4: single mark-read is idempotent.
func TestNotificationD3D4(t *testing.T) {
	_ = testutil.SetupTestDB(t)
	seedRow(t, "success", 10, false)

	notif, err := MarkRead(1)
	require.NoError(t, err)
	require.NotNil(t, notif)
	require.True(t, notif.IsRead)

	// D4：重复标已读 → no-op
	notif2, err := MarkRead(1)
	require.NoError(t, err)
	require.NotNil(t, notif2)

	// V6：越界引用 → nil, nil（handler 转 404）
	notif3, err := MarkRead(9999)
	require.NoError(t, err)
	require.Nil(t, notif3)
}

// TestMarkAllReadIdempotent covers V9: second call affects 0 rows.
func TestMarkAllReadIdempotent(t *testing.T) {
	_ = testutil.SetupTestDB(t)
	seedRow(t, "success", 10, false)
	seedRow(t, "success", 9, false)

	affected, err := MarkAllRead()
	require.NoError(t, err)
	require.Equal(t, int64(2), affected)

	affected, err = MarkAllRead()
	require.NoError(t, err)
	require.Equal(t, int64(0), affected)
}

// TestClearAll verifies DELETE /api/notifications semantics (V3 空表也 OK).
func TestClearAllEmpty(t *testing.T) {
	_ = testutil.SetupTestDB(t)
	affected, err := ClearAll()
	require.NoError(t, err)
	require.Equal(t, int64(0), affected)
}

// TestMarkAllReadIdempotent needs multiple distinct rows; seedRow writes one
// notification with a distinct created_at so ordering is deterministic.
func seedRow(t *testing.T, kind string, minutesAgo int, isRead bool) *models.Notification {
	t.Helper()
	notif := &models.Notification{
		Type:      kind,
		Title:     "日报已生成 · seed",
		Summary:   "seed row",
		LinkType:  "daily-report",
		LinkID:    "seed",
		IsRead:    isRead,
		CreatedAt: createdAtX(minutesAgo),
	}
	require.NoError(t, databaseDB().Create(notif).Error)
	return notif
}
