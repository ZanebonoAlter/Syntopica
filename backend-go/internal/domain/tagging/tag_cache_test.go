package tagging

import (
	"testing"
	"time"

	"syntopica-backend/internal/domain/models"
)

func TestTagCacheSetGet(t *testing.T) {
	cache := &TagCache{ttl: time.Minute}
	tag := &models.TopicTag{ID: 1, Label: "AI", Slug: "ai", Category: "keyword"}

	cache.Set("ai", "keyword", tag)

	got, ok := cache.Get("ai", "keyword")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.ID != 1 {
		t.Fatalf("got ID %d, want 1", got.ID)
	}
}

func TestTagCacheMiss(t *testing.T) {
	cache := &TagCache{ttl: time.Minute}
	_, ok := cache.Get("nonexistent", "keyword")
	if ok {
		t.Fatal("expected cache miss")
	}
}

func TestTagCacheExpiry(t *testing.T) {
	cache := &TagCache{ttl: 1 * time.Millisecond}
	tag := &models.TopicTag{ID: 2, Label: "Go", Slug: "go", Category: "keyword"}
	cache.Set("go", "keyword", tag)

	time.Sleep(5 * time.Millisecond)

	_, ok := cache.Get("go", "keyword")
	if ok {
		t.Fatal("expected cache miss after expiry")
	}
}

func TestTagCacheInvalidate(t *testing.T) {
	cache := &TagCache{ttl: time.Minute}
	tag := &models.TopicTag{ID: 3, Label: "Rust", Slug: "rust", Category: "keyword"}
	cache.Set("rust", "keyword", tag)

	cache.Invalidate("rust", "keyword")

	_, ok := cache.Get("rust", "keyword")
	if ok {
		t.Fatal("expected cache miss after invalidation")
	}
}

func TestTagCacheClear(t *testing.T) {
	cache := &TagCache{ttl: time.Minute}
	cache.Set("a", "keyword", &models.TopicTag{ID: 1})
	cache.Set("b", "event", &models.TopicTag{ID: 2})

	cache.Clear()

	_, ok1 := cache.Get("a", "keyword")
	_, ok2 := cache.Get("b", "event")
	if ok1 || ok2 {
		t.Fatal("expected all entries cleared")
	}
}
