package airouter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
)

// W10 降级观测还原（testcontainer PG 造日志；testing.md：SQL 聚合走真 PG，
// Docker required，-short 跳过）。造数形态对齐 test-cases W10：
// T1（A 失败→A structured 降级 json_object 成功）、T2（A 超时→B 成功 fallback）、
// T3（A、B 全失败）。

// seedLog 插入一行 ai_call_logs。token_usage 走 DB 默认 NULL（jsonb 列不接受
// 空串，与 Store.LogCall 的 Omit 语义一致）。
func seedLog(t *testing.T, db *gorm.DB, row models.AICallLog) {
	t.Helper()
	require.NoError(t, db.Omit("token_usage").Create(&row).Error)
}

// seedW10Traces 造 W10 三条 trace（topic_tagging 能力），返回基准时间。
func seedW10Traces(t *testing.T, db *gorm.DB) time.Time {
	t.Helper()
	base := time.Now().Add(-2 * time.Hour)

	// T1：A 失败（json_schema 4xx）→ A structured 降级 json_object 成功。
	seedLog(t, db, models.AICallLog{
		Operation: "tagmanagement.metadata", Capability: "topic_tagging",
		ProviderName: "local", Model: "qwythos", Success: false, IsFallback: false,
		ErrorCode: "http_400", RequestMeta: `{"structured_stage":"json_schema"}`,
		TraceID: "obs-t1", CreatedAt: base,
	})
	seedLog(t, db, models.AICallLog{
		Operation: "tagmanagement.metadata", Capability: "topic_tagging",
		ProviderName: "local", Model: "qwythos", Success: true, IsFallback: false,
		RequestMeta: `{"structured_stage":"json_object"}`,
		TraceID:     "obs-t1", CreatedAt: base.Add(10 * time.Second),
	})

	// T2：A 超时 → B 顺位迁移成功（fallback）。
	seedLog(t, db, models.AICallLog{
		Operation: "tagmanagement.metadata", Capability: "topic_tagging",
		ProviderName: "local", Model: "qwythos", Success: false, IsFallback: false,
		ErrorCode: "timeout", TraceID: "obs-t2", CreatedAt: base.Add(time.Hour),
	})
	seedLog(t, db, models.AICallLog{
		Operation: "tagmanagement.metadata", Capability: "topic_tagging",
		ProviderName: "deepseek", Model: "deepseek-chat", Success: true, IsFallback: true,
		TraceID: "obs-t2", CreatedAt: base.Add(time.Hour + 10*time.Second),
	})

	// T3：A、B 全失败（全链失败）。
	seedLog(t, db, models.AICallLog{
		Operation: "tagmanagement.metadata", Capability: "topic_tagging",
		ProviderName: "local", Model: "qwythos", Success: false, IsFallback: false,
		ErrorCode: "timeout", TraceID: "obs-t3", CreatedAt: base.Add(90 * time.Minute),
	})
	seedLog(t, db, models.AICallLog{
		Operation: "tagmanagement.metadata", Capability: "topic_tagging",
		ProviderName: "deepseek", Model: "deepseek-chat", Success: false, IsFallback: true,
		ErrorCode: "http_500", TraceID: "obs-t3", CreatedAt: base.Add(90*time.Minute + 10*time.Second),
	})
	return base
}

// findEvent 按 (capability, type, model) 找事件。
func findEvent(events []DegradationEvent, capability, typ, model string) *DegradationEvent {
	for i := range events {
		e := &events[i]
		if e.Capability == capability && e.Type == typ && e.Model == model {
			return e
		}
	}
	return nil
}

