package database_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/testutil"
)

// findVendorModelMigration locates migration 20261005_0001's Up closure
// (ai-provider-model-overhaul design D2: ai_providers → ai_vendors + ai_models
// two-layer split).
func findVendorModelMigration(t *testing.T) func(*gorm.DB) error {
	t.Helper()
	for _, m := range database.ExportedPostgresMigrations() {
		if m.Version == "20261005_0001" {
			return m.Up
		}
	}
	t.Fatal("migration 20261005_0001 must be registered")
	return nil
}

// insertLegacyFixture seeds the pre-migration world: ai_providers 3 rows
// (qwen :8080 with start_command + enable_thinking=true; qwen3-embedding
// :8081 model_kind=embedding enable_thinking=false; deepseek-flash cloud
// enable_thinking=false——覆盖 llm 行 false→off 映射分支) + ai_routes 5
// capabilities + ai_route_providers 10
// bindings (W6 fixture, priorities from 1).
//
// The 10th binding deliberately puts the embedding model on the open_notebook
// llm route — that is the W6-8 conflict scenario ("同一模型挂 embedding 路由 +
// llm 路由"), exercised in the same run.
func insertLegacyFixture(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Exec(`INSERT INTO ai_providers
		(id, name, provider_type, base_url, api_key, model, enabled, timeout_seconds,
		 enable_thinking, model_kind, start_command, created_at, updated_at)
		VALUES
		(1, 'qwen', 'openai_compatible', 'http://10.11.12.111:8080/v1', '', 'qwythos',
		 true, 120, true, 'llm', 'llama-server -m qwythos.gguf --port 8080', now(), now()),
		(2, 'qwen3-embedding', 'openai_compatible', 'http://10.11.12.111:8081/v1', '', 'qwen3-embedding',
		 true, 120, false, 'embedding', '', now(), now()),
		(3, 'deepseek-flash', 'openai_compatible', 'https://api.deepseek.com/v1', 'sk-x', 'deepseek-flash',
		 true, 120, false, 'llm', '', now(), now())`).Error)

	require.NoError(t, db.Exec(`INSERT INTO ai_routes
		(id, name, capability, enabled, description, max_concurrency, created_at, updated_at)
		VALUES
		(1, 'embedding-route', 'embedding', true, '', 0, now(), now()),
		(2, 'topic-tagging-route', 'topic_tagging', true, '', 0, now(), now()),
		(3, 'summary-route', 'summary', true, '', 0, now(), now()),
		(4, 'digest-polish-route', 'digest_polish', true, '', 0, now(), now()),
		(5, 'open-notebook-route', 'open_notebook', true, '', 0, now(), now())`).Error)

	require.NoError(t, db.Exec(`INSERT INTO ai_route_providers
		(route_id, provider_id, priority, enabled, created_at, updated_at)
		VALUES
		(1, 2, 1, true, now(), now()),
		(2, 1, 1, true, now(), now()),
		(2, 3, 2, true, now(), now()),
		(3, 3, 1, true, now(), now()),
		(3, 1, 2, true, now(), now()),
		(4, 1, 1, true, now(), now()),
		(4, 3, 2, true, now(), now()),
		(5, 1, 1, true, now(), now()),
		(5, 3, 2, true, now(), now()),
		(5, 2, 3, true, now(), now())`).Error)
}

// legacyBinding is one ai_route_providers row in expectation form.
type legacyBinding struct {
	RouteID    uint
	ProviderID uint
	Priority   int
}

