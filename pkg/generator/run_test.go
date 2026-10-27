// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2026, RtBrick, Inc.
package generator

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// writeOutputs is a fake tool body appending "new" to every file passed as
// --option=.gen-<name>, like the real tools write their outputs.
const writeOutputs = `for arg in "$@"; do
  case "$arg" in
    --*=.gen-*) echo new >> "${arg#*=}" ;;
  esac
done
`

// fakeTool writes a shell script acting as the tool and returns its path.
// It lives outside dir, like the real tools do.
func fakeTool(t *testing.T, body string) string {
	t.Helper()
	script := filepath.Join(t.TempDir(), "tool")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\n"+body), 0o700)) //nolint:gosec // the fake tool must be executable
	return script
}

func always(v bool) func() bool { return func() bool { return v } }

func readFile(t *testing.T, name string) string {
	t.Helper()
	content, err := os.ReadFile(name)
	require.NoError(t, err)
	return string(content)
}

// requireNoTempFiles checks that a run leaves no temporary file behind.
func requireNoTempFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, entry := range entries {
		require.False(t, IsTempName(entry.Name()), entry.Name())
	}
}

func job(outputs ...string) Job {
	j := Job{Tool: "tool", Outputs: outputs}
	for _, output := range outputs {
		j.Args = append(j.Args, "--out="+TempName(output))
		j.Command = append(j.Command, "--out="+output)
	}
	return j
}

func TestRun_replacesWhenAllowed(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.bgp"), []byte("old\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gen-stale"), nil, 0o600))

	result, err := Run(t.Context(), dir, fakeTool(t, writeOutputs+"echo done\n"), job("a.bgp", "b.pcap"), always(true))
	require.NoError(t, err)
	require.Equal(t, []string{"a.bgp", "b.pcap"}, result.Files)
	require.Equal(t, "tool --out=a.bgp --out=b.pcap", result.Command)
	require.Equal(t, "done\n", result.Stdout)
	require.Equal(t, "new\n", readFile(t, filepath.Join(dir, "a.bgp")))
	require.Equal(t, "new\n", readFile(t, filepath.Join(dir, "b.pcap")))
	requireNoTempFiles(t, dir)
}

func TestRun_existingOutputWhileRunning(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.bgp"), []byte("loaded\n"), 0o600))

	_, err := Run(t.Context(), dir, fakeTool(t, "touch ran\n"), job("a.bgp"), always(false))
	require.ErrorIs(t, err, ErrOutputExists)
	require.NoFileExists(t, filepath.Join(dir, "ran"), "the tool must not run at all")
	require.Equal(t, "loaded\n", readFile(t, filepath.Join(dir, "a.bgp")))
}

// TestRun_outputAppearsDuringRun covers an instance started while the tool
// runs: the outputs are then moved without replacing, and a file that
// appeared in the meantime wins over the whole run.
func TestRun_outputAppearsDuringRun(t *testing.T) {
	dir := t.TempDir()
	calls := 0
	replace := func() bool {
		calls++
		return calls == 1 // stopped before the run, running afterwards
	}
	tool := fakeTool(t, writeOutputs+"echo other > b.bgp\n")

	_, err := Run(t.Context(), dir, tool, job("a.bgp", "b.bgp"), replace)
	require.ErrorIs(t, err, ErrOutputExists)
	require.NoFileExists(t, filepath.Join(dir, "a.bgp"), "outputs already moved are rolled back")
	require.Equal(t, "other\n", readFile(t, filepath.Join(dir, "b.bgp")))
	requireNoTempFiles(t, dir)
}

func TestRun_failureLeavesNothing(t *testing.T) {
	dir := t.TempDir()
	cache := filepath.Join(dir, "isis-level1-sequence-cache.json")
	require.NoError(t, os.WriteFile(cache, []byte("before"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.mrt"), []byte("old\n"), 0o600))
	tool := fakeTool(t, writeOutputs+`echo after > isis-level1-sequence-cache.json
echo new > isis-level2-sequence-cache.json
echo broken >&2
exit 3
`)
	j := job("a.mrt")
	j.SequenceCache = true

	result, err := Run(t.Context(), dir, tool, j, always(true))
	require.ErrorIs(t, err, ErrFailed)
	require.Equal(t, 3, result.ExitCode)
	require.Equal(t, "broken\n", result.Stderr)
	require.Empty(t, result.Files)
	require.Equal(t, "old\n", readFile(t, filepath.Join(dir, "a.mrt")))
	require.Equal(t, "before", readFile(t, cache), "the sequence numbers must not advance")
	require.NoFileExists(t, filepath.Join(dir, "isis-level2-sequence-cache.json"))
	requireNoTempFiles(t, dir)
}

func TestRun_successKeepsSequenceCache(t *testing.T) {
	dir := t.TempDir()
	cache := filepath.Join(dir, "isis-level1-sequence-cache.json")
	require.NoError(t, os.WriteFile(cache, []byte("before"), 0o600))
	j := job("a.mrt")
	j.SequenceCache = true

	_, err := Run(t.Context(), dir, fakeTool(t, writeOutputs+"echo after > isis-level1-sequence-cache.json\n"), j, always(true))
	require.NoError(t, err)
	require.Equal(t, "after\n", readFile(t, cache))
}

func TestRun_append(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "u.bgp"), []byte("old\n"), 0o600))
	j := job("u.bgp", "s.json")
	j.Appends = []string{"u.bgp", "s.json"} // s.json does not exist yet

	result, err := Run(t.Context(), dir, fakeTool(t, writeOutputs), j, always(true))
	require.NoError(t, err)
	require.Equal(t, []string{"u.bgp", "s.json"}, result.Files)
	require.Equal(t, "old\nnew\n", readFile(t, filepath.Join(dir, "u.bgp")))
	require.Equal(t, "new\n", readFile(t, filepath.Join(dir, "s.json")))
}

