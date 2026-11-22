// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package app

import (
	"context"
	"strings"

	"gitea.dev/cli/internal/services"

	"github.com/spf13/cobra"
)

// pullActionCommand builds a command that sets one state of a pull request, such as locked, to
// want, and reports it like bh pr close does. One in that state already is reported and left alone.
func (a *application) pullActionCommand(verb, short, long, example, past string, complete cobra.CompletionFunc, state func(*services.Pull) *bool, want bool, act func(ctx context.Context, service services.Issues, number int64) error) *cobra.Command {
	return &cobra.Command{
		Use: verb + " [<number> | <url> | <branch>]", Short: short, Long: long, Example: example,
		Annotations: pullArgumentsAnnotation(nil), Args: usageArgs(cobra.MaximumNArgs(1)), ValidArgsFunction: complete,
		RunE: func(cmd *cobra.Command, args []string) error {
			t, n, err := a.pullSelectorAny(cmd, args)
			if err != nil {
				return err
			}
			item, err := (services.Pulls{Client: t.client, Repository: t.Repo}).View(cmd.Context(), n)
			if err != nil {
				return err
			}
			if *state(&item) == want {
				a.warn(cmd, "Pull request %s is already %s", refLabel(t.Repo, n, item.Title), strings.ToLower(past))
				return a.pullResult(cmd, t, item)
			}
			if err := act(cmd.Context(), services.Issues{Client: t.client, Repository: t.Repo, Pulls: true}, n); err != nil {
				return err
			}
			*state(&item) = want
			a.done(cmd, "%s pull request %s", past, refLabel(t.Repo, n, item.Title))
			return a.pullResult(cmd, t, item)
		},
	}
}

func pullLocked(item *services.Pull) *bool { return &item.Locked }
func pullPinned(item *services.Pull) *bool { return &item.Pinned }

func (a *application) pullLockCommand() *cobra.Command {
	var reason string
	cmd := a.pullActionCommand("lock", "Lock the conversation of a pull request",
		`Lock the conversation of a pull request, so that only people with write access can comment on
it. One that is locked already is reported and keeps its reason.

BranchSpire does not check the reason: it offers Too heated, Off-topic, Resolved and Spam by default,
and administrators can change that.`,
		`$ bh pr lock 13
$ bh pr lock 13 --reason "Too heated"`, "Locked", firstArg(a.completePulls("open")), pullLocked, true,
		func(ctx context.Context, service services.Issues, n int64) error {
			return service.SetLocked(ctx, n, true, reason)
		})
	cmd.Flags().StringVarP(&reason, "reason", "r", "", "Reason for locking, shown on the pull request, such as \"Resolved\"")
	return cmd
}

func (a *application) pullUnlockCommand() *cobra.Command {
	return a.pullActionCommand("unlock", "Unlock the conversation of a pull request",
		"Unlock the conversation of a pull request, so that everyone who can see it can comment again. One that is not locked is reported.",
		"$ bh pr unlock 13", "Unlocked", firstArg(a.completePulls("all")), pullLocked, false,
		func(ctx context.Context, service services.Issues, n int64) error {
			return service.SetLocked(ctx, n, false, "")
		})
}

func (a *application) pullPinCommand() *cobra.Command {
	return a.pullActionCommand("pin", "Pin a pull request to the top of the list",
		"Pin a pull request to the top of the repository's pull request list. BranchSpire allows only a few pinned items at a time. One that is pinned already is reported.",
		"$ bh pr pin 13", "Pinned", firstArg(a.completePulls("open")), pullPinned, true,
		func(ctx context.Context, service services.Issues, n int64) error {
			return service.SetPinned(ctx, n, true)
		})
}

func (a *application) pullUnpinCommand() *cobra.Command {
	return a.pullActionCommand("unpin", "Unpin a pull request",
		"Unpin a pull request from the top of the repository's pull request list. One that is not pinned is reported.",
		"$ bh pr unpin 13", "Unpinned", firstArg(a.completePulls("all")), pullPinned, false,
		func(ctx context.Context, service services.Issues, n int64) error {
			return service.SetPinned(ctx, n, false)
		})
}

func (a *application) pullDeleteCommand() *cobra.Command {
	return &cobra.Command{
		Use: "delete [<number> | <url> | <branch>]", Aliases: []string{"rm"}, Short: "Delete a pull request",
		Long: `Delete a pull request for good, with its comments, reviews and attachments. This cannot be
undone and needs the permission to delete issues, which BranchSpire gives repository owners and
administrators. The head and base branches stay. To only stop work on it, use bh pr close.
A branch names only an open pull request: give the number of a closed or merged one.
Issues are not deleted by this command: see bh issue delete.`,
		Example: `$ bh pr delete 13
$ bh pr delete 13 --yes`,
		Annotations: pullArgumentsAnnotation(map[string]string{"bh.text": "pull request deletion does not return a resource", "bh.confirm": "true"}),
		Args:        usageArgs(cobra.MaximumNArgs(1)), ValidArgsFunction: firstArg(a.completePulls("all")),
		RunE: func(cmd *cobra.Command, args []string) error {
			t, n, err := a.pullSelector(cmd, args) // a branch never names a closed one, which may be long gone
			if err != nil {
				return err
			}
			item, err := (services.Pulls{Client: t.client, Repository: t.Repo}).View(cmd.Context(), n)
			if err != nil {
				return err
			}
			service := services.Issues{Client: t.client, Repository: t.Repo, Pulls: true}
			return a.deleteConfirmed(cmd, "pull request "+refLabel(t.Repo, n, item.Title), func() error { return service.Delete(cmd.Context(), n) })
		},
	}
}
