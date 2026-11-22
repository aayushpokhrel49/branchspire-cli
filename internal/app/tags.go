// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package app

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"gitea.dev/cli/internal/clierr"
	"gitea.dev/cli/internal/services"
	"gitea.dev/cli/internal/terminal"
	"gitea.dev/cli/internal/transport"

	"github.com/spf13/cobra"
)

// validateTagName rejects names that Git does not take for a tag before any request.
func validateTagName(name string) error {
	if validateBranch(name) != nil {
		return clierr.Wrap(clierr.Usage, fmt.Errorf("invalid tag name %q", name))
	}
	return nil
}

func (a *application) tagCommand() *cobra.Command {
	group := &cobra.Command{Use: "tag", Short: "Manage tags and tag protection rules", Long: `List, view, create and delete the tags of a repository on the server, and protect tags by pattern so
that only some users and teams can create matching ones. A release is made from a tag: see bh release
create. These commands need no clone.`}
	group.AddCommand(a.tagListCommand(), a.tagViewCommand(), a.tagCreateCommand(), a.tagDeleteCommand(), a.tagProtectCommand(), a.tagUnprotectCommand(), a.tagProtectionCommand())
	return jsonOutput[services.Tag](group)
}

// tagURL links a tag's files on the web.
func tagURL(repo target, name string) string {
	return repositoryURL(repo) + "/src/tag/" + compareRef(name)
}

// messageSubject is the first line of a commit or tag message.
func messageSubject(message string) string {
	subject, _, _ := strings.Cut(message, "\n")
	return subject
}

// tagObjectFields are the JSON fields of a tag that are read from its annotated tag object.
var tagObjectFields = []string{"tagger", "tagger_email", "tagged_at", "verified", "verification_reason"}

// tagKind words how a tag was made.
func tagKind(tag services.Tag) string {
	if tag.Annotated {
		return "annotated"
	}
	return "lightweight"
}

func (a *application) tagListCommand() *cobra.Command {
	var limit, page int
	cmd := &cobra.Command{
		Use: "list", Aliases: []string{"ls"}, Short: "List tags", Args: argsExact(0),
		Example: `$ bh tag list
$ bh tag list --limit 100 --json name,commit,annotated`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			target, err := a.resolve(cmd.Context(), true)
			if err != nil {
				return err
			}
			service := services.Tags{Client: target.client}
			tags, err := service.List(cmd.Context(), target.Repo, limit, page)
			if err != nil {
				return err
			}
			if a.jsonSelects(tagObjectFields...) {
				if tags.Items, err = service.WithTagObjects(cmd.Context(), target.Repo, tags.Items); err != nil {
					return err
				}
			}
			tty := a.deps.Streams.OutTTY
			return writeTable(a, cmd, tags.Items, listSpec[services.Tag]{
				Columns: []terminal.Column{{Header: "Tag"}, {Header: "Commit"}, {Header: "Type"}, {Header: "Message", Flex: true}},
				Row: func(tag services.Tag) []terminal.Cell {
					sha := pipedSHA(tag.Commit)
					if tty {
						sha = shortSHA(sha)
					}
					return []terminal.Cell{{Text: terminal.SafeLine(tag.Name), Bold: true}, {Text: sha, Muted: true}, {Text: tagKind(tag), Muted: true}, {Text: terminal.SafeLine(messageSubject(tag.Message))}}
				},
				Context: "tags in " + target.Repo, Empty: "no tags in " + target.Repo,
			}, servicePage(tags, page))
		},
	}
	pageFlags(cmd, &limit, &page)
	return cmd
}

