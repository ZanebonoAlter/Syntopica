package tagging

import (
	"sync"
	"time"

	"my-robot-backend/internal/domain/models"
)

type tagCacheEntry struct {
	tag       *models.TopicTag
	expiresAt time.Time
}

type TagCache struct {
	entries sync.Map
	ttl     time.Duration
}

var globalTagCache = &TagCache{
	ttl: 10 * time.Minute,
}

func GetTagCache() *TagCache {
	return globalTagCache
}

func (c *TagCache) Get(slug, category string) (*models.TopicTag, bool) {
	key := slug + ":" + category
	val, ok := c.entries.Load(key)
	if !ok {
		return nil, false
	}
	entry := val.(*tagCacheEntry)
	if time.Now().After(entry.expiresAt) {
		c.entries.Delete(key)
		return nil, false
	}
	return entry.tag, true
}

func (c *TagCache) Set(slug, category string, tag *models.TopicTag) {
	key := slug + ":" + category
	c.entries.Store(key, &tagCacheEntry{
		tag:       tag,
		expiresAt: time.Now().Add(c.ttl),
	})
}

func (c *TagCache) Invalidate(slug, category string) {
	c.entries.Delete(slug + ":" + category)
}

func (c *TagCache) Clear() {
	c.entries.Range(func(key, _ any) bool {
		c.entries.Delete(key)
		return true
	})
}
