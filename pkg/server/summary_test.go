// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2025, RtBrick, Inc.
package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/rtbrick/bngblaster-controller/pkg/controller"
)

// streamSummaryJSON builds a stream-summary socket response holding streams
// with the given flow ids.
func streamSummaryJSON(flowIDs ...int) []byte {
	streams := make([]controller.StreamSummaryStream, 0, len(flowIDs))
	for _, id := range flowIDs {
		streams = append(streams, controller.StreamSummaryStream{FlowId: id, Name: fmt.Sprintf("stream-%d", id)})
	}
	payload, err := json.Marshal(controller.StreamSummaryResponse{Code: 200, Streams: streams})
	if err != nil {
		panic(err)
	}
	return payload
}

func sessionSummaryJSON(sessionIDs ...int) []byte {
	sessions := make([]controller.SessionSummarySession, 0, len(sessionIDs))
	for _, id := range sessionIDs {
		sessions = append(sessions, controller.SessionSummarySession{SessionId: id})
	}
	payload, err := json.Marshal(controller.SessionSummaryResponse{Code: 200, Sessions: sessions})
	if err != nil {
		panic(err)
	}
	return payload
}

func rangeOf(from, to int) []int {
	ids := make([]int, 0, to-from+1)
	for id := from; id <= to; id++ {
		ids = append(ids, id)
	}
	return ids
}

func doGet(t *testing.T, handler http.Handler, target string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
	return recorder
}

func TestServer_streams_windowedRangeKeepsAbsoluteOffset(t *testing.T) {
	repository := &controller.RepositoryMock{
		ConfigFolderFunc: func() string { return configFolder },
		ExistsFunc:       func(name string) bool { return true },
		CommandFunc: func(name string, command controller.SocketCommand) ([]byte, error) {
			// bngblaster has already narrowed the result to the requested range.
			return streamSummaryJSON(rangeOf(101, 110)...), nil
		},
	}
	handler := NewServer(repository)

	recorder := doGet(t, handler,
		"/api/v1/instances/test/_streams?offset=100&limit=10&flow-id-min=101&flow-id-max=110&window=1")
	require.Equal(t, http.StatusOK, recorder.Code)

	var resp streamsResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	// A scroller window is returned as-is, offset being the absolute row it
	// starts at, so the client can position it without re-deriving anything.
	require.Equal(t, 100, resp.Offset)
	require.Len(t, resp.Items, 10)
	require.Equal(t, 101, resp.Items[0].FlowId)
}

func TestServer_streams_userRangeIsPaginatedAsAFlatList(t *testing.T) {
	repository := &controller.RepositoryMock{
		ConfigFolderFunc: func() string { return configFolder },
		ExistsFunc:       func(name string) bool { return true },
		CommandFunc: func(name string, command controller.SocketCommand) ([]byte, error) {
			return streamSummaryJSON(rangeOf(100001, 100010)...), nil
		},
	}
	handler := NewServer(repository)

	// The same range typed into the filter panel: without "window=1" this is
	// an ordinary filtered list. Reporting offset 100000 with a total of 10
	// (as it once did) made the client render a multi-million pixel spacer
	// above ten rows and claim to be showing "rows 100001-100010 of 10".
	recorder := doGet(t, handler,
		"/api/v1/instances/test/_streams?offset=0&limit=5&flow-id-min=100001&flow-id-max=100010")
	require.Equal(t, http.StatusOK, recorder.Code)

	var resp streamsResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Offset, "offset must be a row index, not a flow id")
	require.Equal(t, 10, resp.Total, "total must be the full filtered count")
	require.Len(t, resp.Items, 5)
	require.Equal(t, 100001, resp.Items[0].FlowId)

	// ... and the second page continues from where the first left off.
	recorder = doGet(t, handler,
		"/api/v1/instances/test/_streams?offset=5&limit=5&flow-id-min=100001&flow-id-max=100010")
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, 5, resp.Offset)
	require.Equal(t, 10, resp.Total)
	require.Equal(t, 100006, resp.Items[0].FlowId)
}

func TestServer_streams_onlyMinBoundIsNotAWindow(t *testing.T) {
	repository := &controller.RepositoryMock{
		ConfigFolderFunc: func() string { return configFolder },
		ExistsFunc:       func(name string) bool { return true },
		CommandFunc: func(name string, command controller.SocketCommand) ([]byte, error) {
			return streamSummaryJSON(rangeOf(50, 59)...), nil
		},
	}
	handler := NewServer(repository)

	recorder := doGet(t, handler, "/api/v1/instances/test/_streams?offset=2&limit=3&flow-id-min=50&window=1")
	require.Equal(t, http.StatusOK, recorder.Code)

	var resp streamsResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	// window=1 needs both bounds to mean anything; a half-open range falls
	// back to plain pagination rather than being returned unsliced.
	require.Equal(t, 2, resp.Offset)
	require.Equal(t, 10, resp.Total)
	require.Len(t, resp.Items, 3)
	require.Equal(t, 52, resp.Items[0].FlowId)
}