// W10-1/2/3/4/7：stats 口径 + 三类事件字段 + occurred_at 降序。
func TestQueryDegradationsRestoresEvents(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.SetupTestDB(t)
	base := seedW10Traces(t, db)

	report, err := NewStore(db).QueryDegradations(24*time.Hour, "")
	require.NoError(t, err)

	// W10-1：calls=3 组；degradations=T1 structured + T2 fallback=2（T3 全链
	// 失败不计入）；failures=T3=1。
	require.EqualValues(t, 3, report.Stats.Calls)
	require.EqualValues(t, 2, report.Stats.Degradations)
	require.EqualValues(t, 1, report.Stats.Failures)
	require.Len(t, report.Events, 3)

	// W10-2：T1 → structured 事件，code 由相邻行 structured_stage 推导。
	t1 := findEvent(report.Events, "topic_tagging", DegradationTypeStructured, "qwythos")
	require.NotNil(t, t1, "T1 应还原 structured 事件")
	require.Equal(t, "json_schema→json_object", t1.Code)
	require.Nil(t, t1.From)
	require.Nil(t, t1.To)
	// PG timestamptz 微秒精度截断纳秒，时间断言用容差比较。
	require.WithinDuration(t, base.Add(10*time.Second), t1.OccurredAt, time.Second)

	// W10-3：T2 → fallback 事件，from/to=0→1，model=命中模型，code=底层错误。
	// T2/T3 的 (capability,type,model) 相同，用顺位迁移标记区分：T2 带 from/to。
	var t2 *DegradationEvent
	for i := range report.Events {
		e := &report.Events[i]
		if e.Capability == "topic_tagging" && e.Type == DegradationTypeFallback && e.From != nil && e.To != nil {
			t2 = e
			break
		}
	}
	require.NotNil(t, t2, "T2 应还原 fallback 事件")
	require.Equal(t, "deepseek-chat", t2.Model)
	require.EqualValues(t, 0, *t2.From)
	require.EqualValues(t, 1, *t2.To)
	require.Equal(t, "timeout", t2.Code)

	// W10-4：T3 → 全链失败标记（from/to 空），model=最后尝试模型，code=最后错误码。
	// T2 与 T3 的 model 相同（T2 已断言）；T3 用 from/to 为空区分。
	var t3 *DegradationEvent
	for i := range report.Events {
		e := &report.Events[i]
		if e.Capability == "topic_tagging" && e.Type == DegradationTypeFallback && e.From == nil && e.To == nil {
			t3 = e
			break
		}
	}
	require.NotNil(t, t3, "T3 应还原全链失败事件")
	require.Equal(t, "deepseek-chat", t3.Model)
	require.Equal(t, "http_500", t3.Code)

	// W10-7：occurred_at 降序（T3 收尾 > T2 收尾 > T1 收尾）。
	for i := 1; i < len(report.Events); i++ {
		require.True(t, report.Events[i-1].OccurredAt.After(report.Events[i].OccurredAt),
			"events 必须按 occurred_at 降序")
	}
	require.Equal(t, "24h", report.Window)
}

// W10-5：capability 过滤只作用于 events，stats 保持全窗口口径。
func TestQueryDegradationsCapabilityFilter(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.SetupTestDB(t)
	base := seedW10Traces(t, db)

	// 另一能力的一组 fallback（summary）。
	seedLog(t, db, models.AICallLog{
		Operation: "reader.content_completion", Capability: "summary",
		ProviderName: "local", Model: "qwythos", Success: false, IsFallback: false,
		ErrorCode: "timeout", TraceID: "obs-sum", CreatedAt: base.Add(30 * time.Minute),
	})
	seedLog(t, db, models.AICallLog{
		Operation: "reader.content_completion", Capability: "summary",
		ProviderName: "deepseek", Model: "deepseek-chat", Success: true, IsFallback: true,
		TraceID: "obs-sum", CreatedAt: base.Add(30*time.Minute + 10*time.Second),
	})

	report, err := NewStore(db).QueryDegradations(24*time.Hour, "topic_tagging")
	require.NoError(t, err)
	require.Len(t, report.Events, 3, "events 只含 topic_tagging")
	for _, e := range report.Events {
		require.Equal(t, "topic_tagging", e.Capability)
	}
	// stats 保持全窗口口径（4 组，含 summary；全链失败只进 failures）。
	require.EqualValues(t, 4, report.Stats.Calls)
	require.EqualValues(t, 3, report.Stats.Degradations)
	require.EqualValues(t, 1, report.Stats.Failures)
	require.Len(t, report.Events, 3, "events 被过滤裁剪为 topic_tagging")
}

// W10-6：窗口边界——25h 前的行 24h 不含、7d 含。
func TestQueryDegradationsWindowBoundary(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.SetupTestDB(t)
	seedW10Traces(t, db)

	// 25h 前的单组（一次成功调用，无事件）。
	seedLog(t, db, models.AICallLog{
		Operation: "reader.content_completion", Capability: "summary",
		ProviderName: "deepseek", Model: "deepseek-chat", Success: true, IsFallback: false,
		TraceID: "obs-old", CreatedAt: time.Now().Add(-25 * time.Hour),
	})

	fresh, err := NewStore(db).QueryDegradations(24*time.Hour, "")
	require.NoError(t, err)
	require.EqualValues(t, 3, fresh.Stats.Calls, "24h 窗口不含 25h 前的行")

	week, err := NewStore(db).QueryDegradations(7*24*time.Hour, "")
	require.NoError(t, err)
	require.EqualValues(t, 4, week.Stats.Calls, "7d 窗口含 25h 前的行")
	require.Equal(t, "7d", week.Window)
}

