// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gitea.dev/cli/internal/services"
	"gitea.dev/cli/internal/terminal"

	"github.com/spf13/cobra"
)

// issueActionCommand builds a command that sets one state of each issue named, such as locked, to
// want, and reports it like bh issue close does. BranchSpire applies these to pull requests too. One in
// that state already is reported and left alone.
func (a *application) issueActionCommand(verb, short, long, example, past string, complete cobra.CompletionFunc, state func(*services.Issue) *bool, want bool, act func(ctx context.Context, service services.Issues, number int64) error) *cobra.Command {
	return &cobra.Command{
		Use: verb + " {<number> | <url>}...", Short: short, Long: long, Example: example,
		Annotations: map[string]string{"bh.arguments": issueArguments}, Args: usageArgs(cobra.MinimumNArgs(1)), ValidArgsFunction: complete,
		RunE: func(cmd *cobra.Command, args []string) error {
			target, numbers, err := a.resolveRefs(cmd, args, issueRef)
			if err != nil {
				return err
			}
			service := services.Issues{Client: target.client, Repository: target.Repo}
			results, failures := applyBatch(cmd, numbers, issueLabel, func(n int64) (services.Issue, error) {
				item, err := service.ViewPinned(cmd.Context(), n)
				if err != nil {
					return item, itemError(err, target.Repo)
				}
				if *state(&item) == want {
					a.warn(cmd, "%s %s is already %s", titleCase(discussionNoun(item.Kind)), refLabel(target.Repo, n, item.Title), strings.ToLower(past))
					return item, nil
				}
				if err := act(cmd.Context(), service, n); err != nil {
					return item, itemError(err, target.Repo)
				}
				*state(&item) = want
				a.done(cmd, "%s %s %s", past, discussionNoun(item.Kind), refLabel(target.Repo, n, item.Title))
				return item, nil
			}, nil)
			return errors.Join(append(failures, writeBatchJSON(a, cmd, len(numbers), results))...)
		},
	}
}

func issueLocked(item *services.Issue) *bool { return &item.Locked }
func issuePinned(item *services.Issue) *bool { return &item.Pinned }

func (a *application) issueLockCommand() *cobra.Command {
	var reason string
	cmd := a.issueActionCommand("lock", "Lock the conversation of issues",
		`Lock the conversation of issues or pull requests, so that only people with write access can
comment on them. One that is locked already is reported and keeps its reason.

BranchSpire does not check the reason: it offers Too heated, Off-topic, Resolved and Spam by default,
and administrators can change that.`,
		`$ bh issue lock 12
$ bh issue lock 12 13 --reason "Too heated"`, "Locked", a.completeIssues("open"), issueLocked, true,
		func(ctx context.Context, service services.Issues, n int64) error {
			return service.SetLocked(ctx, n, true, reason)
		})
	cmd.Flags().StringVarP(&reason, "reason", "r", "", "Reason for locking, shown on the issue, such as \"Resolved\"")
	return cmd
}

func (a *application) issueUnlockCommand() *cobra.Command {
	return a.issueActionCommand("unlock", "Unlock the conversation of issues",
		"Unlock the conversation of issues or pull requests, so that everyone who can see them can comment again. One that is not locked is reported.",
		"$ bh issue unlock 12", "Unlocked", a.completeIssues("all"), issueLocked, false,
		func(ctx context.Context, service services.Issues, n int64) error {
			return service.SetLocked(ctx, n, false, "")
		})
}

func (a *application) issuePinCommand() *cobra.Command {
	return a.issueActionCommand("pin", "Pin issues to the top of the list",
		"Pin issues or pull requests to the top of the repository's lists. BranchSpire allows only a few pinned items at a time. One that is pinned already is reported.",
		"$ bh issue pin 12", "Pinned", a.completeIssues("open"), issuePinned, true,
		func(ctx context.Context, service services.Issues, n int64) error {
			return service.SetPinned(ctx, n, true)
		})
}

func (a *application) issueUnpinCommand() *cobra.Command {
	return a.issueActionCommand("unpin", "Unpin issues",
		"Unpin issues or pull requests from the top of the repository's lists. One that is not pinned is reported.",
		"$ bh issue unpin 12", "Unpinned", a.completeIssues("all"), issuePinned, false,
		func(ctx context.Context, service services.Issues, n int64) error {
			return service.SetPinned(ctx, n, false)
		})
}

func (a *application) issueDeleteCommand() *cobra.Command {
	return &cobra.Command{
		Use: "delete {<number> | <url>}...", Aliases: []string{"rm"}, Short: "Delete issues",
		Long: `Delete issues for good, with their comments, attachments and time tracking. This cannot be
undone and needs the permission to delete issues, which BranchSpire gives repository owners and
administrators. To only stop work on one, use bh issue close. To delete a pull request, use bh pr delete.`,
		Example: `$ bh issue delete 12
$ bh issue delete 12 13 --yes`,
		Annotations: map[string]string{"bh.arguments": issueArguments, "bh.text": "issue deletion does not return a resource", "bh.confirm": "true"},
		Args:        usageArgs(cobra.MinimumNArgs(1)), ValidArgsFunction: a.completeIssues("all"),
		RunE: func(cmd *cobra.Command, args []string) error {
			target, numbers, err := a.resolveRefs(cmd, args, issueRef)
			if err != nil {
				return err
			}
			service := services.Issues{Client: target.client, Repository: target.Repo}
			// Look at every issue first, so that one that cannot be deleted stops the batch before any is.
			items := make([]services.Issue, len(numbers))
			for i, n := range numbers {
				if items[i], err = service.View(cmd.Context(), n); err != nil {
					return fmt.Errorf("%s: %w", issueLabel(n), itemError(err, target.Repo))
				}
				if items[i].Kind == services.KindPull {
					return fmt.Errorf("%s is a pull request; use bh pr delete %d", refLabel(target.Repo, n, items[i].Title), n)
				}
			}
			prompt := "Delete issue " + refLabel(target.Repo, numbers[0], items[0].Title) + "?"
			if len(numbers) > 1 {
				prompt = fmt.Sprintf("Delete %d issues of %s (%s)?", len(numbers), target.Repo, strings.Join(shortRefs(numbers), ", "))
			}
			if err := a.confirmAction(cmd, terminal.Confirmation{Prompt: prompt}); err != nil {
				return err
			}
			deleted, failures := applyBatch(cmd, items, func(item services.Issue) string { return issueLabel(item.Number) }, func(item services.Issue) (services.Issue, error) {
				if err := service.Delete(cmd.Context(), item.Number); err != nil {
					return item, itemError(err, target.Repo)
				}
				return item, nil
			}, func(item services.Issue) {
				a.done(cmd, "Deleted issue %s", refLabel(target.Repo, item.Number, item.Title))
			})
			if len(failures) > 0 && !a.deps.Streams.OutTTY { // done was silent, and scripts must learn what is gone
				for _, item := range deleted {
					a.warn(cmd, "Deleted issue %s", refLabel(target.Repo, item.Number, item.Title))
				}
			}
			return errors.Join(failures...)
		},
	}
}

// shortRefs writes issue numbers as #12.
func shortRefs(numbers []int64) []string {
	refs := make([]string, len(numbers))
	for i, n := range numbers {
		refs[i] = fmt.Sprintf("#%d", n)
	}
	return refs
}