func TestServer_streams_offsetBeyondEnd(t *testing.T) {
	repository := &controller.RepositoryMock{
		ConfigFolderFunc: func() string { return configFolder },
		ExistsFunc:       func(name string) bool { return true },
		CommandFunc: func(name string, command controller.SocketCommand) ([]byte, error) {
			return streamSummaryJSON(1, 2, 3), nil
		},
	}
	handler := NewServer(repository)

	recorder := doGet(t, handler, "/api/v1/instances/test/_streams?offset=99&limit=10")
	require.Equal(t, http.StatusOK, recorder.Code)

	var resp streamsResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, 3, resp.Total)
	require.Equal(t, 3, resp.Offset)
	require.Empty(t, resp.Items)
}

func TestServer_streams_notRunning(t *testing.T) {
	repository := &controller.RepositoryMock{
		ConfigFolderFunc: func() string { return configFolder },
		ExistsFunc:       func(name string) bool { return true },
		CommandFunc: func(name string, command controller.SocketCommand) ([]byte, error) {
			return nil, controller.ErrBlasterNotRunning
		},
	}
	handler := NewServer(repository)

	recorder := doGet(t, handler, "/api/v1/instances/test/_streams")
	require.Equal(t, http.StatusPreconditionFailed, recorder.Code)
}

func TestServer_sessions_windowedRangeKeepsAbsoluteOffset(t *testing.T) {
	repository := &controller.RepositoryMock{
		ConfigFolderFunc: func() string { return configFolder },
		ExistsFunc:       func(name string) bool { return true },
		CommandFunc: func(name string, command controller.SocketCommand) ([]byte, error) {
			return sessionSummaryJSON(rangeOf(21, 30)...), nil
		},
	}
	handler := NewServer(repository)

	recorder := doGet(t, handler,
		"/api/v1/instances/test/_sessions?offset=20&limit=10&session-id-min=21&session-id-max=30&window=1")
	require.Equal(t, http.StatusOK, recorder.Code)

	var resp sessionsResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, 20, resp.Offset)
	require.Len(t, resp.Items, 10)
}

func TestServer_sessions_userRangeIsPaginatedAsAFlatList(t *testing.T) {
	repository := &controller.RepositoryMock{
		ConfigFolderFunc: func() string { return configFolder },
		ExistsFunc:       func(name string) bool { return true },
		CommandFunc: func(name string, command controller.SocketCommand) ([]byte, error) {
			return sessionSummaryJSON(rangeOf(9001, 9010)...), nil
		},
	}
	handler := NewServer(repository)

	recorder := doGet(t, handler,
		"/api/v1/instances/test/_sessions?offset=0&limit=4&session-id-min=9001&session-id-max=9010")
	require.Equal(t, http.StatusOK, recorder.Code)

	var resp sessionsResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Offset)
	require.Equal(t, 10, resp.Total)
	require.Len(t, resp.Items, 4)
}

func TestSummaryCache_coalescesConcurrentMisses(t *testing.T) {
	cache := newSummaryCache[int]()
	var fetches int32
	release := make(chan struct{})

	var wg sync.WaitGroup
	results := make([]int, 20)
	for i := range results {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			value, err := cache.get("instance", func() (int, error) {
				atomic.AddInt32(&fetches, 1)
				<-release
				return 42, nil
			})
			require.NoError(t, err)
			results[idx] = value
		}(i)
	}

	// Let every goroutine reach the cache before the single fetch completes.
	for atomic.LoadInt32(&fetches) == 0 {
	}
	close(release)
	wg.Wait()

	require.Equal(t, int32(1), atomic.LoadInt32(&fetches),
		"concurrent misses for one key must share a single control socket round-trip")
	for _, value := range results {
		require.Equal(t, 42, value)
	}
}

func TestSummaryCache_servesWithinTTLAndInvalidates(t *testing.T) {
	cache := newSummaryCache[int]()
	fetches := 0
	fetch := func() (int, error) {
		fetches++
		return fetches, nil
	}

	first, err := cache.get("inst", fetch)
	require.NoError(t, err)
	require.Equal(t, 1, first)

	second, err := cache.get("inst", fetch)
	require.NoError(t, err)
	require.Equal(t, 1, second, "a hit within the TTL must not refetch")
	require.Equal(t, 1, fetches)

	cache.invalidate("inst")
	third, err := cache.get("inst", fetch)
	require.NoError(t, err)
	require.Equal(t, 2, third, "invalidate must force the next read to refetch")
}

