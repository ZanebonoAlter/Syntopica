package datasources

import (
	"testing"
	"time"
)

func TestTTLCacheHitKeepsRetrievedAt(t *testing.T) {
	c := NewTTLCache(10 * time.Minute)
	at := time.Now().Add(-2 * time.Minute).UTC() // original fetch time
	c.Put("k", FetchedDoc{Payload: []byte("v"), RetrievedAt: at, SHA256: "abc"})

	doc, ok := c.Get("k")
	if !ok || string(doc.Payload) != "v" {
		t.Fatalf("want hit, got ok=%v body=%q", ok, doc.Payload)
	}
	if !doc.RetrievedAt.Equal(at) || doc.SHA256 != "abc" {
		t.Fatalf("cached doc must keep original metadata: %+v", doc)
	}
}

func TestTTLCacheExpiry(t *testing.T) {
	c := NewTTLCache(30 * time.Millisecond)
	c.Put("k", FetchedDoc{Payload: []byte("v"), RetrievedAt: time.Now()})
	if _, ok := c.Get("k"); !ok {
		t.Fatal("fresh entry should hit")
	}
	time.Sleep(50 * time.Millisecond)
	if _, ok := c.Get("k"); ok {
		t.Fatal("expired entry must miss")
	}
	// After expiry+eviction the slot is gone; a new Put re-caches fine.
	c.Put("k", FetchedDoc{Payload: []byte("v2"), RetrievedAt: time.Now()})
	if got, ok := c.Get("k"); !ok || string(got.Payload) != "v2" {
		t.Fatalf("re-put should hit with new body, got ok=%v", ok)
	}
}

func TestTTLCacheEvict(t *testing.T) {
	c := NewTTLCache(time.Minute)
	c.Put("k", FetchedDoc{Payload: []byte("v"), RetrievedAt: time.Now()})
	c.Evict("k")
	if _, ok := c.Get("k"); ok {
		t.Fatal("evicted entry must miss")
	}
	// Evicting a missing key is a no-op.
	c.Evict("nope")
}

func TestTTLCacheDefensiveCopy(t *testing.T) {
	c := NewTTLCache(time.Minute)
	buf := []byte("original")
	c.Put("k", FetchedDoc{Payload: buf, RetrievedAt: time.Now()})
	buf[0] = 'X' // mutate the caller's slice after Put
	if got, ok := c.Get("k"); !ok || string(got.Payload) != "original" {
		t.Fatalf("cache must store a copy: %q", got.Payload)
	}
}
