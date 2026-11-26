// Command entity-backfill resolves HISTORICAL event tags against the entity
// identity layer (shadow). It exists because the entity hook only runs at tag
// time: tags created before 2026-11-10 have event fingerprints but no
// metadata.entity_subject, so entity-based story recall cannot see them.
//
// Run from backend-go:
//
//	go run ./cmd/entity-backfill --dry-run --limit 50
//	go run ./cmd/entity-backfill --subject-keys /tmp/drift-keys.txt --with-desc
//
// Safety contract (mirrors backfill-fingerprints):
//   - --dry-run never calls the model and never writes.
//   - Only TWO write surfaces: the entity layer (entity_registry /
//     entity_mentions / entity_aux_links, via the shared resolver) and the
//     lazy topic_tags.metadata.entity_subject key. No story, report, tag
//     business field, or fingerprint is touched.
//   - Existing successful entity_subject records are frozen (first writer
//     wins); unresolved records may be retried.
//   - Idempotent: rerunning skips tags that already hold a successful record.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/config"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/textutil"
	"syntopica-backend/internal/tagmanagement/repository"
	"syntopica-backend/internal/tagmanagement/service/core"
	"syntopica-backend/internal/tagmanagement/service/entity"
)

type options struct {
	Limit       int
	Workers     int
	DryRun      bool
	WithDesc    bool
	SubjectKeys string
	Since       string
	Until       string
	MaxWrites   int
}

func parseOptions(args []string) (options, error) {
	opts := options{}
	fs := flag.NewFlagSet("entity-backfill", flag.ContinueOnError)
	fs.IntVar(&opts.Limit, "limit", 0, "maximum tags to process (0 = unlimited)")
	fs.IntVar(&opts.Workers, "workers", 1, "concurrent tags (1-4; the resolver serializes on a global gate, so >1 rarely helps)")
	fs.BoolVar(&opts.DryRun, "dry-run", false, "read-only preview: no model calls, no writes")
	fs.BoolVar(&opts.WithDesc, "with-desc", false, "generate a stable identity description when the tag has none (one call per distinct subject)")
	fs.StringVar(&opts.SubjectKeys, "subject-keys", "", "file with one subject_key per line (empty = all historical event tags)")
	fs.StringVar(&opts.Since, "since", "", "only tags created at/after this date (YYYY-MM-DD, empty = no lower bound)")
	fs.StringVar(&opts.Until, "until", "", "only tags created before this date (YYYY-MM-DD, empty = no upper bound)")
	fs.IntVar(&opts.MaxWrites, "max-writes", 0, "hard cap on successful resolutions this run (0 = unlimited)")
	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	if fs.NArg() != 0 || opts.Limit < 0 || opts.Workers < 1 || opts.Workers > 4 || opts.MaxWrites < 0 {
		return opts, fmt.Errorf("invalid options: limit >= 0, workers 1-4, max-writes >= 0, no positional arguments")
	}
	return opts, nil
}

type summary struct {
	Planned       int
	Created       int
	Reused        int
	Idempotent    int
	Unresolved    int
	Skipped       int
	Failed        int
	DescGenerated int
	DescCached    int
	DescMissing   int
}

func parseFingerprint(meta models.MetadataMap) (core.EventFingerprint, bool) {
	raw, ok := meta["event_fingerprint"]
	if !ok {
		return core.EventFingerprint{}, false
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return core.EventFingerprint{}, false
	}
	var fp core.EventFingerprint
	if err := json.Unmarshal(data, &fp); err != nil {
		return core.EventFingerprint{}, false
	}
	return fp, fp.Subject != "" && fp.SubjectKey != ""
}

// hasSuccessfulRecord mirrors mergeEntitySubjectMetadata: anything other than
// an unresolved record is a frozen decision and must not be re-decided.
func hasSuccessfulRecord(meta models.MetadataMap) bool {
	raw, ok := meta["entity_subject"]
	if !ok {
		return false
	}
	nested, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	status, _ := nested["status"].(string)
	return status != "" && status != entity.StatusUnresolved
}

