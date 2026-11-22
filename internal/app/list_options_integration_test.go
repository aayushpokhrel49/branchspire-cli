// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"gitea.dev/cli/internal/services"

	"github.com/stretchr/testify/require"
)

func TestGiteaIssueCreateSetsDependencies(t *testing.T) {
	_, deps, out, diagnostics := giteaDeps(t)
	execute := outputExecutor(t, &deps, out, diagnostics)
	create := func(prefix string) services.Repository {
		var repo services.Repository
		require.NoError(t, json.Unmarshal(execute("repo", "create", fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano()), "--auto-init", "--private", "--jq", "."), &repo))
		t.Cleanup(func() {
			if err := Execute(context.Background(), deps, []string{"repo", "delete", "-R", repo.FullName, "--yes"}); err != nil {
				t.Errorf("repository cleanup: %v", err)
			}
		})
		return repo
	}
	repo, other := create("bh-create-deps"), create("bh-create-deps-other")
	issue := func(r services.Repository, title string, args ...string) string {
		var item services.Issue
		require.NoError(t, json.Unmarshal(execute(append([]string{"issue", "create", "-R", r.FullName, "--title", title, "--body", "b", "--jq", "."}, args...)...), &item))
		return strconv.FormatInt(item.Number, 10)
	}
	first, second := issue(repo, "First"), issue(repo, "Second")
	foreign := issue(other, "In the other repository")
	dependencies := func(n string) string {
		return string(execute("issue", "view", n, "-R", repo.FullName, "--json", "blocked_by,blocks"))
	}

	created := issue(repo, "Waits and blocks", "--blocked-by", first+","+other.FullName+"#"+foreign, "--blocking", second)
	require.JSONEq(t, `{"blocked_by":[{"repository":"`+repo.FullName+`","number":`+first+`},{"repository":"`+other.FullName+`","number":`+foreign+`}],"blocks":[{"repository":"`+repo.FullName+`","number":`+second+`}]}`,
		dependencies(created), "the new issue waits for an issue of this repository and one of another, and holds up the second")
	require.JSONEq(t, `{"blocked_by":[{"repository":"`+repo.FullName+`","number":`+created+`}],"blocks":[]}`, dependencies(second), "the other end sees it too")

	// An issue that does not exist is refused before the new one is created.
	err := Execute(context.Background(), deps, []string{"issue", "create", "-R", repo.FullName, "--title", "Waits for nothing", "--blocked-by", "9999", "--no-input", "--jq", "."})
	require.ErrorContains(t, err, "--blocked-by 9999: issue #9999 not found in "+repo.FullName)
	titles := strings.TrimSpace(string(execute("issue", "list", "-R", repo.FullName, "--state", "all", "--json", "title", "--jq", `[.[].title] | join(",")`)))
	require.NotContains(t, titles, "Waits for nothing", "the issue was not created")

	// Bad values are refused before anything is created.
	before := strings.TrimSpace(string(execute("issue", "list", "-R", repo.FullName, "--state", "all", "--json", "number", "--jq", "length")))
	require.ErrorContains(t, Execute(context.Background(), deps, []string{"issue", "create", "-R", repo.FullName, "--title", "x", "--blocked-by", "abc", "--no-input"}), "--blocked-by takes issues")
	require.ErrorContains(t, Execute(context.Background(), deps, []string{"issue", "create", "-R", repo.FullName, "--title", "x", "--web", "--blocking", "1", "--no-input"}), "--web")
	require.Equal(t, before, strings.TrimSpace(string(execute("issue", "list", "-R", repo.FullName, "--state", "all", "--json", "number", "--jq", "length"))))
}