// 空 trace_id 行每行自成组：单行全败 → 全链失败事件；成功行无事件。
func TestQueryDegradationsEmptyTraceIDRowsAreSingletonGroups(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.SetupTestDB(t)
	now := time.Now()
	seedLog(t, db, models.AICallLog{
		Operation: "tagmanagement.metadata", Capability: "topic_tagging",
		ProviderName: "local", Model: "qwythos", Success: false, IsFallback: false,
		ErrorCode: "timeout", CreatedAt: now.Add(-time.Hour), // 无 trace_id
	})
	seedLog(t, db, models.AICallLog{
		Operation: "tagmanagement.metadata", Capability: "topic_tagging",
		ProviderName: "local", Model: "qwythos", Success: true, IsFallback: false,
		CreatedAt: now.Add(-30 * time.Minute), // 无 trace_id
	})

	report, err := NewStore(db).QueryDegradations(24*time.Hour, "")
	require.NoError(t, err)
	require.EqualValues(t, 2, report.Stats.Calls, "空 trace_id 每行自成组")
	require.EqualValues(t, 1, report.Stats.Failures)
	require.Len(t, report.Events, 1)
	require.Nil(t, report.Events[0].From)
	require.Nil(t, report.Events[0].To)
	require.Equal(t, DegradationTypeFallback, report.Events[0].Type)
}

// 空窗口：stats 全 0 + events 空数组（非 null）。
func TestQueryDegradationsEmptyWindow(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.SetupTestDB(t)

	report, err := NewStore(db).QueryDegradations(24*time.Hour, "")
	require.NoError(t, err)
	require.EqualValues(t, 0, report.Stats.Calls)
	require.EqualValues(t, 0, report.Stats.Degradations)
	require.EqualValues(t, 0, report.Stats.Failures)
	require.NotNil(t, report.Events)
	require.Empty(t, report.Events)
}

// TestQueryDegradationsDoubleStructuredChain: 三档连降（json_schema→json_object
// →plain，同一模型两次降档）按「每次降级一条事件」还原两条 structured 事件，
// 不折叠为一条（spec「每次降级 SHALL 产生降级事件」；review low#6 修复固化）。
func TestQueryDegradationsDoubleStructuredChain(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.SetupTestDB(t)
	base := time.Now().Add(-1 * time.Hour)

	// T4：同模型三档连降——两档 4xx + 末档成功。
	seedLog(t, db, models.AICallLog{
		Operation: "tagmanagement.metadata", Capability: "digest_polish",
		ProviderName: "vendor-a", Model: "m1", Success: false, IsFallback: false,
		ErrorCode: "http_400", RequestMeta: `{"structured_stage":"json_schema"}`,
		TraceID: "obs-t4", CreatedAt: base,
	})
	seedLog(t, db, models.AICallLog{
		Operation: "tagmanagement.metadata", Capability: "digest_polish",
		ProviderName: "vendor-a", Model: "m1", Success: false, IsFallback: false,
		ErrorCode: "http_400", RequestMeta: `{"structured_stage":"json_object"}`,
		TraceID: "obs-t4", CreatedAt: base.Add(5 * time.Second),
	})
	seedLog(t, db, models.AICallLog{
		Operation: "tagmanagement.metadata", Capability: "digest_polish",
		ProviderName: "vendor-a", Model: "m1", Success: true, IsFallback: false,
		RequestMeta: `{"structured_stage":"plain"}`,
		TraceID:     "obs-t4", CreatedAt: base.Add(10 * time.Second),
	})

	report, err := NewStore(db).QueryDegradations(24*time.Hour, "digest_polish")
	require.NoError(t, err)
	require.EqualValues(t, 1, report.Stats.Calls)
	require.EqualValues(t, 2, report.Stats.Degradations, "两次降档各计一次")
	require.EqualValues(t, 0, report.Stats.Failures)

	var codes []string
	for _, e := range report.Events {
		require.Equal(t, DegradationTypeStructured, e.Type)
		require.Equal(t, "m1", e.Model)
		codes = append(codes, e.Code)
	}
	require.ElementsMatch(t, []string{"json_schema→json_object", "json_object→plain"}, codes)
}