func loadSubjectKeys(path string) (map[string]bool, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path) //nolint:gosec // operator-supplied key list path
	if err != nil {
		return nil, fmt.Errorf("read subject keys: %w", err)
	}
	keys := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		key := strings.TrimSpace(line)
		if key != "" && !strings.HasPrefix(key, "#") {
			keys[key] = true
		}
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("subject keys file %q is empty", path)
	}
	return keys, nil
}

func buildQuery(ctx context.Context, db *gorm.DB, opts options, keys map[string]bool) ([]models.TopicTag, error) {
	query := db.WithContext(ctx).Model(&models.TopicTag{}).
		Where("category = ?", "event").
		Where("metadata ? 'event_fingerprint'").
		Where("NOT (metadata ? 'entity_subject')").
		Order("id ASC")
	if len(keys) > 0 {
		list := make([]string, 0, len(keys))
		for key := range keys {
			list = append(list, key)
		}
		query = query.Where("metadata->'event_fingerprint'->>'subject_key' IN ?", list)
	}
	if opts.Since != "" {
		query = query.Where("created_at >= ?", opts.Since)
	}
	if opts.Until != "" {
		query = query.Where("created_at < ?", opts.Until)
	}
	if opts.Limit > 0 {
		query = query.Limit(opts.Limit)
	}
	var tags []models.TopicTag
	if err := query.Find(&tags).Error; err != nil {
		return nil, fmt.Errorf("select historical event tags: %w", err)
	}
	return tags, nil
}

// articleContext pulls one article's title plus a bounded summary for the
// second-round question. Missing rows are not an error (tag may be orphaned).
func articleContext(ctx context.Context, db *gorm.DB, tagID uint) (uint, string) {
	var row struct {
		ArticleID uint   `gorm:"column:article_id"`
		Title     string `gorm:"column:title"`
		Summary   string `gorm:"column:summary"`
	}
	err := db.WithContext(ctx).Raw(`
		SELECT a.id AS article_id,
		       a.title,
		       coalesce(nullif(a.description, ''), a.ai_content_summary, '') AS summary
		FROM article_topic_tags att
		JOIN articles a ON a.id = att.article_id
		WHERE att.topic_tag_id = ?
		ORDER BY a.id DESC
		LIMIT 1`, tagID).Scan(&row).Error
	if err != nil || row.ArticleID == 0 {
		return 0, ""
	}
	snippet := row.Title
	if body := plainText(row.Summary); body != "" {
		snippet += "\n" + truncateRunes(body, 200)
	}
	return row.ArticleID, snippet
}

var htmlTagPattern = regexp.MustCompile(`<[^>]*>`)
var whitespacePattern = regexp.MustCompile(`\s+`)

// plainText strips HTML tags from article bodies before they enter a prompt.
func plainText(s string) string {
	s = htmlTagPattern.ReplaceAllString(s, " ")
	s = strings.NewReplacer("&nbsp;", " ", "&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", "\"").Replace(s)
	return strings.TrimSpace(whitespacePattern.ReplaceAllString(s, " "))
}

