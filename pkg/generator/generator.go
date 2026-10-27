// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2026, RtBrick, Inc.

// Package generator runs the file generators shipped with the BNG Blaster
// (lspgen, bgpupdate and ldpupdate) inside an instance folder.
//
// The tools are never handed caller-supplied argv. Each tool has a
// whitelist of options (see tools.go) and a request is a JSON object keyed
// by the tool's long option names; Build validates every value against its
// option and renders the command line itself. File names must be plain
// names inside the instance folder, and every file a tool writes is
// first written under a temporary name and only moved into place once the
// tool succeeded (see Run), so bngblaster never reads a half-written file.
//
// Socket mode of lspgen (-S, injecting into a running instance) is
// deliberately not part of this package: it is a long-running process with
// its own lifecycle rather than a file generator.
package generator

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// Kind is the value type of a tool option.
type Kind string

const (
	// KindBool is a flag without value, set if the request value is true.
	KindBool Kind = "bool"
	// KindInt is an integer value, optionally bounded by Min and Max.
	KindInt Kind = "int"
	// KindNumber is a floating point value.
	KindNumber Kind = "number"
	// KindString is a free-form string value (an address, prefix, ...).
	KindString Kind = "string"
	// KindEnum is a string value out of Enum.
	KindEnum Kind = "enum"
	// KindOutput is the name of a file the tool writes into the instance
	// folder.
	KindOutput Kind = "output"
	// KindInput is the name of an existing file in the instance folder the
	// tool reads.
	KindInput Kind = "input"
)

// Request limits. A string option carries an address, prefix, key or
// similar - nothing that needs more than a few hundred characters - and a
// repeated option a handful of values.
const (
	maxStringLength = 256
	maxListLength   = 64
	maxRequestKeys  = 128
)

// Option describes one command line option of a tool. The JSON form is
// served by the generators endpoint, so that clients (the web UI) can
// render a form for a tool without knowing its options in advance.
type Option struct {
	// Name is the long option name without the leading dashes; it is also
	// the key of the option in a request.
	Name string `json:"name"`
	Kind Kind   `json:"type"`
	Help string `json:"help,omitempty"`
	// Group groups related options for display.
	Group string `json:"group,omitempty"`
	// Repeat accepts a list of values, passed as one option per value.
	Repeat bool `json:"repeat,omitempty"`
	// Join accepts a list of values, passed comma-separated in one option.
	Join     bool     `json:"join,omitempty"`
	Required bool     `json:"required,omitempty"`
	Enum     []string `json:"enum,omitempty"`
	Min      *int64   `json:"minimum,omitempty"`
	Max      *int64   `json:"maximum,omitempty"`
	// Default is the tool's own default. For an output it is also the
	// name used if the request omits the option, since the tool would
	// otherwise write that file itself, bypassing the temporary name.
	Default any `json:"default,omitempty"`
	// Appends names the output option this flag makes the tool append to
	// instead of overwrite.
	Appends string `json:"appends,omitempty"`
	// Conflicts names an option that must not be given together with this
	// one, because the tool silently ignores one of them.
	Conflicts string `json:"conflicts,omitempty"`
	// Suffixes marks an output whose value is a base name the tool appends
	// these suffixes to (lspgen writes <name>.dot and <name>.svg). A
	// suffixed file the tool did not write is simply not reported.
	Suffixes []string `json:"suffixes,omitempty"`
}

// Tool describes a generator tool.
type Tool struct {
	Name string `json:"name"`
	// Description is a one-line summary for display.
	Description string `json:"description"`
	// DefaultExecutable is the installed location of the tool.
	DefaultExecutable string `json:"-"`
	// ProbeArgs are the arguments of a cheap invocation that succeeds if
	// the tool is usable, see Probe.
	ProbeArgs []string `json:"-"`
	// SequenceCache marks a tool that keeps sequence numbers in
	// *-sequence-cache.json files in its working directory, which Run
	// restores if the tool fails.
	SequenceCache bool     `json:"-"`
	Options       []Option `json:"options"`
}

