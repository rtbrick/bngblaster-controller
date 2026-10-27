// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2026, RtBrick, Inc.
package controller

import (
	"os"
	"path"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/rtbrick/bngblaster-controller/pkg/generator"
)

// TestIsReservedName_matchesRunFiles keeps the generator's copy of the
// controller's file names in sync: a run file missing there could be
// overwritten by a generator.
func TestIsReservedName_matchesRunFiles(t *testing.T) {
	for _, name := range []string{
		ConfigFilename, runPidFilename, RunSockFilename, RunConfigFilename, RunLogFilename,
		RunReportFilename, RunPcapFilename, RunStdErr, RunStdOut,
	} {
		require.True(t, generator.IsReservedName(name), name)
	}
}

func TestDefaultRepository_Generate(t *testing.T) {
	folder := t.TempDir()
	instance := path.Join(folder, "test")
	require.NoError(t, os.MkdirAll(instance, permission))
	tool := path.Join(t.TempDir(), "tool")
	require.NoError(t, os.WriteFile(tool, []byte("#!/bin/sh\necho new > \"${1#*=}\"\n"), 0o700)) //nolint:gosec // the fake tool must be executable
	job, err := generator.Build(generator.LDPUpdate, []byte(`{"lsr-id": "10.0.0.1", "file": "u.ldp"}`))
	require.NoError(t, err)
	// The fake tool writes its first argument's file.
	job.Args = []string{"--file=" + generator.TempName("u.ldp")}

	r := NewDefaultRepository(WithConfigFolder(folder))
	_, err = r.Generate(t.Context(), "missing", tool, job)
	require.ErrorIs(t, err, ErrBlasterNotExists)

	result, err := r.Generate(t.Context(), "test", tool, job)
	require.NoError(t, err)
	require.Equal(t, []string{"u.ldp"}, result.Files)

	// Running: the existing file must not be replaced.
	require.NoError(t, os.WriteFile(path.Join(instance, runPidFilename), []byte(strconv.Itoa(os.Getpid())), permission))
	_, err = r.Generate(t.Context(), "test", tool, job)
	require.ErrorIs(t, err, generator.ErrOutputExists)
	require.NoError(t, os.Remove(path.Join(instance, runPidFilename)))

	// Busy: a second run and a delete are rejected, not queued.
	require.True(t, generating.tryLock(instance))
	_, err = r.Generate(t.Context(), "test", tool, job)
	require.ErrorIs(t, err, ErrGeneratorBusy)
	require.ErrorIs(t, r.Delete("test"), ErrGeneratorBusy)
	require.DirExists(t, instance)
	generating.unlock(instance)
	require.NoError(t, r.Delete("test"))
}

func TestDefaultRepository_RemoveGeneratorTempFiles(t *testing.T) {
	folder := t.TempDir()
	instance := path.Join(folder, "test")
	require.NoError(t, os.MkdirAll(instance, permission))
	require.NoError(t, os.WriteFile(path.Join(instance, generator.TempName("isis.mrt")), nil, permission))
	require.NoError(t, os.WriteFile(path.Join(instance, "isis.mrt"), nil, permission))

	r := NewDefaultRepository(WithConfigFolder(folder))
	files, err := r.Files("test")
	require.NoError(t, err)
	require.Equal(t, []InstanceFile{{Name: "isis.mrt"}}, files, "temporary files are never listed")

	r.RemoveGeneratorTempFiles()
	require.NoFileExists(t, path.Join(instance, generator.TempName("isis.mrt")))
	require.FileExists(t, path.Join(instance, "isis.mrt"))
}