// say writes progress lines; a closed/failing stdout must not abort a
// backfill run (the DB writes are what matter).
func say(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func descSystemPrompt() string {
	return `你是实体识别助手。给定事件主体与事件上下文，输出一句不超过 30 字的稳定身份说明，
唯一指向该主体（谁 / 什么机构 / 什么产品），不得包含本次事件的动作或结论。
只输出这句说明本身，不要引号、不要解释、不要 JSON。`
}

func descUserPrompt(subject, label, context string) string {
	var b strings.Builder
	b.WriteString("主体：")
	b.WriteString(subject)
	if label != "" {
		b.WriteString("\n事件标签：")
		b.WriteString(label)
	}
	if context != "" {
		b.WriteString("\n上下文：")
		b.WriteString(truncateRunes(context, 300))
	}
	return b.String()
}

// generateDesc produces a stable identity description for a historical tag.
// One call per distinct subject; failures degrade to an empty desc (name-only
// resolution still works, just weaker for abbreviations).
func generateDesc(ctx context.Context, sessionID, subject, label, contextSnippet string) (string, error) {
	temperature, maxTokens := 0.1, 128
	req := airouter.ChatRequest{
		Operation:  "tagmanagement.entity_subject.desc",
		SessionID:  sessionID,
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: descSystemPrompt()},
			{Role: "user", Content: descUserPrompt(subject, label, contextSnippet)},
		},
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
		Metadata:    map[string]any{"prompt_version": "1", "surface": "entity_backfill"},
	}
	result, err := airouter.NewRouter().Chat(ctx, req)
	if err != nil {
		return "", err
	}
	desc := strings.TrimSpace(result.Content)
	desc = strings.Trim(desc, "\"'`")
	return truncateRunes(desc, 60), nil
}

type descCache struct {
	mu    sync.Mutex
	byKey map[string]string
}

func (c *descCache) get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	value, ok := c.byKey[key]
	return value, ok
}

func (c *descCache) put(key, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.byKey[key] = value
}

