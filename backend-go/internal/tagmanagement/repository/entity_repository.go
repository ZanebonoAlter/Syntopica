package repository

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"syntopica-backend/internal/platform/logging"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
)

// Entity-subject storage (normalize-event-subject-entities T1).
//
// Repository for the three entity tables plus the cross-process resolution
// gate. Integration tests MUST run against testutil.SetupTestDB (isolated
// pgvector container) — repository package tests never use SQLite
// (standard/backend/testing.md §数据访问层禁 SQLite).

// ---- pgvector string helpers (local, mirrors auxlabel.ParsePgVector) ----

// EntityFloatsToPgVector renders a float vector as a pgvector literal.
func EntityFloatsToPgVector(v []float64) string {
	parts := make([]string, len(v))
	for i, f := range v {
		parts[i] = strconv.FormatFloat(f, 'g', -1, 64)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// EntityParsePgVector parses a pgvector literal into floats.
func EntityParsePgVector(s string) ([]float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	s = strings.TrimPrefix(strings.TrimSuffix(s, "]"), "[")
	if s == "" {
		return []float64{}, nil
	}
	parts := strings.Split(s, ",")
	out := make([]float64, 0, len(parts))
	for _, p := range parts {
		f, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil {
			return nil, fmt.Errorf("parse pgvector component %q: %w", p, err)
		}
		out = append(out, f)
	}
	return out, nil
}

// ---- EntityRepository ----

type EntityRepository struct {
	db *gorm.DB
}

func NewEntityRepository(db *gorm.DB) *EntityRepository {
	return &EntityRepository{db: db}
}

func (r *EntityRepository) DB() *gorm.DB { return r.db }

// CreateEntityAtomic creates the identity row + the first mention-evidence
// row + aux provenance rows in ONE short transaction (spec「none 在单个短
// 事务内原子登记」). Any step failing rolls the whole thing back — no
// half-entity ever lands. The transaction never spans a model call.
// mention.EntityID is filled from the created registry row.
func (r *EntityRepository) CreateEntityAtomic(ctx context.Context, registry *tagmodels.EntityRegistry, mention *tagmodels.EntityMention, auxLabelIDs []uint) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(registry).Error; err != nil {
			return fmt.Errorf("create entity registry: %w", err)
		}
		mention.EntityID = registry.ID
		if err := tx.Create(mention).Error; err != nil {
			return fmt.Errorf("create entity mention: %w", err)
		}
		for _, auxID := range auxLabelIDs {
			link := &tagmodels.EntityAuxLink{EntityID: registry.ID, MentionID: mention.ID, AuxLabelID: auxID}
			if err := tx.Create(link).Error; err != nil {
				return fmt.Errorf("create entity aux link: %w", err)
			}
		}
		return nil
	})
}

// AttachMentionAtomic attaches a new mention-evidence row (and its aux
// provenance rows) to an EXISTING entity in one short transaction. Same
// atomicity contract as CreateEntityAtomic.
func (r *EntityRepository) AttachMentionAtomic(ctx context.Context, entityID uint, mention *tagmodels.EntityMention, auxLabelIDs []uint) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var reg tagmodels.EntityRegistry
		if err := tx.Select("id").Where("id = ?", entityID).First(&reg).Error; err != nil {
			return fmt.Errorf("load entity %d for attach: %w", entityID, err)
		}
		mention.EntityID = entityID
		if err := tx.Create(mention).Error; err != nil {
			return fmt.Errorf("attach entity mention: %w", err)
		}
		for _, auxID := range auxLabelIDs {
			link := &tagmodels.EntityAuxLink{EntityID: entityID, MentionID: mention.ID, AuxLabelID: auxID}
			if err := tx.Create(link).Error; err != nil {
				return fmt.Errorf("create entity aux link: %w", err)
			}
		}
		return nil
	})
}

// FindDecisionByIdempotency replays the prior decision for the idempotency
// pair (source tag, mention key): returns the stored evidence row when the
// SAME tag already resolved the SAME mention — the caller reuses the entity
// without another model call (spec「同来源同提及重放幂等」). sourceTagID nil
// never replays (NULL sources are not idempotent).
func (r *EntityRepository) FindDecisionByIdempotency(ctx context.Context, sourceTagID *uint, mentionKey string) (*tagmodels.EntityMention, bool) {
	if sourceTagID == nil || mentionKey == "" {
		return nil, false
	}
	var m tagmodels.EntityMention
	err := r.db.WithContext(ctx).
		Where("source_tag_id = ? AND mention_key = ?", *sourceTagID, mentionKey).
		First(&m).Error
	if err != nil {
		return nil, false
	}
	return &m, true
}

