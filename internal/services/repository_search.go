// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package services

import (
	"context"
	"slices"
	"strconv"
	"strings"

	gitea "gitea.dev/sdk"
)

// RepositoryListOptions selects a repository collection and page.
type RepositoryListOptions struct {
	Owner, Kind      string
	Starred, Watched bool
	Limit, Page      int
	// Visibility is public, internal or private. Archived keeps only the archived repositories when true and
	// leaves them out when false. Topics must all be topics of a repository, Language its main
	// language, without regard to case. BranchSpire does not filter these lists, so they are filtered here.
	Visibility string
	Archived   *bool
	Topics     []string
	Language   string
}

// keeps reports whether a repository passes the filters of opts.
func (o RepositoryListOptions) keeps(item Repository) bool {
	if !repositoryKind(item, o.Kind) {
		return false
	}
	switch {
	case o.Visibility == "public" && (item.Private || item.Internal), o.Visibility == "internal" && !item.Internal, o.Visibility == "private" && !item.Private:
		return false
	case o.Archived != nil && item.Archived != *o.Archived:
		return false
	case o.Language != "" && !strings.EqualFold(item.Language, o.Language):
		return false
	}
	for _, topic := range o.Topics {
		if !slices.ContainsFunc(item.Topics, func(have string) bool { return strings.EqualFold(have, topic) }) {
			return false
		}
	}
	return true
}

// filtered reports whether opts has a filter that the server does not apply.
func (o RepositoryListOptions) filtered() bool {
	return o.Kind != "" || o.Visibility != "" || o.Archived != nil || len(o.Topics) > 0 || o.Language != ""
}

// RepositorySearchOptions supplies explicit repository search filters.
type RepositorySearchOptions struct {
	Query, Owner, Kind, Sort, Order string
	Topic                           bool
	Private, Archived               *bool
	Limit, Page                     int
}

func (s Repositories) ownerID(ctx context.Context, owner string) (int64, bool, error) {
	if id, isOrg, err := organization(ctx, s.Client, owner); err != nil || isOrg {
		return id, isOrg, err
	}
	user, _, err := call(ctx, s.Client, func(ctx context.Context, c *gitea.Client) (*gitea.User, *gitea.Response, error) {
		return c.Users.GetUserInfo(ctx, owner)
	})
	if err != nil {
		return 0, false, Classify(err, "user", strconv.Quote(owner), "")
	}
	return user.ID, false, nil
}

func repositoryKind(item Repository, kind string) bool {
	switch kind {
	case "fork":
		return item.Fork
	case "mirror":
		return item.Mirror
	case "source":
		return !item.Fork && !item.Mirror
	default:
		return true
	}
}

// List returns repositories accessible to the account, or its selected collection.
// It reports whether more repositories follow the selected page.
func (s Repositories) List(ctx context.Context, opts RepositoryListOptions) ([]Repository, bool, error) {
	page, err := s.ListPage(ctx, opts)
	return page.Items, page.More, err
}

// ListPage is List that also reports the size of the collection when the server counts it.
// The server has no filters, so they are applied on the client, reading pages until the requested
// one is full.
func (s Repositories) ListPage(ctx context.Context, opts RepositoryListOptions) (Page[Repository], error) {
	isOrg := false
	var err error
	if opts.Owner != "" {
		if _, isOrg, err = s.ownerID(ctx, opts.Owner); err != nil {
			return Page[Repository]{}, err
		}
	}
	fetch := func(limit, page int) (Page[Repository], error) {
		window, err := callPage(ctx, s.Client, limit, page, func(ctx context.Context, c *gitea.Client, list gitea.ListOptions) ([]*gitea.Repository, *gitea.Response, error) {
			switch {
			case opts.Owner == "" && opts.Starred:
				return c.Repositories.ListMyStarredRepos(ctx, list)
			case opts.Owner == "" && opts.Watched:
				return c.Repositories.ListMyWatchedRepos(ctx, list)
			case isOrg:
				return c.Repositories.ListOrgRepos(ctx, opts.Owner, gitea.ListOrgReposOptions{ListOptions: list})
			case opts.Owner != "":
				return c.Repositories.ListUserRepos(ctx, opts.Owner, gitea.ListReposOptions{ListOptions: list})
			default:
				return c.Repositories.ListMyRepos(ctx, gitea.ListReposOptions{ListOptions: list})
			}
		})
		return convertPage(window, repositoryFrom), err
	}
	if !opts.filtered() {
		page, err := fetch(opts.Limit, opts.Page)
		if err != nil {
			return Page[Repository]{}, err
		}
		return page, nil
	}
	return filteredPage(opts.Limit, opts.Page, fetch, opts.keeps)
}

// Search searches repository names, descriptions or topics within one instance.
func (s Repositories) Search(ctx context.Context, opts RepositorySearchOptions) (Page[Repository], error) {
	options := gitea.SearchRepoOptions{
		Keyword: opts.Query, KeywordIsTopic: opts.Topic, KeywordInDescription: true,
		IsPrivate: opts.Private, IsArchived: opts.Archived, Type: gitea.RepoType(opts.Kind),
	}
	if opts.Sort != "" {
		options.Sort, options.Order = opts.Sort, opts.Order
	}
	if opts.Owner != "" {
		id, _, err := s.ownerID(ctx, opts.Owner)
		if err != nil {
			return Page[Repository]{}, err
		}
		options.OwnerID = id
	}
	// BranchSpire groups results by owner ahead of the requested sort, so an explicit sort cannot keep it.
	// The account only orders the results, so a token that cannot read it still searches.
	if opts.Sort == "" {
		if account, _, err := call(ctx, s.Client, func(ctx context.Context, c *gitea.Client) (*gitea.User, *gitea.Response, error) {
			return c.Users.GetMyUserInfo(ctx)
		}); err == nil {
			options.PrioritizedByOwnerID = account.ID
		}
	}
	window, err := callPage(ctx, s.Client, opts.Limit, opts.Page, func(ctx context.Context, c *gitea.Client, list gitea.ListOptions) ([]*gitea.Repository, *gitea.Response, error) {
		options.ListOptions = list
		return c.Repositories.SearchRepos(ctx, options)
	})
	if err != nil {
		return Page[Repository]{}, err
	}
	return convertPage(window, repositoryFrom), nil
}