func run(ctx context.Context, args []string, out io.Writer) error {
	opts, err := parseOptions(args)
	if err != nil {
		return err
	}
	keys, err := loadSubjectKeys(opts.SubjectKeys)
	if err != nil {
		return err
	}
	if err := config.LoadConfig("./configs"); err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	logging.Init(config.AppConfig.Log.Level, logging.FileConfig{Enabled: false})
	// Do not use InitDB: this tool must not migrate or seed anything.
	db, err := gorm.Open(postgres.New(postgres.Config{DSN: config.AppConfig.Database.DSN, PreferSimpleProtocol: true}), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("open db (schema must already exist): %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer func() { _ = sqlDB.Close() }()
	database.DB = db
	repository.InitRepository(db)

	mode, modeErr := entity.CurrentMode()
	if modeErr != nil {
		say(out, "entity mode: error=%v (continuing: explicit backfill)\n", modeErr)
	} else {
		say(out, "entity mode: %s\n", mode)
	}

	tags, err := buildQuery(ctx, db, opts, keys)
	if err != nil {
		return err
	}
	say(out, "dry-run=%t with-desc=%t selected=%d keys=%d\n", opts.DryRun, opts.WithDesc, len(tags), len(keys))

	stats := summary{Planned: len(tags)}
	if opts.DryRun {
		for _, tag := range tags {
			fp, ok := parseFingerprint(tag.Metadata)
			if !ok {
				stats.Skipped++
				continue
			}
			if opts.WithDesc && strings.TrimSpace(readDesc(tag.Metadata)) == "" {
				stats.DescMissing++
			}
			_ = fp
		}
		say(out, "planned=%d skipped=%d desc_missing=%d (no model calls, no writes)\n", stats.Planned, stats.Skipped, stats.DescMissing)
		return nil
	}

	sessionID := fmt.Sprintf("entity_backfill_%d", time.Now().UnixNano())
	resolver := entity.DefaultResolver()
	cache := &descCache{byKey: map[string]string{}}
	started := time.Now()

	for index, tag := range tags {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if opts.MaxWrites > 0 && stats.Created+stats.Reused+stats.Idempotent >= opts.MaxWrites {
			say(out, "max-writes reached (%d), stopping\n", opts.MaxWrites)
			break
		}
		if err := resolveOne(ctx, db, resolver, cache, sessionID, opts, tag, &stats); err != nil {
			stats.Failed++
			logging.Warnf("entity backfill: tag %d failed: %v", tag.ID, err)
			continue
		}
		if (index+1)%25 == 0 {
			say(out, "progress %d/%d created=%d reused=%d idempotent=%d unresolved=%d failed=%d elapsed=%s\n",
				index+1, len(tags), stats.Created, stats.Reused, stats.Idempotent, stats.Unresolved, stats.Failed, time.Since(started).Round(time.Second))
		}
	}

	say(out, "done planned=%d created=%d reused=%d idempotent=%d unresolved=%d skipped=%d failed=%d desc_generated=%d desc_cached=%d elapsed=%s\n",
		stats.Planned, stats.Created, stats.Reused, stats.Idempotent, stats.Unresolved, stats.Skipped, stats.Failed,
		stats.DescGenerated, stats.DescCached, time.Since(started).Round(time.Millisecond))
	if stats.Failed > 0 {
		return fmt.Errorf("%d tags failed; successful writes retained; rerun safely", stats.Failed)
	}
	return nil
}

func readDesc(meta models.MetadataMap) string {
	raw, ok := meta["entity_subject"]
	if !ok {
		return ""
	}
	nested, ok := raw.(map[string]any)
	if !ok {
		return ""
	}
	desc, _ := nested["subject_desc"].(string)
	return desc
}

func resolveOne(ctx context.Context, db *gorm.DB, resolver *entity.Resolver, cache *descCache, sessionID string, opts options, tag models.TopicTag, stats *summary) error {
	fp, ok := parseFingerprint(tag.Metadata)
	if !ok {
		stats.Skipped++
		return nil
	}
	// Re-read to avoid racing a concurrent tag-time hook: a successful record
	// written between selection and now wins.
	var fresh models.TopicTag
	if err := db.WithContext(ctx).Select("id", "metadata").First(&fresh, tag.ID).Error; err != nil {
		return err
	}
	if hasSuccessfulRecord(fresh.Metadata) {
		stats.Skipped++
		return nil
	}

	articleID, snippet := articleContext(ctx, db, tag.ID)
	desc := readDesc(fresh.Metadata)
	if desc == "" {
		cacheKey := textutil.NormalizeEntityText(fp.Subject)
		if cached, hit := cache.get(cacheKey); hit {
			desc = cached
			stats.DescCached++
		} else if opts.WithDesc {
			generated, err := generateDesc(ctx, sessionID, fp.Subject, tag.Label, snippet)
			if err != nil {
				logging.Warnf("entity backfill: desc generation failed for tag %d (%q): %v", tag.ID, fp.Subject, err)
				generated = ""
			}
			desc = generated
			if desc != "" {
				stats.DescGenerated++
			}
			cache.put(cacheKey, desc)
		}
	}

	req := entity.MentionRequest{
		MentionText:    fp.Subject,
		SubjectDesc:    desc,
		ContextSnippet: strings.TrimSpace(tag.Label + "\n" + snippet),
		SourceTagID:    &tag.ID,
		SessionID:      sessionID,
	}
	if articleID != 0 {
		req.SourceArticleID = &articleID
	}
	resolution := resolver.ResolveMention(ctx, req)
	if resolution == nil {
		return fmt.Errorf("resolver returned nil resolution")
	}
	switch resolution.Status {
	case entity.StatusCreated:
		stats.Created++
	case entity.StatusReused:
		stats.Reused++
	case entity.StatusIdempotent:
		stats.Idempotent++
	default:
		stats.Unresolved++
	}

	record := &core.EntitySubjectRecord{
		Subject:     fp.Subject,
		SubjectKey:  fp.SubjectKey,
		SubjectDesc: desc,
		EntityID:    resolution.EntityID,
		EntityKey:   resolution.EntityKey,
		Status:      resolution.Status,
		Reason:      resolution.Reason,
		Mode:        "shadow",
	}
	if err := core.PersistEntitySubject(&fresh, record); err != nil {
		return fmt.Errorf("persist entity_subject: %w", err)
	}
	return nil
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:], os.Stdout)
	cancel()
	if err != nil && !errors.Is(err, flag.ErrHelp) {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