// Job is a validated request, ready to run.
type Job struct {
	Tool string
	// Args are the arguments passed to the tool, with every output
	// replaced by its temporary name.
	Args []string
	// Command is the equivalent command line with the final file names,
	// for display and logging.
	Command []string
	// Outputs are the final names of the files the tool writes.
	Outputs []string
	// Appends are the outputs the tool appends to; an existing file is
	// copied to the temporary name before the tool runs.
	Appends []string
	// Inputs are the files the tool reads, which must exist.
	Inputs []string
	// SequenceCache see Tool.SequenceCache.
	SequenceCache bool
}

// Lookup returns the tool with the given name.
func Lookup(name string) (Tool, bool) {
	for _, tool := range Tools {
		if tool.Name == name {
			return tool, true
		}
	}
	return Tool{}, false
}

// Build validates a request for the named tool and renders its command
// line. The request is a JSON object keyed by option name; unknown keys
// are rejected rather than ignored, so a typo does not silently produce a
// different file than intended.
func Build(name string, request []byte) (Job, error) {
	tool, ok := Lookup(name)
	if !ok {
		return Job{}, ErrUnknownTool
	}
	values, err := tool.decode(request)
	if err != nil {
		return Job{}, err
	}
	b := builder{job: Job{Tool: tool.Name, SequenceCache: tool.SequenceCache}, outputs: map[string]bool{}}
	// Options are rendered in the order of the tool's option list, not
	// the request's: lspgen evaluates its arguments in order and the
	// protocol has to come before the options that depend on it.
	for _, option := range tool.Options {
		raw, present, err := option.value(values)
		if err != nil {
			return Job{}, err
		}
		if !present {
			continue
		}
		args, err := option.render(raw)
		if err != nil {
			return Job{}, err
		}
		for _, value := range args {
			if err := b.add(option, value); err != nil {
				return Job{}, err
			}
		}
	}
	if err := b.resolveAppends(tool); err != nil {
		return Job{}, err
	}
	return b.job, nil
}

// decode parses a request into its option values, rejecting unknown
// options. A null value counts as not given.
func (t Tool) decode(request []byte) (map[string]json.RawMessage, error) {
	values := map[string]json.RawMessage{}
	if len(bytes.TrimSpace(request)) > 0 {
		if err := json.Unmarshal(request, &values); err != nil {
			return nil, invalid("request must be a JSON object: %v", err)
		}
	}
	if len(values) > maxRequestKeys {
		return nil, invalid("too many options")
	}
	for key, raw := range values {
		if !slices.ContainsFunc(t.Options, func(o Option) bool { return o.Name == key }) {
			return nil, invalid("unknown option %q", key)
		}
		if string(bytes.TrimSpace(raw)) == "null" {
			delete(values, key)
		}
	}
	for _, option := range t.Options {
		_, set := values[option.Name]
		if _, other := values[option.Conflicts]; set && other {
			return nil, invalid("options %q and %q cannot be combined", option.Name, option.Conflicts)
		}
	}
	return values, nil
}

// value returns the request value of an option. An output with a default
// name is always present: the tool would otherwise write that file itself,
// bypassing its temporary name.
func (o Option) value(values map[string]json.RawMessage) (json.RawMessage, bool, error) {
	if raw, ok := values[o.Name]; ok {
		return raw, true, nil
	}
	if o.Kind == KindOutput && o.Default != nil {
		raw, err := json.Marshal(o.Default)
		return raw, err == nil, err
	}
	if o.Required {
		return nil, false, invalid("option %q is required", o.Name)
	}
	return nil, false, nil
}

// builder collects the command line of a Job.
type builder struct {
	job Job
	// outputs are the files written so far, to reject a file written by
	// two options.
	outputs map[string]bool
}