// FindEntityByKey loads one identity row by its opaque entity_key.
func (r *EntityRepository) FindEntityByKey(ctx context.Context, entityKey string) (*tagmodels.EntityRegistry, error) {
	var reg tagmodels.EntityRegistry
	if err := r.db.WithContext(ctx).Where("entity_key = ?", entityKey).First(&reg).Error; err != nil {
		return nil, err
	}
	return &reg, nil
}

// FindEntityByID loads one identity row by primary key.
func (r *EntityRepository) FindEntityByID(ctx context.Context, id uint) (*tagmodels.EntityRegistry, error) {
	var reg tagmodels.EntityRegistry
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&reg).Error; err != nil {
		return nil, err
	}
	return &reg, nil
}

// FindEntityIDsByMentionKey is candidate path ③ (registered-name hit): every
// DISTINCT entity that was ever called by this normalized spelling. Names are
// allowed to be ambiguous — several entities may legitimately come back, the
// second-round model picks (or none). This is a candidate source ONLY; it
// never decides identity (D4).
func (r *EntityRepository) FindEntityIDsByMentionKey(ctx context.Context, mentionKey string) ([]uint, error) {
	if mentionKey == "" {
		return nil, nil
	}
	var ids []uint
	err := r.db.WithContext(ctx).Model(&tagmodels.EntityMention{}).
		Where("mention_key = ?", mentionKey).
		Distinct().Pluck("entity_id", &ids).Error
	return ids, err
}

// LoadEntityPool loads every identity row (in-memory cosine retrieval — the
// pool starts empty and grows slowly; ANN is out of scope this iteration).
// Vectors are parsed to floats once here.
func (r *EntityRepository) LoadEntityPool(ctx context.Context) ([]tagmodels.EntityRegistry, error) {
	var rows []tagmodels.EntityRegistry
	if err := r.db.WithContext(ctx).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load entity pool: %w", err)
	}
	return rows, nil
}

// ActiveAuxLinkCount returns the count of provenance rows whose aux label is
// still active (not GC-disabled, not deleted). GC-disabled aux rows are
// "失效" at read time — the rows stay but never count (spec「溯源行被移除或
// 失效」).
func (r *EntityRepository) ActiveAuxLinkCount(ctx context.Context, entityID uint) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&tagmodels.EntityAuxLink{}).
		Joins("JOIN semantic_labels ON semantic_labels.id = entity_aux_links.aux_label_id").
		Where("entity_aux_links.entity_id = ? AND semantic_labels.status = 'active'", entityID).
		Count(&n).Error
	return n, err
}

// ListActiveAuxLinks returns the active provenance rows for one entity
// (eval-report「材料溯源」列 and ops troubleshooting are the only consumers).
func (r *EntityRepository) ListActiveAuxLinks(ctx context.Context, entityID uint) ([]tagmodels.EntityAuxLink, error) {
	var links []tagmodels.EntityAuxLink
	err := r.db.WithContext(ctx).Model(&tagmodels.EntityAuxLink{}).
		Joins("JOIN semantic_labels ON semantic_labels.id = entity_aux_links.aux_label_id").
		Where("entity_aux_links.entity_id = ? AND semantic_labels.status = 'active'", entityID).
		Find(&links).Error
	return links, err
}

// CountEntities returns the total identity-row count (gate tests / eval).
func (r *EntityRepository) CountEntities(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&tagmodels.EntityRegistry{}).Count(&n).Error
	return n, err
}

// ---- Cross-process resolution gate (D6) ----

// EntityResolveGateKey is the process-external PostgreSQL session advisory
// lock key serializing「re-read latest pool → second-round choice → atomic
// register」across EVERY instance sharing the database. ONE global key on
// purpose (design D6 rejects per-mention-name sharding: Trump vs 特朗普 would
// shard differently and same-name-different-entity must not hard-merge).
//
// Constant = fnv64a("syntopica:entity-subject:resolve-gate"); compile-time
// stable so every instance computes the same key.
const EntityResolveGateKey int64 = 3433821233564581021 // 0x2fa7623800cf7c9d

