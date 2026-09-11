// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2025, RtBrick, Inc.
package server

import (
	"sync"
	"time"
)

const (
	// summaryCacheTTL is how long a summary response fetched from the
	// bngblaster control socket is reused for. The stream/session list views
	// issue one HTTP request per visible range while the user scrolls;
	// without a short-lived cache each of those would open a new unix socket
	// connection and re-run the (potentially large) summary command.
	summaryCacheTTL = 2 * time.Second

	// maxSummaryCacheEntries bounds a cache, which is keyed per instance
	// *and* per distinct filter combination (see streamFilters). It is reset
	// wholesale once this many entries accumulate rather than tracked with
	// per-entry eviction, since it only exists to make short-lived UI polling
	// cheap, not to be a long-lived store.
	maxSummaryCacheEntries = 64
)

type cacheEntry[T any] struct {
	fetchedAt time.Time
	value     T
	err       error
}

// inflight is a single in-progress fetch that later arrivals for the same key
// wait on instead of issuing a duplicate control-socket round-trip.
type inflight[T any] struct {
	done  chan struct{}
	value T
	err   error
}

// summaryCache memoizes control-socket responses per instance (and filter
// combination) for a short period. It exists purely to make server-side
// pagination cheap; it is not a source of truth and always expires quickly.
//
// Concurrent misses for the same key are coalesced into a single fetch: the
// UI polls every 2s with a 2s TTL, so without coalescing every poll would be
// a miss by construction and N open browser tabs would mean N socket
// round-trips for identical data.
type summaryCache[T any] struct {
	mu      sync.Mutex
	entries map[string]cacheEntry[T]
	calls   map[string]*inflight[T]
}

func newSummaryCache[T any]() *summaryCache[T] {
	return &summaryCache[T]{
		entries: map[string]cacheEntry[T]{},
		calls:   map[string]*inflight[T]{},
	}
}

func (c *summaryCache[T]) get(key string, fetch func() (T, error)) (T, error) {
	c.mu.Lock()
	if entry, ok := c.entries[key]; ok && time.Since(entry.fetchedAt) < summaryCacheTTL {
		c.mu.Unlock()
		return entry.value, entry.err
	}
	// Somebody else is already fetching exactly this: wait for their result
	// rather than opening a second socket for the same data.
	if call, ok := c.calls[key]; ok {
		c.mu.Unlock()
		<-call.done
		return call.value, call.err
	}
	call := &inflight[T]{done: make(chan struct{})}
	c.calls[key] = call
	c.mu.Unlock()

	call.value, call.err = fetch()

	c.mu.Lock()
	if len(c.entries) >= maxSummaryCacheEntries {
		// Keyed per instance *and* filter combination, so an interactive user
		// trying out several filters can otherwise grow this unboundedly over
		// a long session. This isn't a source of truth, so wiping it wholesale
		// is safe - anyone still polling just refetches on their next request.
		c.entries = map[string]cacheEntry[T]{}
	}
	c.entries[key] = cacheEntry[T]{fetchedAt: time.Now(), value: call.value, err: call.err}
	delete(c.calls, key)
	c.mu.Unlock()

	close(call.done)
	return call.value, call.err
}

// invalidate drops every entry belonging to one instance. Called whenever an
// instance's lifecycle changes (start/stop/kill/delete) so the UI does not
// keep being served up to summaryCacheTTL of stale rows from the previous
// run, and so a deleted instance leaves nothing behind.
func (c *summaryCache[T]) invalidate(instance string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key := range c.entries {
		if key == instance || (len(key) > len(instance) && key[:len(instance)] == instance && key[len(instance)] == '|') {
			delete(c.entries, key)
		}
	}
}