// add adds one rendered value of an option to the command line. The
// "--name=value" form keeps a value starting with a dash from being
// parsed as an option by getopt or argparse.
func (b *builder) add(option Option, value string) error {
	flag := "--" + option.Name
	switch option.Kind {
	case KindBool:
		b.job.Args = append(b.job.Args, flag)
		b.job.Command = append(b.job.Command, flag)
		return nil
	case KindOutput:
		files := []string{value}
		if len(option.Suffixes) > 0 {
			files = files[:0]
			for _, suffix := range option.Suffixes {
				files = append(files, value+suffix)
			}
		}
		for _, file := range files {
			if b.outputs[file] {
				return invalid("file %q is written by more than one option", file)
			}
			if IsReservedName(file) {
				return invalid("option %q: %q is reserved by the controller", option.Name, file)
			}
			b.outputs[file] = true
		}
		b.job.Outputs = append(b.job.Outputs, files...)
		// TempName(value)+suffix is TempName(value+suffix), so the
		// suffixed files land under their temporary names as well.
		b.job.Args = append(b.job.Args, flag+"="+TempName(value))
		b.job.Command = append(b.job.Command, flag+"="+value)
		return nil
	case KindInput:
		b.job.Inputs = append(b.job.Inputs, value)
	case KindInt, KindNumber, KindString, KindEnum:
	}
	b.job.Args = append(b.job.Args, flag+"="+value)
	b.job.Command = append(b.job.Command, flag+"="+value)
	return nil
}

// resolveAppends records the outputs the tool appends to, for every
// append flag (see Option.Appends) that is set.
func (b *builder) resolveAppends(tool Tool) error {
	for _, option := range tool.Options {
		if option.Appends == "" || !slices.Contains(b.job.Args, "--"+option.Name) {
			continue
		}
		target, ok := renderedValue(b.job.Command, "--"+option.Appends)
		if !ok {
			return invalid("option %q requires %q", option.Name, option.Appends)
		}
		b.job.Appends = append(b.job.Appends, target)
	}
	return nil
}

// render validates the request value of an option and returns its
// rendered values: one per occurrence of the option on the command line,
// an empty slice to omit it, or a single empty string for a set flag.
func (o Option) render(raw json.RawMessage) ([]string, error) {
	if o.Repeat || o.Join {
		return o.renderList(raw)
	}
	if o.Kind == KindBool {
		var set bool
		if err := json.Unmarshal(raw, &set); err != nil {
			return nil, invalid("option %q must be a boolean", o.Name)
		}
		if !set {
			return nil, nil
		}
		return []string{""}, nil
	}
	value, err := o.scalar(raw)
	if err != nil {
		return nil, err
	}
	return []string{value}, nil
}

// renderList renders the values of a Repeat or Join option. A single
// value is accepted as a list of one.
func (o Option) renderList(raw json.RawMessage) ([]string, error) {
	var list []json.RawMessage
	if err := json.Unmarshal(raw, &list); err != nil {
		list = []json.RawMessage{raw}
	}
	if len(list) > maxListLength {
		return nil, invalid("option %q: too many values", o.Name)
	}
	values := make([]string, 0, len(list))
	for _, item := range list {
		value, err := o.scalar(item)
		if err != nil {
			return nil, err
		}
		if o.Join && strings.Contains(value, ",") {
			return nil, invalid("option %q: value %q must not contain a comma", o.Name, value)
		}
		values = append(values, value)
	}
	if o.Join && len(values) > 0 {
		return []string{strings.Join(values, ",")}, nil
	}
	return values, nil
}

// scalar validates and renders one value of an option.
func (o Option) scalar(raw json.RawMessage) (string, error) {
	switch o.Kind {
	case KindInt:
		return o.integer(raw)
	case KindNumber:
		return o.number(raw)
	case KindString, KindEnum, KindOutput, KindInput:
		return o.text(raw)
	case KindBool:
	}
	return "", invalid("option %q has an unsupported type", o.Name)
}

