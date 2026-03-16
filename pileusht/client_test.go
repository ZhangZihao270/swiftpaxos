package pileusht

import (
	"testing"

	"github.com/imdea-software/swiftpaxos/state"
)

func TestCacheEntryBasic(t *testing.T) {
	e := cacheEntry{
		Value:    []byte("hello"),
		LogIndex: 42,
	}
	if e.LogIndex != 42 {
		t.Errorf("expected LogIndex=42, got %d", e.LogIndex)
	}
	if string(e.Value) != "hello" {
		t.Errorf("expected Value=hello, got %s", string(e.Value))
	}
}

func TestWriteCachePopulatedOnWeakReply(t *testing.T) {
	cache := make(map[int64]cacheEntry)
	key := int64(100)
	val := state.Value("testval")
	slot := int32(55)

	// Simulate what handleWeakReply does: cache the write
	existing, exists := cache[key]
	if !exists || slot > existing.LogIndex {
		cache[key] = cacheEntry{Value: val, LogIndex: slot}
	}

	entry, ok := cache[key]
	if !ok {
		t.Fatal("expected cache entry for key 100")
	}
	if entry.LogIndex != 55 {
		t.Errorf("expected LogIndex=55, got %d", entry.LogIndex)
	}
	if string(entry.Value) != "testval" {
		t.Errorf("expected Value=testval, got %s", string(entry.Value))
	}
}

func TestWriteCacheHigherSlotWins(t *testing.T) {
	cache := make(map[int64]cacheEntry)
	key := int64(200)

	// First write at slot 10
	cache[key] = cacheEntry{Value: []byte("first"), LogIndex: 10}

	// Second write at slot 20 should overwrite
	slot := int32(20)
	existing := cache[key]
	if slot > existing.LogIndex {
		cache[key] = cacheEntry{Value: []byte("second"), LogIndex: slot}
	}

	entry := cache[key]
	if entry.LogIndex != 20 {
		t.Errorf("expected LogIndex=20, got %d", entry.LogIndex)
	}
	if string(entry.Value) != "second" {
		t.Errorf("expected Value=second, got %s", string(entry.Value))
	}

	// Third write at slot 15 (lower) should NOT overwrite
	slot = int32(15)
	existing = cache[key]
	if slot > existing.LogIndex {
		cache[key] = cacheEntry{Value: []byte("third"), LogIndex: slot}
	}

	entry = cache[key]
	if entry.LogIndex != 20 {
		t.Errorf("expected LogIndex still 20, got %d", entry.LogIndex)
	}
	if string(entry.Value) != "second" {
		t.Errorf("expected Value still second, got %s", string(entry.Value))
	}
}

func TestCacheMergeClientNewer(t *testing.T) {
	// Simulate: client has cached write at logIndex=50, follower at version=30
	cache := make(map[int64]cacheEntry)
	key := int64(300)
	cache[key] = cacheEntry{Value: []byte("cached_val"), LogIndex: 50}

	followerValue := []byte("stale_val")
	followerVersion := int32(30)

	// Merge logic from handleWeakReadReply
	result := followerValue
	if cached, ok := cache[key]; ok {
		if cached.LogIndex > followerVersion {
			result = cached.Value // Client's write is newer
		} else {
			delete(cache, key) // Follower caught up, evict
		}
	}

	if string(result) != "cached_val" {
		t.Errorf("expected cached_val (client newer), got %s", string(result))
	}
	// Cache should still exist (not evicted)
	if _, ok := cache[key]; !ok {
		t.Error("cache entry should still exist when client is newer")
	}
}

func TestCacheMergeFollowerCaughtUp(t *testing.T) {
	// Simulate: client has cached write at logIndex=50, follower at version=60
	cache := make(map[int64]cacheEntry)
	key := int64(400)
	cache[key] = cacheEntry{Value: []byte("cached_val"), LogIndex: 50}

	followerValue := []byte("fresh_val")
	followerVersion := int32(60)

	result := followerValue
	if cached, ok := cache[key]; ok {
		if cached.LogIndex > followerVersion {
			result = cached.Value
		} else {
			delete(cache, key) // Follower caught up, evict
		}
	}

	if string(result) != "fresh_val" {
		t.Errorf("expected fresh_val (follower newer), got %s", string(result))
	}
	// Cache should be evicted
	if _, ok := cache[key]; ok {
		t.Error("cache entry should be evicted when follower caught up")
	}
}

func TestCacheMergeNoCacheEntry(t *testing.T) {
	// No cache entry for this key — just use follower's reply
	cache := make(map[int64]cacheEntry)
	key := int64(500)

	followerValue := []byte("follower_val")
	followerVersion := int32(10)
	_ = followerVersion

	result := followerValue
	if cached, ok := cache[key]; ok {
		if cached.LogIndex > followerVersion {
			result = cached.Value
		} else {
			delete(cache, key)
		}
	}

	if string(result) != "follower_val" {
		t.Errorf("expected follower_val, got %s", string(result))
	}
}

func TestCacheMergeEqualVersion(t *testing.T) {
	// Edge case: logIndex == followerVersion — follower has caught up, evict cache
	cache := make(map[int64]cacheEntry)
	key := int64(600)
	cache[key] = cacheEntry{Value: []byte("cached_val"), LogIndex: 50}

	followerValue := []byte("follower_val")
	followerVersion := int32(50) // Equal to cache

	result := followerValue
	if cached, ok := cache[key]; ok {
		if cached.LogIndex > followerVersion {
			result = cached.Value
		} else {
			delete(cache, key) // Equal means follower has applied it
		}
	}

	if string(result) != "follower_val" {
		t.Errorf("expected follower_val (versions equal), got %s", string(result))
	}
	if _, ok := cache[key]; ok {
		t.Error("cache should be evicted when versions are equal")
	}
}