func TestGiteaLabelListSearchSortAndOrganization(t *testing.T) {
	_, deps, out, diagnostics := giteaDeps(t)
	execute := outputExecutor(t, &deps, out, diagnostics)
	var repo services.Repository
	require.NoError(t, json.Unmarshal(execute("repo", "create", fmt.Sprintf("bh-label-list-%d", time.Now().UnixNano()), "--auto-init", "--private", "--jq", "."), &repo))
	t.Cleanup(func() {
		if err := Execute(context.Background(), deps, []string{"repo", "delete", "-R", repo.FullName, "--yes"}); err != nil {
			t.Errorf("repository cleanup: %v", err)
		}
	})
	scoped := func(args ...string) []byte { return execute(append(args, "-R", repo.FullName)...) }
	scoped("label", "create", "kind/bug", "--color", "ff0000", "--description", "Something is broken")
	scoped("label", "create", "Docs", "--color", "00ff00", "--description", "About the BUG tracker")
	scoped("label", "create", "feature", "--color", "0000ff")
	scoped("label", "create", "bugfix", "--color", "123456", "--description", "A fix")
	names := func(args ...string) string {
		t.Helper()
		out.Reset()
		diagnostics.Reset()
		require.NoError(t, Execute(context.Background(), deps, append([]string{"label", "list", "-R", repo.FullName, "--json", "name", "--jq", `[.[].name] | join(",")`}, args...)))
		return strings.TrimSpace(out.String())
	}

	own := strings.Split(names(), ",")
	require.ElementsMatch(t, []string{"Docs", "bugfix", "feature", "kind/bug"}, own, "whether capitals come first depends on the database")
	require.ElementsMatch(t, []string{"Docs", "bugfix", "kind/bug"}, strings.Split(names("--search", "bug"), ","), "the name or the description, without regard to case")
	require.Equal(t, "bugfix,Docs,kind/bug", names("--search", "BUG", "--sort", "name"), "sorted by name without regard to case")
	require.Equal(t, "kind/bug,feature,Docs,bugfix", names("--sort", "name", "--order", "desc"))
	require.Equal(t, "kind/bug,Docs,feature,bugfix", names("--sort", "created"), "by creation: the order the labels were made in")
	require.Equal(t, "bugfix,feature,Docs,kind/bug", names("--sort", "created", "--order", "desc"))
	slices.Reverse(own)
	require.Equal(t, strings.Join(own, ","), names("--order", "desc"), "--order alone reverses BranchSpire's order")
	require.Equal(t, "bugfix,Docs", names("--sort", "name", "--limit", "2"), "a page of the sorted labels")
	require.Equal(t, "feature,kind/bug", names("--sort", "name", "--limit", "2", "--page", "2"))
	require.Empty(t, names("--search", "nothing-like-this"))

	org := fmt.Sprintf("bh-label-org-%d", time.Now().UnixNano())
	execute("org", "create", org)
	t.Cleanup(func() { _ = Execute(context.Background(), deps, []string{"org", "delete", org, "--yes"}) })
	execute("label", "create", "org-wide", "--org", org, "--color", "abcdef", "--description", "For every repository")
	execute("label", "create", "other", "--org", org, "--color", "fedcba")
	out.Reset()
	require.NoError(t, Execute(context.Background(), deps, []string{"label", "list", "--org", org, "--search", "every", "--json", "name", "--jq", ".[].name"}))
	require.Equal(t, "org-wide\n", out.String(), "the same options on the labels of an organization")
}

func TestGiteaRepoListFilters(t *testing.T) {
	_, deps, out, diagnostics := giteaDeps(t)
	execute := outputExecutor(t, &deps, out, diagnostics)
	prefix := fmt.Sprintf("bh-filters-%d", time.Now().UnixNano())
	newRepo := func(name string, args ...string) string {
		var repo services.Repository
		require.NoError(t, json.Unmarshal(execute(append([]string{"repo", "create", prefix + "-" + name, "--auto-init", "--jq", "."}, args...)...), &repo))
		t.Cleanup(func() {
			if err := Execute(context.Background(), deps, []string{"repo", "delete", repo.FullName, "--yes"}); err != nil {
				t.Errorf("repository cleanup: %v", err)
			}
		})
		return repo.FullName
	}
	public := newRepo("public")
	newRepo("private", "--private")
	archived := newRepo("archived")
	execute("repo", "archive", archived, "--yes")
	golang := newRepo("go")
	deps.Streams.In = strings.NewReader(fmt.Sprintf(`{"branch":"main","content":%q,"message":"go"}`, base64.StdEncoding.EncodeToString([]byte("package main\n\nfunc main() {}\n"))))
	execute("api", "repos/"+golang+"/contents/main.go", "--input", "-")
	execute("api", "-X", "PUT", "repos/"+golang+"/topics", "-F", `topics=["cli","gitea"]`)
	execute("api", "-X", "PUT", "repos/"+public+"/topics", "-F", `topics=["cli"]`)
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(500 * time.Millisecond) {
		if strings.TrimSpace(string(execute("api", "repos/"+golang, "--jq", ".language"))) == "Go" {
			break
		}
	}
	mine := func(args ...string) string {
		t.Helper()
		out.Reset()
		diagnostics.Reset()
		require.NoError(t, Execute(context.Background(), deps, append([]string{
			"repo", "list", "--limit", "100", "--json", "full_name",
			"--jq", fmt.Sprintf(`[.[] | .full_name | select(contains("%s")) | sub(".*%s-"; "")] | sort | join(",")`, prefix, prefix),
		}, args...)))
		return strings.TrimSpace(out.String())
	}

	require.Equal(t, "archived,go,private,public", mine())
	require.Equal(t, "archived,go,public", mine("--visibility", "public"))
	require.Equal(t, "private", mine("--visibility", "private"))
	require.Equal(t, "archived", mine("--archived"))
	require.Equal(t, "go,private,public", mine("--no-archived"))
	require.Equal(t, "go", mine("--language", "go"), "a language is matched without regard to case")
	require.Equal(t, "go,public", mine("--topic", "CLI"))
	require.Equal(t, "go", mine("--topic", "cli", "--topic", "gitea"), "every topic must match")
	require.Equal(t, "go", mine("--no-archived", "--visibility", "public", "--language", "Go", "--topic", "gitea"))
	require.Empty(t, mine("--language", "Zig"))
	require.ErrorContains(t, Execute(context.Background(), deps, []string{"repo", "list", "--archived", "--no-archived"}), "--archived cannot be combined with --no-archived")
}
