package datasources

import (
	"sync"
	"time"
)

// TTLCache is the in-process response cache (design decision 4). Successes
// only — the fetchers decide what to cache; errors are never Put. Cache hits
// return the ORIGINAL FetchedDoc unchanged (spec: 缓存命中保留原元数据:
// retrieved_at/sha256/last_modified). No single-flight: single-user
// single-instance, concurrent correctness is not promised (test-cases ② 并发
// 划除).
type TTLCache struct {
	mu      sync.Mutex
	ttl     time.Duration
	entries map[string]*cacheEntry
}

type cacheEntry struct {
	expiresAt time.Time
	doc       FetchedDoc
}

// NewTTLCache builds a cache with the given TTL.
func NewTTLCache(ttl time.Duration) *TTLCache {
	return &TTLCache{ttl: ttl, entries: make(map[string]*cacheEntry)}
}

// Get returns the cached document. Expired entries are removed and reported
// as misses.
func (c *TTLCache) Get(key string) (FetchedDoc, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok {
		return FetchedDoc{}, false
	}
	if time.Now().After(e.expiresAt) {
		delete(c.entries, key)
		return FetchedDoc{}, false
	}
	doc := e.doc
	doc.FromCache = true
	return doc, true
}

// Put stores a successful document; the doc's RetrievedAt/SHA256 must be the
// ORIGINAL upstream values so later hits report them faithfully.
func (c *TTLCache) Put(key string, doc FetchedDoc) {
	c.mu.Lock()
	defer c.mu.Unlock()
	stored := doc
	stored.Payload = append([]byte(nil), doc.Payload...) // defensive copy
	c.entries[key] = &cacheEntry{expiresAt: time.Now().Add(c.ttl), doc: stored}
}

// Evict drops a key (used for SCHEMA_CHANGED / HTML masquerade on the same
// cache key).
func (c *TTLCache) Evict(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}