func TestSummaryCache_invalidateIsScopedToTheInstance(t *testing.T) {
	cache := newSummaryCache[int]()
	fetches := map[string]int{}
	fetchFor := func(key string) func() (int, error) {
		return func() (int, error) { fetches[key]++; return fetches[key], nil }
	}

	// "foo" plus one of its filter combinations, and a similarly named
	// instance that must not be caught by the prefix match.
	_, _ = cache.get("foo", fetchFor("foo"))
	_, _ = cache.get("foo|flow-id-min=1", fetchFor("foo-filtered"))
	_, _ = cache.get("foobar", fetchFor("foobar"))

	cache.invalidate("foo")

	_, _ = cache.get("foo", fetchFor("foo"))
	_, _ = cache.get("foo|flow-id-min=1", fetchFor("foo-filtered"))
	_, _ = cache.get("foobar", fetchFor("foobar"))

	require.Equal(t, 2, fetches["foo"])
	require.Equal(t, 2, fetches["foo-filtered"])
	require.Equal(t, 1, fetches["foobar"], "an instance with a shared name prefix must be left alone")
}

func TestServer_overview_aggregatesCommandsIntoOneCall(t *testing.T) {
	var issued []string
	var mu sync.Mutex
	repository := &controller.RepositoryMock{
		ConfigFolderFunc: func() string { return configFolder },
		ExistsFunc:       func(name string) bool { return true },
		CommandFunc: func(name string, command controller.SocketCommand) ([]byte, error) {
			mu.Lock()
			issued = append(issued, command.Command)
			mu.Unlock()
			switch command.Command {
			case "session-counters":
				return []byte(`{"status":"ok","code":200,"session-counters":{"sessions":7}}`), nil
			case "test-info":
				return []byte(`{"status":"ok","code":200,"test-info":{"duration":12,"state":"active"}}`), nil
			case "network-interfaces":
				return []byte(`{"status":"ok","code":200,"network-interfaces":[{"name":"eth0"}]}`), nil
			}
			// The remaining interface commands are unsupported by this build.
			return nil, fmt.Errorf("unknown command")
		},
	}
	handler := NewServer(repository)

	recorder := doGet(t, handler, "/api/v1/instances/test/_overview")
	require.Equal(t, http.StatusOK, recorder.Code)

	var resp map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.JSONEq(t, `{"sessions":7}`, string(resp["session-counters"]))
	require.JSONEq(t, `{"duration":12,"state":"active"}`, string(resp["test-info"]))
	require.JSONEq(t, `[{"name":"eth0"}]`, string(resp["network-interfaces"]))
	// A command that fails individually yields null rather than failing the
	// whole response, so the UI simply hides that section.
	require.Equal(t, "null", string(resp["access-interfaces"]))
	require.Equal(t, overviewCommands, issued)

	// The second request inside the cache period must not reach the socket
	// again: this is the whole point of aggregating them.
	mu.Lock()
	issued = nil
	mu.Unlock()
	require.Equal(t, http.StatusOK, doGet(t, handler, "/api/v1/instances/test/_overview").Code)
	require.Empty(t, issued)
}

func TestServer_overview_notRunning(t *testing.T) {
	repository := &controller.RepositoryMock{
		ConfigFolderFunc: func() string { return configFolder },
		ExistsFunc:       func(name string) bool { return true },
		CommandFunc: func(name string, command controller.SocketCommand) ([]byte, error) {
			return nil, controller.ErrBlasterNotRunning
		},
	}
	handler := NewServer(repository)

	recorder := doGet(t, handler, "/api/v1/instances/test/_overview")
	require.Equal(t, http.StatusPreconditionFailed, recorder.Code)
}

func TestServer_instances_detail(t *testing.T) {
	repository := &controller.RepositoryMock{
		ConfigFolderFunc: func() string { return configFolder },
		InstancesFunc:    func() []string { return []string{"alpha", "beta"} },
		RunningFunc:      func(name string) bool { return name == "beta" },
	}
	handler := NewServer(repository)

	// Without the flag the response is the plain name array it has always been.
	recorder := doGet(t, handler, "/api/v1/instances")
	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `["alpha","beta"]`, recorder.Body.String())

	// With it, one request replaces the list plus one status call per instance.
	recorder = doGet(t, handler, "/api/v1/instances?detail=true")
	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t,
		`[{"name":"alpha","status":"stopped"},{"name":"beta","status":"started"}]`,
		recorder.Body.String())
}

func TestServer_instances_detailOnEmptyListIsAnArray(t *testing.T) {
	repository := &controller.RepositoryMock{
		ConfigFolderFunc: func() string { return configFolder },
		InstancesFunc:    func() []string { return nil },
	}
	handler := NewServer(repository)

	recorder := doGet(t, handler, "/api/v1/instances?detail=true")
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "[]", strings.TrimSpace(recorder.Body.String()))
}