func (a *application) tagViewCommand() *cobra.Command {
	var web bool
	cmd := &cobra.Command{
		Use: "view <tag>", Short: "View a tag", Args: nonBlankArgs(cobra.ExactArgs(1), "tag"), ValidArgsFunction: a.completeTags(),
		Long: `View a tag: the commit it points to and, for an annotated tag, who made it and its message. The
signature check describes the tagged commit, not the tag, as BranchSpire checks only the commit; in --json it
is verified and verification_reason. It and tagger, tagger_email and tagged_at are read for annotated
tags only.`,
		Example: `$ bh tag view v1.2.0
$ bh tag view v1.2.0 --json commit,annotated --jq .commit`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateTagName(args[0]); err != nil {
				return err
			}
			target, err := a.resolve(cmd.Context(), true)
			if err != nil {
				return err
			}
			service := services.Tags{Client: target.client}
			tag, err := service.View(cmd.Context(), target.Repo, args[0])
			if err != nil {
				return err
			}
			if web {
				return a.openBrowser(cmd, tagURL(target, tag.Name))
			}
			if a.options.json == "" || a.jsonSelects(tagObjectFields...) {
				if tag, err = service.WithTagObject(cmd.Context(), target.Repo, tag); err != nil {
					return err
				}
			}
			d := a.display()
			return writeView(a, cmd, tag, func(tag services.Tag) terminal.View {
				badges := []terminal.Cell{{Text: tagKind(tag), Muted: true}}
				if tag.Verified {
					badges = append(badges, terminal.Cell{Text: "commit verified", Color: "green"})
				}
				fields := []terminal.Field{{Label: "Commit", Value: tag.Commit}}
				if tag.Annotated {
					fields = append(fields, terminal.Field{Label: "Tag object", Value: tag.ID}, terminal.Field{Label: "Tagger", Value: strings.TrimSpace(deref(tag.Tagger) + " <" + deref(tag.TaggerEmail) + ">")}, timeField("Tagged", deref(tag.TaggedAt), d))
					if tag.VerificationReason != "" && !tag.Verified && tag.VerificationReason != "gpg.error.not_signed_commit" {
						fields = append(fields, terminal.Field{Label: "Commit signature", Value: tag.VerificationReason})
					}
				}
				fields = append(fields, terminal.Field{Label: "Archives", Value: tag.TarballURL + " " + tag.ZipballURL})
				return terminal.View{Title: tag.Name, Badges: badges, Fields: fields, Body: tag.Message, Plain: true, Bodyless: tag.Message == "", Kind: "tag", URL: tagURL(target, tag.Name)}
			})
		},
	}
	cmd.Flags().BoolVarP(&web, "web", "w", false, "Open the tag in the browser")
	return cmd
}

