// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"gitea.dev/cli/internal/clierr"
	"gitea.dev/cli/internal/services"

	"github.com/stretchr/testify/require"
)

func TestIssueCreateSetsDependencies(t *testing.T) {
	var mu sync.Mutex
	var writes []string
	failBlocking := false
	server := newAuthenticatedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/repos/alice/project")
		switch {
		case r.Method == http.MethodGet && path == "":
			_, _ = w.Write([]byte(`{"full_name":"alice/project","has_issues":true}`))
		case r.Method == http.MethodGet && path == "/issues/404":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"issue does not exist [id: 0, repo_id: 1, index: 404]"}`))
		case r.Method == http.MethodGet && (strings.HasPrefix(path, "/issues/") || r.URL.Path == "/api/v1/repos/bob/lib/issues/4"):
			_, _ = w.Write([]byte(`{"number":9,"title":"Other","state":"open"}`))
		case r.Method == http.MethodPost && path == "/issues":
			mu.Lock()
			writes = append(writes, "POST /issues")
			mu.Unlock()
			_, _ = w.Write([]byte(`{"number":21,"title":"New","state":"open","html_url":"http://x/21"}`))
		case r.Method == http.MethodPost && (path == "/issues/21/dependencies" || path == "/issues/21/blocks"):
			var meta map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&meta))
			mu.Lock()
			writes = append(writes, fmt.Sprintf("POST %s %v/%v#%v", path, meta["owner"], meta["repo"], meta["index"]))
			mu.Unlock()
			if failBlocking && path == "/issues/21/blocks" {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"message":"issue does not exist [id: 0, repo_id: 0, index: 300]"}`))
				return
			}
			_, _ = w.Write([]byte(`{"number":9}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	run := func(args ...string) (string, error) {
		mu.Lock()
		writes = nil
		mu.Unlock()
		deps, out, _ := serverDeps(t, server.URL)
		err := Execute(context.Background(), deps, append([]string{"issue", "create", "-R", "alice/project", "--title", "New", "--no-input", "--json", "number"}, args...))
		return out.String(), err
	}

	out, err := run("--blocked-by", "9,bob/lib#4", "--blocking", "#12")
	require.NoError(t, err)
	require.JSONEq(t, `{"number":21}`, out)
	require.Equal(t, []string{"POST /issues", "POST /issues/21/dependencies alice/project#9", "POST /issues/21/dependencies bob/lib#4", "POST /issues/21/blocks alice/project#12"}, writes,
		"the issue is created first, then it is made to wait for 9 and 4 and to hold up 12")

	deps, _, _ := serverDeps(t, server.URL)
	writes = nil
	require.NoError(t, Execute(context.Background(), deps, []string{"issue", "create", "-R", "project", "--title", "New", "--no-input", "--blocked-by", "9"}))
	require.Contains(t, writes, "POST /issues/21/dependencies alice/project#9", "a bare repository name takes its owner from the account")

	failBlocking = true
	_, err = run("--blocked-by", "9", "--blocking", "300")
	require.ErrorContains(t, err, "issue #21 was created, but follow-up steps failed")
	require.ErrorContains(t, err, "set them later with: bh issue edit 21 --add-blocked-by alice/project#9 --add-blocking alice/project#300")
	failBlocking = false

	_, err = run("--blocked-by", "9", "--blocking", "404")
	require.Equal(t, clierr.Usage, clierr.ExitCode(err))
	require.ErrorContains(t, err, "--blocking 404: issue #404 not found in alice/project")
	require.Empty(t, writes, "a missing issue is refused before the issue is created")

	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--blocked-by", "lib#4"}, "--blocked-by takes issues as N, #N, OWNER/REPO#N or an issue URL"},
		{[]string{"--blocking", "0"}, "--blocking takes issues as N, #N, OWNER/REPO#N or an issue URL"},
		{[]string{"--blocked-by", "x"}, "--blocked-by takes issues"},
	} {
		_, err := run(tc.args...)
		require.Equal(t, clierr.Usage, clierr.ExitCode(err), tc.args)
		require.ErrorContains(t, err, tc.want)
		require.Empty(t, writes, "a bad value is refused before the issue is created")
	}
	_, err = run("--web", "--blocked-by", "9")
	require.Equal(t, clierr.Usage, clierr.ExitCode(err), "the browser takes no dependencies")
	require.Empty(t, writes)
}

func TestLabelListSearchSortAndWeb(t *testing.T) {
	server := newAuthenticatedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/repos/alice/project":
			_, _ = w.Write([]byte(`{"full_name":"alice/project"}`))
		case "/api/v1/repos/alice/project/labels":
			if r.URL.Query().Get("page") != "1" && r.URL.Query().Get("page") != "" {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			_, _ = w.Write([]byte(`[{"id":1,"name":"kind/bug","color":"ff0000","description":"Broken"},{"id":2,"name":"docs","color":"00ff00","description":"A bug in the docs"},{"id":3,"name":"feature","color":"0000ff","description":""}]`))
		default:
			t.Errorf("unexpected %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	run := func(args ...string) (string, error) {
		deps, out, _ := serverDeps(t, server.URL)
		browser := &recordingBrowser{}
		deps.Browser = browser
		err := Execute(context.Background(), deps, append([]string{"label", "list", "-R", "alice/project", "--no-input"}, args...))
		return out.String() + browser.resource, err
	}

	out, err := run("--search", "BUG", "--sort", "name", "--order", "desc", "--json", "name", "--jq", "[.[].name] | join(\",\")")
	require.NoError(t, err)
	require.Equal(t, "kind/bug,docs\n", out)
	out, err = run("-S", "feature", "--json", "id", "--jq", ".[].id")
	require.NoError(t, err)
	require.Equal(t, "3\n", out)

	out, err = run("--web")
	require.NoError(t, err)
	require.True(t, strings.HasSuffix(out, "/alice/project/labels"), out)
	_, err = run("--web", "--org", "acme")
	require.Error(t, err, "--org cannot be combined with --repo")
	deps, _, _ := serverDeps(t, server.URL)
	browser := &recordingBrowser{}
	deps.Browser = browser
	require.NoError(t, Execute(context.Background(), deps, []string{"label", "list", "--org", "acme", "--web", "--no-input"}))
	require.True(t, strings.HasSuffix(browser.resource, "/org/acme/settings/labels"), browser.resource)
}

func TestRepoListFilters(t *testing.T) {
	server := newAuthenticatedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/user/repos" {
			t.Errorf("unexpected %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Query().Get("page") != "1" && r.URL.Query().Get("page") != "" {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		_, _ = w.Write([]byte(`[
			{"id":1,"full_name":"a/one","private":false,"archived":false,"language":"Go","topics":["cli"]},
			{"id":2,"full_name":"a/two","private":true,"archived":false,"language":"Go","topics":["cli","gitea"]},
			{"id":3,"full_name":"a/three","private":false,"archived":true,"language":"Rust","topics":["cli"]},
			{"id":4,"full_name":"a/four","private":false,"internal":true,"archived":false}]`))
	}))
	defer server.Close()
	run := func(args ...string) (string, error) {
		deps, out, _ := serverDeps(t, server.URL)
		err := Execute(context.Background(), deps, append([]string{"repo", "list", "--no-input", "--json", "full_name", "--jq", "[.[].full_name] | join(\",\")"}, args...))
		return strings.TrimSpace(out.String()), err
	}
	for args, want := range map[string]string{
		"--visibility public":                       "a/one,a/three",
		"--visibility private":                      "a/two",
		"--visibility internal":                     "a/four",
		"--archived":                                "a/three",
		"--no-archived":                             "a/one,a/two,a/four",
		"--archived=false":                          "a/one,a/two,a/four",
		"--language go":                             "a/one,a/two",
		"-l Rust":                                   "a/three",
		"--topic cli --topic gitea":                 "a/two",
		"--no-archived --visibility public":         "a/one",
		"--language Go --topic gitea --type source": "a/two",
	} {
		out, err := run(strings.Fields(args)...)
		require.NoError(t, err, args)
		require.Equal(t, want, out, args)
	}
	_, err := run("--archived", "--no-archived")
	require.Equal(t, clierr.Usage, clierr.ExitCode(err))
	require.ErrorContains(t, err, "--archived cannot be combined with --no-archived")
	_, err = run("--visibility", "limited")
	require.Equal(t, clierr.Usage, clierr.ExitCode(err))
	require.Equal(t, []string{"internal", "fork"}, repositoryInfo(services.Repository{Internal: true, Fork: true}))
}