// integer validates and renders an integer value. json.Number also
// accepts a quoted number, which a typed client would never send; only a
// JSON number is an integer here.
func (o Option) integer(raw json.RawMessage) (string, error) {
	var number json.Number
	if bytes.HasPrefix(bytes.TrimSpace(raw), []byte(`"`)) || json.Unmarshal(raw, &number) != nil {
		return "", invalid("option %q must be an integer", o.Name)
	}
	value, err := strconv.ParseInt(number.String(), 10, 64)
	if err != nil {
		return "", invalid("option %q must be an integer", o.Name)
	}
	if (o.Min != nil && value < *o.Min) || (o.Max != nil && value > *o.Max) {
		return "", invalid("option %q is out of range", o.Name)
	}
	return strconv.FormatInt(value, 10), nil
}

// number validates and renders a floating point value.
func (o Option) number(raw json.RawMessage) (string, error) {
	var value float64
	if err := json.Unmarshal(raw, &value); err != nil || math.IsInf(value, 0) || math.IsNaN(value) {
		return "", invalid("option %q must be a number", o.Name)
	}
	if (o.Min != nil && value < float64(*o.Min)) || (o.Max != nil && value > float64(*o.Max)) {
		return "", invalid("option %q is out of range", o.Name)
	}
	return strconv.FormatFloat(value, 'f', -1, 64), nil
}

// text validates a string, enum or file name value.
func (o Option) text(raw json.RawMessage) (string, error) {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", invalid("option %q must be a string", o.Name)
	}
	if err := checkText(value); err != nil {
		return "", invalid("option %q %v", o.Name, err)
	}
	switch o.Kind {
	case KindEnum:
		if !slices.Contains(o.Enum, value) {
			return "", invalid("option %q must be one of %s", o.Name, strings.Join(o.Enum, ", "))
		}
	case KindOutput, KindInput:
		if err := checkFileName(value); err != nil {
			return "", invalid("option %q: %v", o.Name, err)
		}
	case KindBool, KindInt, KindNumber, KindString:
	}
	return value, nil
}

// checkText bounds a string value. The value ends up as a command line
// argument and in the log, neither of which has a use for control
// characters.
func checkText(value string) error {
	if value == "" || len(value) > maxStringLength {
		return fmt.Errorf("must have 1 to %d characters", maxStringLength)
	}
	if strings.ContainsFunc(value, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return errors.New("must not contain control characters")
	}
	return nil
}

// checkFileName accepts only a plain file name inside the instance
// folder. Hidden names are rejected as a whole: they include the
// temporary names of Run, and nothing a tool writes needs to be hidden.
// The controller's own files are rejected too, so a tool can neither
// replace the configuration nor run.pid, which decides what _stop and
// _kill signal.
func checkFileName(name string) error {
	if filepath.Base(name) != name || strings.ContainsRune(name, '/') {
		return errors.New("must be a file name without directory")
	}
	if strings.HasPrefix(name, ".") {
		return errors.New("must not start with a dot")
	}
	if IsReservedName(name) {
		return fmt.Errorf("%q is reserved by the controller", name)
	}
	return nil
}

// IsReservedName reports whether name is one of the controller's own files
// of an instance folder. It mirrors controller.IsRunFile plus the
// configuration; the controller package imports this one, not vice versa,
// and tests there keep both lists in sync.
func IsReservedName(name string) bool {
	switch name {
	case "config.json", "run.pid", "run.sock", "run.json", "run.log",
		"run_report.json", "run.pcap", "run.stderr", "run.stdout":
		return true
	}
	return false
}

// renderedValue returns the value of flag in an argument list rendered by
// builder.add ("--name=value").
func renderedValue(args []string, flag string) (string, bool) {
	for _, arg := range args {
		if value, ok := strings.CutPrefix(arg, flag+"="); ok {
			return value, true
		}
	}
	return "", false
}

// invalid returns an ErrInvalidRequest with the given detail.
func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidRequest, fmt.Sprintf(format, args...))
}
