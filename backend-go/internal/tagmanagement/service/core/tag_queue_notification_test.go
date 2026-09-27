package core

import (
	"testing"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
)

// TestTagQueueDrainProducesNoNotification is the whitelist SHALL NOT anchor
// (spec notification-center「非白名单事件不产生通知」+「队列排空不产生通知」，
// test-cases 白盒 B 负向节拍）：the tag-queue completion/failed broadcasts go
// straight to the WS hub and MUST NOT write notification rows. The queue-drain
// path never touches the notification service; this test locks that invariant
// so a future accidental wiring fails here.
func TestTagQueueDrainProducesNoNotification(t *testing.T) {
	// 需要真实 PG：notifications 表走 AutoMigrate golden schema 且淘汰 SQL
	// 是 PG 方言（禁 SQLite）。
	db := testutil.SetupTestDB(t)

	queue := &TagQueue{} // 零值即可调广播方法（不启动 worker 池）

	// 模拟打标任务完成/失败的既有广播路径
	queue.broadcastTagCompleted(1, 100, []TopicTag{{
		Slug: "quantum-gravity", Label: "量子引力", Category: "physics", Score: 0.92, Icon: "mdi:atom",
	}})
	queue.broadcastTagFailed(2, 101, "model timeout")

	// 队列排空事件序列后：通知表必须无新增行
	var count int64
	require.NoError(t, db.Model(&models.Notification{}).Count(&count).Error)
	require.Zero(t, count, "tag queue events must NOT produce notifications")
}

// guard: keep gorm import used (signature parity with sibling test setups).
