// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2025, RtBrick, Inc.
package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeLogFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func TestTailLogFile_readsCompleteLinesOnly(t *testing.T) {
	file := filepath.Join(t.TempDir(), "run.log")
	writeLogFile(t, file, "first\nsecond\npartial")

	resp, err := tailLogFile(file, 0, defaultLogReadLimit)
	require.NoError(t, err)

	// "partial" has no terminating newline yet, so it must be held back for
	// the next poll rather than emitted as a truncated line.
	require.Equal(t, []string{"first", "second"}, resp.Lines)
	require.Equal(t, int64(0), resp.Offset)
	require.Equal(t, int64(len("first\nsecond\n")), resp.NextOffset)
	require.False(t, resp.EOF)
	require.NotZero(t, resp.Generation)
}

func TestTailLogFile_resumesFromOffset(t *testing.T) {
	file := filepath.Join(t.TempDir(), "run.log")
	writeLogFile(t, file, "first\nsecond\n")

	first, err := tailLogFile(file, 0, defaultLogReadLimit)
	require.NoError(t, err)
	require.Equal(t, []string{"first", "second"}, first.Lines)
	require.True(t, first.EOF)

	writeLogFile(t, file, "first\nsecond\nthird\n")

	second, err := tailLogFile(file, first.NextOffset, defaultLogReadLimit)
	require.NoError(t, err)
	require.Equal(t, []string{"third"}, second.Lines)
	require.True(t, second.EOF)
}

func TestTailLogFile_rewindsWhenFileShrank(t *testing.T) {
	file := filepath.Join(t.TempDir(), "run.log")
	writeLogFile(t, file, "aaaa\nbbbb\ncccc\n")
	long, err := tailLogFile(file, 0, defaultLogReadLimit)
	require.NoError(t, err)

	// A restart recreates run.log; a shorter replacement is detectable from
	// the size alone and must restart from the beginning.
	writeLogFile(t, file, "new\n")
	resp, err := tailLogFile(file, long.NextOffset, defaultLogReadLimit)
	require.NoError(t, err)
	require.Equal(t, int64(0), resp.Offset)
	require.Equal(t, []string{"new"}, resp.Lines)
}

func TestTailLogFile_generationChangesWhenFileIsReplaced(t *testing.T) {
	file := filepath.Join(t.TempDir(), "run.log")
	writeLogFile(t, file, "one\ntwo\n")
	before, err := tailLogFile(file, 0, defaultLogReadLimit)
	require.NoError(t, err)

	// The case a size comparison cannot catch: the file is replaced and the
	// replacement is already longer than the offset carried over from the
	// previous run. Only the generation reveals that the offset is stale.
	require.NoError(t, os.Remove(file))
	writeLogFile(t, file, "alpha\nbravo\ncharlie\ndelta\n")

	after, err := tailLogFile(file, before.NextOffset, defaultLogReadLimit)
	require.NoError(t, err)
	require.NotEqual(t, before.Generation, after.Generation,
		"a recreated log file must report a different generation")
}

func TestTailLogFile_respectsLimit(t *testing.T) {
	file := filepath.Join(t.TempDir(), "run.log")
	writeLogFile(t, file, "aaaa\nbbbb\ncccc\n")

	// Only "aaaa\n" fits whole inside a 7 byte budget.
	resp, err := tailLogFile(file, 0, 7)
	require.NoError(t, err)
	require.Equal(t, []string{"aaaa"}, resp.Lines)
	require.Equal(t, int64(5), resp.NextOffset)
	require.False(t, resp.EOF)
}

func TestTailLogFile_missingFile(t *testing.T) {
	_, err := tailLogFile(filepath.Join(t.TempDir(), "absent.log"), 0, defaultLogReadLimit)
	require.True(t, os.IsNotExist(err))
}
