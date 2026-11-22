// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package app

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"gitea.dev/cli/internal/clierr"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

const schemaGolden = "testdata/schema.json.txt"

func runSchema(t *testing.T, args ...string) schemaReport {
	t.Helper()
	deps, out, _ := testDeps(t)
	require.NoError(t, Execute(context.Background(), deps, append([]string{"schema"}, args...)))
	var report schemaReport
	require.NoError(t, json.Unmarshal(out.Bytes(), &report))
	return report
}

func describeSchema(c commandSchema) []string {
	fields := make([]string, len(c.Fields))
	for i, f := range c.Fields {
		fields[i] = f.Name + ":" + f.Type
		if f.Format != "" {
			fields[i] += "(" + f.Format + ")"
		}
		if f.Items != "" {
			fields[i] += "<" + f.Items + ">"
		}
		if f.Nullable {
			fields[i] += "?"
		}
	}
	return fields
}

func TestSchemaCommand(t *testing.T) {
	report := runSchema(t, "issue", "list")
	require.Equal(t, schemaVersion, report.Version)
	require.Len(t, report.Commands, 1)
	require.Equal(t, "bh issue list", report.Commands[0].Command)
	described := describeSchema(report.Commands[0])
	require.Contains(t, described, "number:integer")
	require.Contains(t, described, "title:string")
	require.Contains(t, described, "labels:array<object>")
	require.Contains(t, described, "created_at:string(date-time)?")

	require.Len(t, runSchema(t, "pr", "list").Commands, 1)

	all := runSchema(t)
	require.Greater(t, len(all.Commands), 50)
	for _, c := range all.Commands {
		command := mustFind(t, strings.Fields(c.Command)[1:])
		require.True(t, outputsJSON(command), c.Command)
		require.Len(t, c.Fields, len(commandOutputFields(command)), c.Command)
		if fieldsDeclaration(command).Annotations[outputTypeAnnotation] != "" { // varyingJSONOutput has no types
			for _, field := range c.Fields {
				require.NotEmpty(t, field.Type, "%s: the field %s has no type", c.Command, field.Name)
			}
		}
	}

	for _, args := range [][]string{{"bogus"}, {"issue", "list", "extra"}, {"version"}, {"branch", "delete"}} {
		deps, out, _ := testDeps(t)
		err := Execute(context.Background(), deps, append([]string{"schema"}, args...))
		require.Equal(t, clierr.Usage, clierr.ExitCode(err), "%v: %v", args, err)
		require.Empty(t, out.String())
	}
}

// TestJSONFieldsAreNeverLeftOut enforces that every item has every field its command declares,
// null when it has no value: no output type leaves a field out when it is empty, and a time, or
// the user who did something, is nullable rather than "".
func TestJSONFieldsAreNeverLeftOut(t *testing.T) {
	deps, _, _ := testDeps(t)
	NewRoot(deps) // registers the output types
	seen := map[reflect.Type]bool{}
	var check func(path string, t reflect.Type)
	check = func(path string, typ reflect.Type) {
		for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array || typ.Kind() == reflect.Map {
			typ = typ.Elem()
		}
		if typ.Kind() != reflect.Struct || typ == timeType || seen[typ] {
			return
		}
		seen[typ] = true
		structJSONFields(typ, func(name, options string, field reflect.Type) {
			for option := range strings.SplitSeq(options, ",") {
				require.NotContains(t, []string{"omitempty", "omitzero"}, option, "%s.%s leaves the field out", path, name)
			}
			if strings.HasSuffix(name, "_at") || strings.HasSuffix(name, "_by") || strings.HasSuffix(name, "_login") {
				require.NotEqual(t, reflect.String, field.Kind(), "%s.%s must be null without a value", path, name)
			}
			check(path+"."+name, field)
		})
	}
	outputTypes.Range(func(_, value any) bool {
		typ, ok := value.(reflect.Type)
		require.True(t, ok)
		check(typ.String(), typ)
		return true
	})
	require.Greater(t, len(seen), 50)
}

func mustFind(t *testing.T, path []string) (command *cobra.Command) {
	t.Helper()
	deps, _, _ := testDeps(t)
	command, _, err := NewRoot(deps).Find(path)
	require.NoError(t, err)
	return command
}

// TestSchemaIsBackwardCompatible enforces the promise of bh help formatting: a command's JSON
// fields are never removed and keep their type. A new field or command passes; to accept an
// intended break or to record additions, run UPDATE_SCHEMA=1 go test -run TestSchemaIsBackwardCompatible ./internal/app/.
func TestSchemaIsBackwardCompatible(t *testing.T) {
	current := map[string][]string{}
	var commands []string
	for _, c := range runSchema(t).Commands {
		current[c.Command] = describeSchema(c)
		commands = append(commands, c.Command)
	}
	slices.Sort(commands)
	var golden strings.Builder
	for _, name := range commands {
		golden.WriteString(name + "\t" + strings.Join(current[name], ",") + "\n")
	}
	if os.Getenv("UPDATE_SCHEMA") != "" {
		require.NoError(t, os.WriteFile(schemaGolden, []byte(golden.String()), 0o644))
	}
	data, err := os.ReadFile(schemaGolden)
	require.NoError(t, err)
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		name, fields, _ := strings.Cut(line, "\t")
		now, ok := current[name]
		require.True(t, ok, "%s no longer outputs JSON", name)
		for field := range strings.SplitSeq(fields, ",") {
			require.Contains(t, now, field, "%s lost or changed the JSON field %s", name, field)
		}
	}
}