// gateProbeTimeout bounds one advisory-lock roundtrip (probe / release).
const gateProbeTimeout = 5 * time.Second

// AcquireEntityResolveGate claims the global entity-resolution gate on a
// DEDICATED pooled connection (advisory locks bind to one session; a pooled
// borrow would let the lock leak onto a shared conn). It first tries
// non-blocking; on contention it waits on pg_advisory_lock bounded by ctx —
// ctx expiry ends as an explicit technical failure, never an infinite wait.
//
// The returned release func unlocks, hands the connection back to the pool
// and is safe to call from defer. Callers MUST call it exactly once on every
// path (success, error, panic) — the resolver wraps this with defer.
//
// DANGER (mirrors topicgraph generation_gate review H-1): a ctx timeout may
// fire client-side while the server ALREADY granted the lock — Close alone
// would return a healthy, still-locked session to the pool and leak the gate
// forever. That path kills the session first (a dead session releases its
// advisory locks; the pool discards the broken connection), then closes.
func (r *EntityRepository) AcquireEntityResolveGate(ctx context.Context) (func(), error) {
	sqlDB, err := r.db.DB()
	if err != nil {
		return nil, fmt.Errorf("entity gate: resolve *sql.DB: %w", err)
	}
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("entity gate: checkout dedicated conn: %w", err)
	}

	release := func() {
		rctx, cancel := context.WithTimeout(context.Background(), gateProbeTimeout)
		defer cancel()
		if _, err := conn.ExecContext(rctx, "SELECT pg_advisory_unlock($1)", EntityResolveGateKey); err != nil {
			// Unlock may have timed out client-side while actually succeeding
			// server-side — terminate the session to guarantee release; a
			// truly dead connection fails the kill harmlessly.
			logging.Warnf("entity gate: unlock failed (%v); terminating session to guarantee release", err)
			killCtx, kcancel := context.WithTimeout(context.Background(), gateProbeTimeout)
			defer kcancel()
			_, _ = conn.ExecContext(killCtx, "SELECT pg_terminate_backend(pg_backend_pid())")
		}
		_ = conn.Close() // hand the pool slot back on EVERY path
	}

	// Fast path: non-blocking try.
	probeCtx, pcancel := context.WithTimeout(ctx, gateProbeTimeout)
	defer pcancel()
	var locked bool
	if err := conn.QueryRowContext(probeCtx, "SELECT pg_try_advisory_lock($1)", EntityResolveGateKey).Scan(&locked); err != nil {
		// Probe failed — the DANGER window: kill the session (a dead session
		// drops its advisory locks) before returning the conn to the pool.
		killCtx, kcancel := context.WithTimeout(context.Background(), gateProbeTimeout)
		defer kcancel()
		_, _ = conn.ExecContext(killCtx, "SELECT pg_terminate_backend(pg_backend_pid())")
		_ = conn.Close()
		return nil, fmt.Errorf("entity gate: try-lock probe failed: %w", err)
	}
	if locked {
		return release, nil
	}

	// Contended: wait on the blocking lock bounded by the caller's ctx.
	// pg_advisory_lock returns void — Exec (result ignored), not Query+Scan.
	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", EntityResolveGateKey); err != nil {
		// Same DANGER window as above — the wait may have been granted
		// server-side right as ctx expired.
		killCtx, kcancel := context.WithTimeout(context.Background(), gateProbeTimeout)
		defer kcancel()
		_, _ = conn.ExecContext(killCtx, "SELECT pg_terminate_backend(pg_backend_pid())")
		_ = conn.Close()
		if ctx.Err() != nil {
			return nil, fmt.Errorf("entity gate: wait timed out (bounded, explicit technical failure): %w", ctx.Err())
		}
		return nil, fmt.Errorf("entity gate: acquire failed: %w", err)
	}
	return release, nil
}

// CosineSimilarity is the in-memory cosine used by the two retrieval paths.
// Dimension mismatch (cross-space leak) is an explicit error — callers skip
// the row, never fall back to another space (D3).
func CosineSimilarity(a, b []float64) (float64, error) {
	if len(a) != len(b) {
		return 0, fmt.Errorf("vector dimensions don't match: %d vs %d", len(a), len(b))
	}
	if len(a) == 0 {
		return 0, fmt.Errorf("empty vector")
	}
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0, nil
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb)), nil
}