func (a *application) tagCreateCommand() *cobra.Command {
	var target, message string
	cmd := &cobra.Command{
		Use: "create <tag>", Short: "Create a tag", Args: nonBlankArgs(cobra.ExactArgs(1), "tag"),
		Long: `Create a tag on the server. It points to the head of the default branch, or to --target, a branch or
a commit. A message with --message makes it an annotated tag, which records who made it and why;
without one it is a lightweight tag.

To announce the tag with notes and files, create a release with bh release create instead, which makes
the tag when it does not exist.`,
		Example: `$ bh tag create v1.2.0
$ bh tag create v1.2.0 --target release/1.2 --message "Version 1.2.0"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateTagName(args[0]); err != nil {
				return err
			}
			if cmd.Flags().Changed("target") && strings.TrimSpace(target) == "" {
				return clierr.Wrap(clierr.Usage, errors.New("--target cannot be empty"))
			}
			repo, err := a.resolve(cmd.Context(), true)
			if err != nil {
				return err
			}
			ref := target
			if target != "" {
				if ref, err = a.serverRef(cmd.Context(), repo, a.isCloneOf(cmd.Context(), repo), target); err != nil {
					return err
				}
			}
			service := services.Tags{Client: repo.client}
			tag, err := service.Create(cmd.Context(), repo.Repo, args[0], ref, message)
			if err = a.explainTarget(cmd.Context(), repo, err, target); err != nil {
				return err
			}
			a.done(cmd, "Created %s tag %s at %s", tagKind(tag), terminal.SafeLine(tag.Name), shortSHA(tag.Commit))
			if a.jsonSelects(tagObjectFields...) {
				if tag, err = service.WithTagObject(cmd.Context(), repo.Repo, tag); err != nil {
					return err
				}
			}
			return a.jsonResult(cmd, tag)
		},
	}
	cmd.Flags().StringVar(&target, "target", "", "Branch or commit to tag (default: the default branch)")
	cmd.Flags().StringVarP(&message, "message", "m", "", "Message of an annotated tag")
	return cmd
}

func (a *application) tagDeleteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use: "delete <tag>...", Aliases: []string{"rm"}, Short: "Delete tags", Args: usageArgs(cobra.MinimumNArgs(1)), ValidArgsFunction: a.completeTags(),
		Long: `Delete tags on the server. To delete a release together with its tag, use bh release delete --delete-tag.`,
		Example: `$ bh tag delete v1.2.0-rc1
$ bh tag delete v1.2.0-rc1 v1.2.0-rc2 --yes`,
		Annotations: map[string]string{"bh.confirm": "true", "bh.text": "tag deletion does not return a resource"},
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, name := range args {
				if err := validateTagName(name); err != nil {
					return err
				}
			}
			repo, err := a.resolve(cmd.Context(), true)
			if err != nil {
				return err
			}
			subject := "tag " + args[0]
			if len(args) > 1 {
				subject = fmt.Sprintf("%d tags (%s)", len(args), strings.Join(args, ", "))
			}
			if err := a.confirmAction(cmd, terminal.Confirmation{Prompt: "Delete " + subject + " from " + repo.Repo + "?"}); err != nil {
				return err
			}
			service := services.Tags{Client: repo.client}
			return batchDone(a, cmd, args, func(name string) string { return "delete tag " + name }, func(name string) (string, error) {
				err := service.Delete(cmd.Context(), repo.Repo, name)
				// BranchSpire refuses a tag that a release uses, with a 409 that names no way out.
				if apiErr, ok := errors.AsType[*transport.APIError](err); ok && apiErr.Status == http.StatusConflict {
					err = fmt.Errorf("%w; delete the release too with bh release delete %s --delete-tag", err, name)
				}
				return name, err
			}, func(name string) string { return "Deleted tag " + name + " from " + repo.Repo })
		},
	}
	return cmd
}

func (a *application) tagProtectCommand() *cobra.Command {
	var users, teams []string
	cmd := &cobra.Command{
		Use: "protect <pattern>...", Short: "Protect the tags matching a pattern", Args: nonBlankArgs(cobra.MinimumNArgs(1), "pattern"),
		Long: `Add a rule for each pattern, a tag name or a glob such as v*, so that only the users of --allow-user and
the teams of --allow-team may create the matching tags. The teams are those of the organization that owns
the repository, so --allow-team requires an organization. Creating a tag also takes write access, and BranchSpire
lists only the users and teams that can read the repository: bh warns about those it leaves out.

When a pattern has a rule already, the lists that the flags name are replaced and the other one stays. An
empty --allow-user= removes every user from the rule. A rule that lists nobody stops everyone from creating
the matching tags.`,
		Example: `# Only the release team may create version tags
$ bh tag protect 'v*' --allow-team release

# Replace the users of the rule with alice and bob, keeping its teams
$ bh tag protect 'v*' --allow-user alice,bob

# Remove every user from the rule, keeping its teams
$ bh tag protect 'v*' --allow-user=`,
		Annotations: textOutput("tag protect does not return a resource"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Changed("allow-user") && !cmd.Flags().Changed("allow-team") {
				return clierr.Wrap(clierr.Usage, errors.New("tag protect requires --allow-user or --allow-team"))
			}
			if err := requireNames("user", "", users); err != nil {
				return err
			}
			if err := requireNames("team", "", teams); err != nil {
				return err
			}
			repo, err := a.resolve(cmd.Context(), true)
			if err != nil {
				return err
			}
			service := services.TagProtections{Client: repo.client}
			if len(teams) > 0 {
				isOrg, err := service.OwnerIsOrganization(cmd.Context(), repo.owner())
				if err != nil {
					return err
				}
				if !isOrg { // BranchSpire would drop the teams and answer with a rule that lets nobody in
					return clierr.Wrap(clierr.Usage, fmt.Errorf("--allow-team requires a repository owned by an organization, and %s is not", repo.Repo))
				}
			}
			ids, err := service.RuleIDs(cmd.Context(), repo.Repo)
			if err != nil {
				return err
			}
			updated := map[string]bool{}
			return batchDone(a, cmd, args, func(pattern string) string { return "protect tags " + pattern }, func(pattern string) (string, error) {
				changes := services.TagProtectionChanges{Users: users, Teams: teams}
				rule, created, err := service.Protect(cmd.Context(), repo.Repo, pattern, changes, ids)
				if errors.Is(err, services.ErrTagProtectionNeedsAllowed) {
					return pattern, clierr.Wrap(clierr.Usage, fmt.Errorf("%s has no rule yet: %w; name them with --allow-user or --allow-team", pattern, err))
				}
				if err != nil {
					return pattern, err
				}
				updated[pattern] = !created
				if a.options.dryRun { // a dry run gets no rule back
					return pattern, nil
				}
				unlistedUsers, unlistedTeams := changes.Unlisted(rule)
				for _, team := range unlistedTeams {
					unlistedUsers = append(unlistedUsers, "team "+repo.owner()+"/"+team)
				}
				if len(unlistedUsers) > 0 {
					a.warn(cmd, "BranchSpire leaves %s out of the rule for tags %s, as only those who can read %s are listed; creating tags takes write access", strings.Join(unlistedUsers, ", "), pattern, repo.Repo)
				}
				if len(rule.Users)+len(rule.Teams) == 0 {
					a.warn(cmd, "The rule for tags %s in %s lists no user or team, so nobody can create matching tags", pattern, repo.Repo)
				}
				return pattern, nil
			}, func(pattern string) string {
				if updated[pattern] {
					return "Updated the rule for tags " + pattern + " in " + repo.Repo
				}
				return "Protected tags " + pattern + " in " + repo.Repo
			})
		},
	}
	cmd.Flags().StringSliceVar(&users, "allow-user", nil, "Users that may create the matching tags (repeatable)")
	cmd.Flags().StringSliceVar(&teams, "allow-team", nil, "Teams of the repository's organization that may create the matching tags (repeatable)")
	return cmd
}

func (a *application) tagUnprotectCommand() *cobra.Command {
	return &cobra.Command{
		Use: "unprotect <pattern>...", Short: "Remove the protection of tags", Args: nonBlankArgs(cobra.MinimumNArgs(1), "pattern"),
		Long:        "Remove the rule for each pattern, as bh tag protection list lists them. Everyone who can write to the repository may then create matching tags.",
		Example:     "$ bh tag unprotect 'v*'",
		Annotations: map[string]string{"bh.confirm": "true", "bh.text": "removing protection does not return a resource"},
		RunE: func(cmd *cobra.Command, args []string) error {
			repo, err := a.resolve(cmd.Context(), true)
			if err != nil {
				return err
			}
			subject := "the protection of tags " + args[0]
			if len(args) > 1 {
				subject = fmt.Sprintf("the protection of %d tag patterns (%s)", len(args), strings.Join(args, ", "))
			}
			if err := a.confirmAction(cmd, terminal.Confirmation{Prompt: "Remove " + subject + " in " + repo.Repo + "?"}); err != nil {
				return err
			}
			service := services.TagProtections{Client: repo.client}
			ids, err := service.RuleIDs(cmd.Context(), repo.Repo)
			if err != nil {
				return err
			}
			return batchDone(a, cmd, args, func(pattern string) string { return "unprotect tags " + pattern }, func(pattern string) (string, error) {
				return pattern, service.Unprotect(cmd.Context(), repo.Repo, pattern, ids)
			}, func(pattern string) string { return "Removed the protection of tags " + pattern + " in " + repo.Repo })
		},
	}
}

func (a *application) tagProtectionCommand() *cobra.Command {
	group := &cobra.Command{Use: "protection", Short: "Read tag protection rules", Long: "Read the rules that bh tag protect and bh tag unprotect change."}
	group.AddCommand(a.tagProtectionListCommand())
	return group
}

func (a *application) tagProtectionListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use: "list", Aliases: []string{"ls"}, Short: "List the tag protection rules", Args: argsExact(0),
		Example: `$ bh tag protection list
$ bh tag protection list --json name_pattern,users,teams`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			repo, err := a.resolve(cmd.Context(), true)
			if err != nil {
				return err
			}
			rules, err := (services.TagProtections{Client: repo.client}).List(cmd.Context(), repo.Repo)
			if err != nil {
				return err
			}
			return writeTable(a, cmd, rules, listSpec[services.TagProtection]{
				Columns: []terminal.Column{{Header: "Pattern"}, {Header: "Users", Flex: true}, {Header: "Teams"}},
				Row: func(rule services.TagProtection) []terminal.Cell {
					return []terminal.Cell{{Text: terminal.SafeLine(rule.NamePattern), Bold: true}, {Text: terminal.SafeLine(strings.Join(rule.Users, ", "))}, {Text: terminal.SafeLine(strings.Join(rule.Teams, ", "))}}
				},
				Context: "tag protection rules of " + repo.Repo, Empty: "no tag protection rules in " + repo.Repo,
			}, listPage{Number: 1})
		},
	}
	return jsonOutput[services.TagProtection](cmd)
}
