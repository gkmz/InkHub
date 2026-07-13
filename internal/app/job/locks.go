package job

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
)

type lockEntry struct {
	semaphore  chan struct{}
	references int
}

type keyedLocker struct {
	mu      sync.Mutex
	entries map[string]*lockEntry
}

func newKeyedLocker() *keyedLocker {
	return &keyedLocker{entries: make(map[string]*lockEntry)}
}

func (l *keyedLocker) acquire(ctx context.Context, keys []string) (func(), error) {
	// 所有任务按同一稳定顺序获取多把锁，避免文章锁与 Provider 锁形成循环等待。
	keys = uniqueSorted(keys)
	entries := make([]*lockEntry, len(keys))
	l.mu.Lock()
	for index, key := range keys {
		entry := l.entries[key]
		if entry == nil {
			entry = &lockEntry{semaphore: make(chan struct{}, 1)}
			entry.semaphore <- struct{}{}
			l.entries[key] = entry
		}
		entry.references++
		entries[index] = entry
	}
	l.mu.Unlock()

	acquired := 0
	for index, entry := range entries {
		select {
		case <-ctx.Done():
			l.release(keys, entries, acquired)
			return nil, ctx.Err()
		case <-entry.semaphore:
			acquired = index + 1
		}
	}
	return func() { l.release(keys, entries, acquired) }, nil
}

func (l *keyedLocker) release(keys []string, entries []*lockEntry, acquired int) {
	for index := acquired - 1; index >= 0; index-- {
		entries[index].semaphore <- struct{}{}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	// 引用归零后删除锁项，避免长期运行时按文章数量无限增长。
	for index, key := range keys {
		entries[index].references--
		if entries[index].references == 0 {
			delete(l.entries, key)
		}
	}
}

func lockKeys(payloadJSON string) []string {
	var payload struct {
		ArticleID          string `json:"article_id"`
		ProviderInstanceID string `json:"provider_instance_id"`
	}
	if json.Unmarshal([]byte(payloadJSON), &payload) != nil {
		return nil
	}
	var keys []string
	if payload.ArticleID != "" {
		keys = append(keys, "article:"+payload.ArticleID)
	}
	if payload.ProviderInstanceID != "" {
		keys = append(keys, "provider:"+payload.ProviderInstanceID)
	}
	return keys
}

func uniqueSorted(values []string) []string {
	sort.Strings(values)
	result := values[:0]
	for _, value := range values {
		if value == "" || (len(result) > 0 && result[len(result)-1] == value) {
			continue
		}
		result = append(result, value)
	}
	return result
}