func TestRun_skipsOutputsNotWritten(t *testing.T) {
	dir := t.TempDir()
	j := Job{
		Tool:    LSPGen,
		Args:    []string{"--graphviz-file=" + TempName("topo")},
		Outputs: []string{"topo.dot", "topo.svg"},
	}
	// Without graphviz installed, lspgen writes the .dot file only.
	result, err := Run(t.Context(), dir, fakeTool(t, `echo graph > "${1#*=}.dot"`+"\n"), j, always(true))
	require.NoError(t, err)
	require.Equal(t, []string{"topo.dot"}, result.Files)
}

func TestRun_inputMissing(t *testing.T) {
	dir := t.TempDir()
	j := job("a.mrt")
	j.Inputs = []string{"topo.json"}

	_, err := Run(t.Context(), dir, fakeTool(t, writeOutputs), j, always(true))
	require.ErrorIs(t, err, ErrInputMissing)
	require.NoFileExists(t, filepath.Join(dir, "a.mrt"))
}

func TestRun_timeout(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()

	_, err := Run(ctx, dir, fakeTool(t, writeOutputs+"exec sleep 10\n"), job("a.mrt"), always(true))
	require.ErrorIs(t, err, ErrTimeout)
	require.NoFileExists(t, filepath.Join(dir, "a.mrt"))
	requireNoTempFiles(t, dir)
}

func TestRun_unavailable(t *testing.T) {
	_, err := Run(t.Context(), t.TempDir(), filepath.Join(t.TempDir(), "missing"), job("a.mrt"), always(true))
	require.ErrorIs(t, err, ErrUnavailable)
}

func TestRun_canceled(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(t.Context())
	time.AfterFunc(100*time.Millisecond, cancel)

	_, err := Run(ctx, dir, fakeTool(t, writeOutputs+"exec sleep 10\n"), job("a.mrt"), always(true))
	require.ErrorIs(t, err, ErrCanceled, "a client going away is no timeout")
	requireNoTempFiles(t, dir)
}

// TestRemoveTempFiles_keepsOtherDotFiles pins that only the two reserved
// prefixes are cleaned up, not every name starting with ".gen".
func TestRemoveTempFiles_keepsOtherDotFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{".gen-a.mrt", ".gen.bak-a.mrt", ".generated.json", ".gen_notes"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), nil, 0o600))
	}
	RemoveTempFiles(dir)
	require.NoFileExists(t, filepath.Join(dir, ".gen-a.mrt"))
	require.NoFileExists(t, filepath.Join(dir, ".gen.bak-a.mrt"))
	require.FileExists(t, filepath.Join(dir, ".generated.json"))
	require.FileExists(t, filepath.Join(dir, ".gen_notes"))
}

func TestRenameNoReplace(t *testing.T) {
	dir := t.TempDir()
	from, to := filepath.Join(dir, "from"), filepath.Join(dir, "to")
	require.NoError(t, os.WriteFile(from, []byte("new"), 0o600))
	require.NoError(t, os.WriteFile(to, []byte("old"), 0o600))

	require.ErrorIs(t, renameNoReplace(from, to), fs.ErrExist)
	require.Equal(t, "old", readFile(t, to))

	require.NoError(t, os.Remove(to))
	require.NoError(t, renameNoReplace(from, to))
	require.Equal(t, "new", readFile(t, to))
	require.NoFileExists(t, from)
}

func TestTailBuffer(t *testing.T) {
	b := &tailBuffer{limit: 4}
	_, _ = b.Write([]byte("ab"))
	require.Equal(t, "ab", b.String())
	_, _ = b.Write([]byte("cdef"))
	require.Equal(t, "[...]\ncdef", b.String())
}