// TestVendorModelMigration exercises migration 20261005_0001 against a
// testcontainer PG: legacy ai_providers split into ai_vendors/ai_models,
// ai_route_providers reattached to ai_route_models, snapshot to JSON,
// idempotent re-run (test-cases.md W6-1..8).
//
// Note: the golden-schema build already ran this migration once against an
// empty ai_providers (AutoMigrate created the table with 0 rows, so the first
// pass split nothing). The manual Up call below is the second pass over real
// rows — the legacy_provider_id dedup must produce the full split on it.
func TestVendorModelMigration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.SetupTestDB(t)
	up := findVendorModelMigration(t)

	// 快照目录改到临时目录，避免写进仓库工作目录（W6-6）。
	snapshotDir := t.TempDir()
	*database.ExportedVendorSnapshotDir = snapshotDir

	insertLegacyFixture(t, db)

	// W6-8 的冲突告警在首跑即触发（fixture 第 10 条绑定：qwen3-embedding 同时
	// 挂 embedding 路由与 open_notebook llm 路由）——迁移不得失败，日志 WARN 列出。
	var logBuf bytes.Buffer
	logging.SetWriters(&logBuf, &logBuf)
	t.Cleanup(logging.ResetWriters)

	require.NoError(t, up(db), "first run over legacy rows must succeed (W6-8: conflict must not fail the migration)")

	// ── W6-1: ai_vendors=3，字段与推断方言/结构化档逐项核对 ──────────────
	type vendorRow struct {
		ID               uint
		Name             string
		ProviderType     string
		BaseURL          string
		APIKey           string
		TimeoutSeconds   int
		StartCommand     string
		ThinkingDialect  string
		StructuredOutput string
		LegacyProviderID *uint
		Enabled          bool
	}
	var vendors []vendorRow
	require.NoError(t, db.Raw(`SELECT id, name, provider_type, base_url, api_key, timeout_seconds,
		start_command, thinking_dialect, structured_output, legacy_provider_id, enabled
		FROM ai_vendors ORDER BY legacy_provider_id`).Scan(&vendors).Error)
	require.Len(t, vendors, 3, "W6-1: 3 legacy providers → 3 vendors")

	byName := make(map[string]vendorRow, len(vendors))
	for _, v := range vendors {
		byName[v.Name] = v
		require.NotNil(t, v.LegacyProviderID, "W6-5: every vendor carries its legacy_provider_id anchor")
		require.True(t, v.Enabled, "W6-2: enabled 沿用（fixture 全 true）")
	}

	qwen := byName["qwen"]
	require.Equal(t, "openai_compatible", qwen.ProviderType)
	require.Equal(t, "http://10.11.12.111:8080/v1", qwen.BaseURL)
	require.Empty(t, qwen.APIKey, "qwen 无 key")
	require.Equal(t, "llama-server -m qwythos.gguf --port 8080", qwen.StartCommand)
	require.Equal(t, 120, qwen.TimeoutSeconds)
	require.EqualValues(t, 1, *qwen.LegacyProviderID)
	require.Equal(t, "qwen-chat-template", qwen.ThinkingDialect, "W6-1: qwen*→qwen-chat-template")
	require.Equal(t, "json_schema", qwen.StructuredOutput, "W6-1: qwen*→json_schema")

	emb := byName["qwen3-embedding"]
	require.Equal(t, "http://10.11.12.111:8081/v1", emb.BaseURL)
	require.EqualValues(t, 2, *emb.LegacyProviderID)
	require.Equal(t, "qwen-chat-template", emb.ThinkingDialect, "W6-1: qwen 前缀推断同样命中 embedding 行")
	require.Equal(t, "json_schema", emb.StructuredOutput)

	ds := byName["deepseek-flash"]
	require.Equal(t, "https://api.deepseek.com/v1", ds.BaseURL)
	require.Equal(t, "sk-x", ds.APIKey)
	require.Empty(t, ds.StartCommand, "云端 vendor start_command 空")
	require.EqualValues(t, 3, *ds.LegacyProviderID)
	require.Equal(t, "deepseek", ds.ThinkingDialect, "W6-1: deepseek→deepseek")
	require.Equal(t, "json_object", ds.StructuredOutput, "W6-1: deepseek→json_object")

	// ── W6-2: ai_models=3，enable_thinking→thinking_level 映射 ───────────
	type modelRow struct {
		ID            uint
		VendorID      uint
		Name          string
		ModelKind     string
		ThinkingLevel *string
		Enabled       bool
		Status        string
		LastSeenAt    *time.Time
	}
	var aiModels []modelRow
	require.NoError(t, db.Raw(`SELECT id, vendor_id, name, model_kind, thinking_level, enabled, status, last_seen_at
		FROM ai_models ORDER BY id`).Scan(&aiModels).Error)
	require.Len(t, aiModels, 3, "W6-2: 3 legacy providers → 3 models")

	vendorIDByLegacy := map[uint]uint{} // legacy_provider_id → vendor.id
	for _, v := range vendors {
		vendorIDByLegacy[*v.LegacyProviderID] = v.ID
	}
	byModelName := make(map[string]modelRow, len(aiModels))
	for _, m := range aiModels {
		byModelName[m.Name] = m
		require.Equal(t, "ok", m.Status, "W6-2: status=ok")
		require.NotNil(t, m.LastSeenAt, "W6-2: last_seen_at=now()")
		require.True(t, m.Enabled, "W6-2: enabled 沿用")
	}

	qwythos := byModelName["qwythos"]
	require.Equal(t, "llm", qwythos.ModelKind, "W6-2: model_kind 沿用")
	require.NotNil(t, qwythos.ThinkingLevel)
	require.Equal(t, "high", *qwythos.ThinkingLevel, "W6-2: enable_thinking=true → high")
	require.Equal(t, vendorIDByLegacy[1], qwythos.VendorID, "model 挂属拆出的 vendor")

	embModel := byModelName["qwen3-embedding"]
	require.Equal(t, "embedding", embModel.ModelKind)
	require.Nil(t, embModel.ThinkingLevel, "W6-2: embedding 行 thinking_level 恒 NULL")
	require.Equal(t, vendorIDByLegacy[2], embModel.VendorID)

	dsModel := byModelName["deepseek-flash"]
	require.Equal(t, "llm", dsModel.ModelKind)
	require.NotNil(t, dsModel.ThinkingLevel)
	require.Equal(t, "off", *dsModel.ThinkingLevel, "W6-2: enable_thinking=false → off（deepseek 双向映射覆盖）")
	require.Equal(t, vendorIDByLegacy[3], dsModel.VendorID)

	// ── W6-3: ai_route_models=10，model 改挂对应、priority 保序、全 NULL 继承 ──
	var legacyBindings []legacyBinding
	require.NoError(t, db.Raw(`SELECT route_id, provider_id, priority FROM ai_route_providers ORDER BY route_id, priority`).Scan(&legacyBindings).Error)

	type bindingRow struct {
		RouteID       uint
		ModelID       uint
		Priority      int
		ThinkingLevel *string
		Enabled       bool
	}
	var bindings []bindingRow
	require.NoError(t, db.Raw(`SELECT route_id, model_id, priority, thinking_level, enabled
		FROM ai_route_models ORDER BY route_id, priority`).Scan(&bindings).Error)
	require.Len(t, bindings, len(legacyBindings), "W6-3: 绑定全部改挂（fixture 10 条）")

	modelIDByLegacy := map[uint]uint{} // legacy_provider_id → 拆出 model.id
	var vendorModelPairs []struct {
		LegacyProviderID uint
		ModelID          uint
	}
	require.NoError(t, db.Raw(`SELECT v.legacy_provider_id AS legacy_provider_id, m.id AS model_id
		FROM ai_models m JOIN ai_vendors v ON v.id = m.vendor_id`).Scan(&vendorModelPairs).Error)
	for _, p := range vendorModelPairs {
		modelIDByLegacy[p.LegacyProviderID] = p.ModelID
	}
	for i, lb := range legacyBindings {
		require.Equal(t, lb.RouteID, bindings[i].RouteID, "W6-3: route 保持")
		require.Equal(t, modelIDByLegacy[lb.ProviderID], bindings[i].ModelID,
			"W6-3: binding %d 改挂到 provider %d 拆出的 model", i, lb.ProviderID)
		require.Equal(t, lb.Priority, bindings[i].Priority, "W6-3: priority 顺序保持")
		require.True(t, bindings[i].Enabled)
		require.Nil(t, bindings[i].ThinkingLevel, "W6-3: thinking_level 全 NULL=继承模型默认")
	}

	// ── W6-4: 新层表无 metadata/strategy 死列；ai_routes 活列保留 ─────────
	// （ai_route_models.priority 是绑定顺位活列，不在死列清单内）
	var deadCols []string
	require.NoError(t, db.Raw(`SELECT table_name || '.' || column_name FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name IN ('ai_vendors', 'ai_models', 'ai_route_models')
		AND column_name IN ('metadata', 'strategy')`).Scan(&deadCols).Error)
	require.Empty(t, deadCols, "W6-4: 新层表不携带死字段: %v", deadCols)

	var legacyPriority []string
	require.NoError(t, db.Raw(`SELECT column_name FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name IN ('ai_vendors', 'ai_models')
		AND column_name = 'priority'`).Scan(&legacyPriority).Error)
	require.Empty(t, legacyPriority, "W6-4: vendor/model 层不携带 ai_routes 死列 priority")

	var liveCols []string
	require.NoError(t, db.Raw(`SELECT column_name FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'ai_routes'
		AND column_name IN ('capability', 'name', 'enabled', 'max_concurrency')`).Scan(&liveCols).Error)
	require.Len(t, liveCols, 4, "W6-4: ai_routes 表保留（capability/name/enabled/max_concurrency）")

	// ── W6-6: 拆解前快照落 JSON，3 行且含旧表死列 ────────────────────────
	payload, err := os.ReadFile(filepath.Join(snapshotDir, "ai_providers_legacy_snapshot.json"))
	require.NoError(t, err, "W6-6: 快照文件存在")
	var snapshot []map[string]any
	require.NoError(t, json.Unmarshal(payload, &snapshot))
	require.Len(t, snapshot, 3, "W6-6: 快照行数=3")
	require.Contains(t, snapshot[0], "metadata", "W6-6: 快照=拆解前旧表全列（含 metadata 死列）")
	require.Contains(t, snapshot[0], "enable_thinking")

	// ── W6-7: idx_call_logs_trace 索引存在 ───────────────────────────────
	var indexName string
	require.NoError(t, db.Raw(`SELECT indexname FROM pg_indexes
		WHERE schemaname = 'public' AND tablename = 'ai_call_logs' AND indexname = 'idx_call_logs_trace'`).Scan(&indexName).Error)
	require.Equal(t, "idx_call_logs_trace", indexName, "W6-7: trace_id 观测索引建立")

	// ── W6-5: 幂等——重复执行迁移，计数不变 ─────────────────────────────
	countOf := func(table string) int64 {
		t.Helper()
		var n int64
		require.NoError(t, db.Raw(`SELECT count(*) FROM `+table).Scan(&n).Error)
		return n
	}
	require.NoError(t, up(db), "re-run must be idempotent")
	require.EqualValues(t, 3, countOf("ai_vendors"), "W6-5: 重复执行不产生重复 vendor")
	require.EqualValues(t, 3, countOf("ai_models"), "W6-5: 重复执行不产生重复 model")
	require.EqualValues(t, 10, countOf("ai_route_models"), "W6-5: 重复执行不产生重复绑定")
	require.Len(t, snapshot, 3)

	// ── W6-8: 冲突告警日志（fixture 内造的 embedding×llm 双挂）──────────
	require.Contains(t, logBuf.String(), "bound to both embedding and llm routes",
		"W6-8: 迁移不失败且 WARN 列出冲突模型")
}
