// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2026, RtBrick, Inc.
package generator

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// Errors returned by this package.
var (
	// ErrUnknownTool the requested tool is not supported.
	ErrUnknownTool = errors.New("unknown generator")
	// ErrInvalidRequest the request does not match the tool's options.
	ErrInvalidRequest = errors.New("invalid generator request")
	// ErrUnavailable the tool is not installed or cannot be executed.
	ErrUnavailable = errors.New("generator not available")
	// ErrInputMissing a file the tool reads does not exist.
	ErrInputMissing = errors.New("input file does not exist")
	// ErrOutputExists an output would replace an existing file while that
	// is not allowed (the instance is running).
	ErrOutputExists = errors.New("output file exists")
	// ErrFailed the tool exited with an error.
	ErrFailed = errors.New("generator failed")
	// ErrTimeout the tool was stopped because ctx hit its deadline.
	ErrTimeout = errors.New("generator timed out")
	// ErrCanceled the tool was stopped because ctx was canceled, e.g.
	// since the client of the request went away.
	ErrCanceled = errors.New("generator canceled")
)

const (
	// tempPrefix marks the files Run writes before they are moved into
	// place. backupPrefix marks the hard links of files about to be
	// replaced. They differ in their fifth character, so they never
	// collide, and only these two prefixes are reserved: a user's own
	// ".generated.json" is neither hidden nor removed.
	tempPrefix   = ".gen-"
	backupPrefix = ".gen.bak-"

	// sequenceCacheSuffix see Tool.SequenceCache.
	sequenceCacheSuffix = "-sequence-cache.json"

	// maxOutput bounds the stdout and stderr kept per run. The end is
	// kept since that is where tools report what went wrong.
	maxOutput = 256 << 10

	// sequenceCachePermission is the mode lspgen creates its sequence
	// cache files with (0666 minus the usual umask).
	sequenceCachePermission = 0o644

	// waitDelay is how long a tool may keep its output pipes open after
	// it was killed for running too long.
	waitDelay = 5 * time.Second
)

// ExecCommand exposes exec.CommandContext and allows therefore to test.
var ExecCommand = exec.CommandContext

// TempName returns the temporary name a tool writes the output name to.
func TempName(name string) string {
	return tempPrefix + name
}

// IsTempName reports whether name is a temporary or backup file of Run.
// Those are never listed, uploaded or downloaded.
func IsTempName(name string) bool {
	return strings.HasPrefix(name, tempPrefix) || strings.HasPrefix(name, backupPrefix)
}

// Result is the outcome of a run.
type Result struct {
	// Files are the files the run has written, by their final names.
	Files []string `json:"files"`
	// Command is the command line that was run, with final file names.
	Command    string `json:"command"`
	ExitCode   int    `json:"exit-code"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	DurationMS int64  `json:"duration-ms"`
}

// Run runs job in dir, the instance folder, with executable as the tool.
//
// Every output is written under its temporary name and moved into place
// only once the tool exited successfully; a failed, timed out or
// cancelled run leaves no output behind and restores lspgen's sequence
// cache, so a broken run does not advance the sequence numbers either.
//
// replace is asked twice whether existing files may be replaced: before
// the tool runs, to fail early, and again when the outputs are moved into
// place, since the instance may have been started in between. If not,
// outputs are moved with RENAME_NOREPLACE, so a file that appeared in the
// meantime is never replaced either.
//
// The caller must make sure that only one Run uses dir at a time.
func Run(ctx context.Context, dir, executable string, job Job, replace func() bool) (Result, error) {
	result := Result{Command: strings.Join(append([]string{filepath.Base(executable)}, job.Command...), " ")}
	RemoveTempFiles(dir)
	if err := check(dir, job, replace()); err != nil {
		return result, err
	}

	var cache sequenceCache
	if job.SequenceCache {
		var err error
		if cache, err = snapshotSequenceCache(dir); err != nil {
			return result, err
		}
	}
	// failed undoes everything the run has changed so far.
	failed := func(err error) (Result, error) {
		RemoveTempFiles(dir)
		if job.SequenceCache {
			cache.restore(dir)
		}
		return result, err
	}

	for _, output := range job.Appends {
		err := copyFile(filepath.Join(dir, output), filepath.Join(dir, TempName(output)))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return failed(err)
		}
	}
	if err := execute(ctx, dir, executable, job.Args, &result); err != nil {
		return failed(err)
	}
	files, err := commit(dir, job.Outputs, replace())
	if err != nil {
		return failed(err)
	}
	result.Files = files
	return result, nil
}

// check verifies the files of a job before it runs: the inputs must exist
// and, unless replace, the outputs must not.
func check(dir string, job Job, replace bool) error {
	for _, input := range job.Inputs {
		if _, err := os.Stat(filepath.Join(dir, input)); err != nil {
			return fmt.Errorf("%w: %s", ErrInputMissing, input)
		}
	}
	if replace {
		return nil
	}
	for _, output := range job.Outputs {
		if _, err := os.Lstat(filepath.Join(dir, output)); err == nil {
			return fmt.Errorf("%w: %s", ErrOutputExists, output)
		}
	}
	return nil
}

// execute runs the tool and records its output in result.
func execute(ctx context.Context, dir, executable string, args []string, result *Result) error {
	stdout := &tailBuffer{limit: maxOutput}
	stderr := &tailBuffer{limit: maxOutput}
	cmd := ExecCommand(ctx, executable, args...)
	cmd.Dir = dir
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.WaitDelay = waitDelay
	start := time.Now()
	err := cmd.Run()
	result.DurationMS = time.Since(start).Milliseconds()
	result.Stdout = stdout.String()
	result.Stderr = stderr.String()
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}
	switch {
	case err == nil:
		return nil
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return fmt.Errorf("%w: %w", ErrTimeout, ctx.Err())
	case ctx.Err() != nil:
		return fmt.Errorf("%w: %w", ErrCanceled, ctx.Err())
	case errors.Is(err, exec.ErrNotFound), errors.Is(err, fs.ErrNotExist), errors.Is(err, fs.ErrPermission):
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	default:
		return fmt.Errorf("%w: %w", ErrFailed, err)
	}
}

// moved is an output commit has moved into place. backup is the hard link
// of the file it replaced, if any.
type moved struct {
	name   string
	backup string
}

// commit moves the outputs the tool has written from their temporary
// names into place, all or nothing: if one of them fails, the ones
// already moved are rolled back. Outputs the tool has not written (a
// stream file without streams, the SVG without graphviz) are skipped.
//
// With replace, a file about to be replaced is hard linked to a backup
// name first, which a rollback moves back; otherwise every rename fails
// rather than replacing a file.
func commit(dir string, outputs []string, replace bool) ([]string, error) {
	var done []moved
	files := []string{}
	for _, name := range outputs {
		if _, err := os.Lstat(filepath.Join(dir, TempName(name))); err != nil {
			continue
		}
		entry, err := moveOutput(dir, name, replace)
		if err != nil {
			rollback(dir, done)
			if errors.Is(err, fs.ErrExist) {
				return nil, fmt.Errorf("%w: %s", ErrOutputExists, name)
			}
			return nil, err
		}
		done = append(done, entry)
		files = append(files, name)
	}
	for _, entry := range done {
		if entry.backup != "" {
			_ = os.Remove(entry.backup)
		}
	}
	return files, nil
}

// moveOutput moves one output from its temporary name into place.
func moveOutput(dir, name string, replace bool) (moved, error) {
	temp := filepath.Join(dir, TempName(name))
	final := filepath.Join(dir, name)
	entry := moved{name: name}
	if !replace {
		return entry, renameNoReplace(temp, final)
	}
	backup := filepath.Join(dir, backupPrefix+name)
	_ = os.Remove(backup)
	switch err := os.Link(final, backup); {
	case err == nil:
		entry.backup = backup
	case !errors.Is(err, fs.ErrNotExist):
		return entry, err
	}
	if err := os.Rename(temp, final); err != nil {
		if entry.backup != "" {
			_ = os.Remove(entry.backup)
		}
		return entry, err
	}
	return entry, nil
}

// rollback undoes the moves of commit, restoring replaced files.
func rollback(dir string, done []moved) {
	for _, entry := range slices.Backward(done) {
		final := filepath.Join(dir, entry.name)
		if entry.backup != "" {
			_ = os.Rename(entry.backup, final)
		} else {
			_ = os.Remove(final)
		}
	}
}

// renameNoReplace renames oldpath to newpath unless newpath exists, in one
// atomic step. File systems without RENAME_NOREPLACE fall back to a hard
// link, which fails just the same if newpath exists.
func renameNoReplace(oldpath, newpath string) error {
	err := unix.Renameat2(unix.AT_FDCWD, oldpath, unix.AT_FDCWD, newpath, unix.RENAME_NOREPLACE)
	if !errors.Is(err, unix.EINVAL) && !errors.Is(err, unix.ENOSYS) {
		if errors.Is(err, unix.EEXIST) {
			return fs.ErrExist
		}
		return err
	}
	if err := os.Link(oldpath, newpath); err != nil {
		if errors.Is(err, syscall.EEXIST) {
			return fs.ErrExist
		}
		return err
	}
	return os.Remove(oldpath)
}

// RemoveTempFiles removes the temporary and backup files of Run from dir,
// left behind by a run the controller did not see to its end (a crash or
// a restart while a tool was running).
func RemoveTempFiles(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if IsTempName(entry.Name()) && !entry.IsDir() {
			_ = os.Remove(filepath.Join(dir, entry.Name()))
		}
	}
}

// sequenceCache holds the content of lspgen's sequence cache files before
// a run. lspgen rewrites them in place and not atomically, so they are
// restored from here if the run fails.
type sequenceCache map[string][]byte

// snapshotSequenceCache reads all sequence cache files of dir.
func snapshotSequenceCache(dir string) (sequenceCache, error) {
	cache := sequenceCache{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), sequenceCacheSuffix) {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		cache[entry.Name()] = content
	}
	return cache, nil
}

// restore brings the sequence cache files back to the snapshot: changed
// ones are rewritten and those the run has created are removed.
func (c sequenceCache) restore(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, sequenceCacheSuffix) {
			continue
		}
		if _, ok := c[name]; !ok {
			_ = os.Remove(filepath.Join(dir, name))
		}
	}
	for name, content := range c {
		temp := filepath.Join(dir, TempName(name))
		if err := os.WriteFile(temp, content, sequenceCachePermission); err != nil {
			continue
		}
		_ = os.Rename(temp, filepath.Join(dir, name))
	}
}

// copyFile copies src to the new file dst, with the mode of src; dst
// must not exist yet.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// tailBuffer is an io.Writer keeping only the last limit bytes written.
type tailBuffer struct {
	limit     int
	data      []byte
	truncated bool
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.data = append(b.data, p...)
	if len(b.data) > b.limit {
		b.data = append(b.data[:0], b.data[len(b.data)-b.limit:]...)
		b.truncated = true
	}
	return len(p), nil
}

func (b *tailBuffer) String() string {
	if b.truncated {
		return "[...]\n" + string(b.data)
	}
	return string(b.data)
}
