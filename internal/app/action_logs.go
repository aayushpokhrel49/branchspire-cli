// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package app

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"

	"gitea.dev/cli/internal/clierr"
	"gitea.dev/cli/internal/services"
	"gitea.dev/cli/internal/terminal"

	"github.com/spf13/cobra"
)

func (a *application) runLogsCommand() *cobra.Command {
	var archive downloadFlags
	var job string
	var follow, failed bool
	var logs logOptions
	var interval, timeout time.Duration
	cmd := &cobra.Command{
		Use:   "logs [<run-id>]",
		Short: "Print the logs of a run's jobs",
		Long: `Print the logs of a run's jobs, one after another; the name of each job goes to stderr.
With --tail, bh prints only the last lines of each job's log. Lines print without the timestamp
that BranchSpire starts them with; --timestamps keeps it.

--failed selects the jobs that failed or were canceled, those that bh run rerun --failed reruns,
and prints only the lines of their failed steps, which bh names on stderr. It prints the whole
log of a job that reports no failed step; --job prints the whole log of any job.

With --follow, bh waits for jobs to start and prints new log lines until they complete,
prefixing each line with its job's name when it follows several jobs. With --output or --dir,
it downloads the logs of the whole run as a ZIP archive instead. The URL of a job selects that job,
like --job.`,
		Example: `# print the logs of every job of a run
$ bh run logs 34
# follow the logs of one job while it runs
$ bh run logs 34 --job build --follow
# print the last 50 lines of the failed steps of each failed or canceled job
$ bh run logs 34 --failed --tail 50
# download the logs as a ZIP archive
$ bh run logs 34 --output logs.zip`,
		Args:        usageArgs(cobra.MaximumNArgs(1)),
		Annotations: map[string]string{"bh.arguments": runArguments, "bh.text": "run logs emits text"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := pollInterval(interval); err != nil {
				return err
			}
			if timeout <= 0 {
				return clierr.Wrap(clierr.Usage, errors.New("--timeout must be positive"))
			}
			for _, name := range []string{"interval", "timeout"} {
				if !follow && cmd.Flags().Changed(name) {
					return clierr.Wrap(clierr.Usage, fmt.Errorf("--%s requires --follow", name))
				}
			}
			download := archive.output != "" || archive.dir != ""
			conflicting := ""
			switch {
			case failed:
				conflicting = "--failed"
			case archive.output != "":
				conflicting = "--output"
			case archive.dir != "":
				conflicting = "--dir"
			}
			selected, urlJob, err := jobOfRunURL(args, job, conflicting)
			if err != nil {
				return err
			}
			job = selected
			if download && (follow || failed || job != "" || logs.timestamps || cmd.Flags().Changed("tail")) {
				return clierr.Wrap(clierr.Usage, errors.New("--output and --dir download the whole run and cannot be combined with --job, --failed, --follow, --tail or --timestamps"))
			}
			if cmd.Flags().Changed("tail") && logs.tail < 1 {
				return clierr.Wrap(clierr.Usage, errors.New("--tail must be positive"))
			}
			for _, flag := range []struct {
				name string
				set  bool
			}{{"--clobber", archive.clobber}, {"--skip-existing", archive.skipExisting}} {
				if flag.set && !download {
					return clierr.Wrap(clierr.Usage, errors.New(flag.name+" requires --output or --dir"))
				}
			}
			if err := archive.validate(); err != nil {
				return err
			}
			if follow && failed {
				return clierr.Wrap(clierr.Usage, errors.New("--follow cannot be combined with --failed"))
			}
			if follow && cmd.Flags().Changed("tail") {
				return clierr.Wrap(clierr.Usage, errors.New("--follow cannot be combined with --tail"))
			}
			if failed && job != "" {
				return clierr.Wrap(clierr.Usage, errors.New("--failed cannot be combined with --job"))
			}
			t, run, err := a.runArg(cmd, args, false)
			if err != nil {
				return err
			}
			service := services.Actions{Client: t.client, Repository: t.Repo}
			if download {
				path, skip, err := archive.destination(fmt.Sprintf("run-%d-logs.zip", run))
				switch {
				case err != nil:
					return err
				case skip:
					a.warn(cmd, "Skipped the logs of run %d: %s exists", run, path)
					return nil
				}
				body, err := service.RunLogsArchive(cmd.Context(), run)
				if err != nil {
					return err
				}
				defer body.Close()
				if err := a.download(cmd, archive, path, body); err != nil {
					return err
				}
				if path != "-" {
					a.done(cmd, "Downloaded the logs of run %d to %s", run, path)
				}
				return nil
			}
			jobs, err := service.Jobs(cmd.Context(), run)
			if err != nil {
				return err
			}
			if jobs, err = selectJobs(cmd.Context(), service, jobs, job, failed, run); err != nil {
				return err
			}
			if job != "" {
				if err := sameJob(job, jobs[0], urlJob); err != nil {
					return err
				}
			}
			if !follow {
				logs.failedSteps = failed
				return a.printJobLogs(cmd, service, run, jobs, logs)
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()
			return timeoutError(a.followJobLogs(ctx, cmd, service, run, jobs, interval, logs.timestamps), timeout, "following the logs of run %d", run)
		},
	}
	archive.register(cmd)
	cmd.Flags().StringVarP(&job, "job", "j", "", "Only the logs of this job, by `name or ID`")
	cmd.Flags().BoolVar(&failed, "failed", false, "Only the failed steps of failed and canceled jobs")
	cmd.Flags().IntVar(&logs.tail, "tail", 0, "Only the last `N` lines of each job's log")
	cmd.Flags().BoolVar(&logs.timestamps, "timestamps", false, "Keep the timestamp that starts each line")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Wait for jobs to start and print new lines until they complete")
	cmd.Flags().DurationVarP(&interval, "interval", "i", 5*time.Second, "Polling interval with --follow, at least 1s")
	cmd.Flags().DurationVar(&timeout, "timeout", time.Hour, "Maximum time to follow")
	cmd.ValidArgsFunction = firstArg(a.completeRuns())
	_ = cmd.RegisterFlagCompletionFunc("job", a.completeJobs())
	return cmd
}

// runJob returns the job of run that raw names among jobs, those of its latest attempt, else the
// job of an earlier attempt with that ID, which bh showed before a rerun.
func runJob(ctx context.Context, service services.Actions, jobs []services.Job, raw string, run int64) (services.Job, error) {
	job, err := findJob(jobs, raw, run)
	id, parseErr := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if _, missing := errors.AsType[*services.NotFoundError](err); !missing || parseErr != nil {
		return job, err
	}
	return service.RunJob(ctx, run, id)
}

// selectJobs narrows jobs of run to the one that raw names, when set, or else with failed to those
// that failed or were canceled.
func selectJobs(ctx context.Context, service services.Actions, jobs []services.Job, raw string, failed bool, run int64) ([]services.Job, error) {
	switch {
	case raw != "":
		selected, err := runJob(ctx, service, jobs, raw, run)
		if err != nil {
			return nil, err
		}
		return []services.Job{selected}, nil
	case failed:
		running := slices.ContainsFunc(jobs, func(job services.Job) bool { return !job.Completed() })
		if jobs = slices.DeleteFunc(jobs, func(job services.Job) bool { return !failedJob(job) }); len(jobs) == 0 {
			if running {
				return nil, fmt.Errorf("run %d has no failed or canceled jobs yet; it is still running", run)
			}
			return nil, fmt.Errorf("run %d has no failed or canceled jobs", run)
		}
	}
	return jobs, nil
}

// logOptions shapes the logs that bh prints of a job.
type logOptions struct {
	// tail, when positive, keeps only the last lines.
	tail int
	// timestamps keeps the timestamp that BranchSpire starts each line with.
	timestamps bool
	// failedSteps keeps only the lines of the job's failed steps, when it reports any.
	failedSteps bool
}

// printJobLogs prints the logs of jobs on stdout as opts shapes them, each headed by its name on
// stderr. Jobs that have not started are reported on stderr and skipped; it fails when none of
// them has.
func (a *application) printJobLogs(cmd *cobra.Command, service services.Actions, run int64, jobs []services.Job, opts logOptions) (err error) {
	out := a.logWriter(cmd)
	defer func() { err = cmp.Or(err, terminal.ResetSGR(out)) }()
	printed := false
	for _, job := range jobs {
		label := fmt.Sprintf("%s (job %d)", terminal.SafeLine(job.Name), job.ID)
		var logs io.ReadCloser
		var err error
		if job.Started() {
			logs, err = service.JobLogs(cmd.Context(), job.ID)
		}
		if !job.Started() || errors.Is(err, services.ErrJobNotStarted) {
			if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "%s: %s\n", label, unstarted(job)); err != nil {
				return err
			}
			continue
		}
		if errors.Is(err, services.ErrJobLogsUnavailable) {
			if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "%s: %s\n", label, services.ErrJobLogsUnavailable); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		var steps []stepWindow
		if opts.failedSteps {
			var names []string
			if steps, names = failedSteps(job); len(names) > 0 {
				label += ", failed steps: " + terminal.SafeLine(strings.Join(names, ", "))
			}
		}
		if _, err := fmt.Fprintln(cmd.ErrOrStderr(), label); err != nil {
			_ = logs.Close()
			return err
		}
		copyErr := copyLog(out, logs, opts, steps)
		closeErr := logs.Close()
		// The colors of one job's log end with it.
		if err := cmp.Or(copyErr, closeErr, terminal.ResetSGR(out)); err != nil {
			return err
		}
		printed = true
	}
	if !printed {
		return noLogsError(run)
	}
	return nil
}

var errNoStartedJobs = errors.New("none of the selected jobs has started")

func noLogsError(run int64) error {
	return fmt.Errorf("run %d has no logs: %w", run, errNoStartedJobs)
}

// stepWindow is the time during which a step ran, at the second precision of BranchSpire's step times.
type stepWindow struct{ start, end time.Time }

// failedSteps returns when the failed and canceled steps of job ran, and their names.
func failedSteps(job services.Job) (windows []stepWindow, names []string) {
	for _, step := range job.Steps {
		if step.Conclusion != "failure" && step.Conclusion != "cancelled" || step.StartedAt == nil { //nolint:misspell // BranchSpire API spelling.
			continue
		}
		window := stepWindow{start: terminal.ParseTime(*step.StartedAt)}
		if window.start.IsZero() {
			continue
		}
		if step.CompletedAt != nil {
			window.end = terminal.ParseTime(*step.CompletedAt).Add(time.Second)
		}
		windows, names = append(windows, window), append(names, step.Name)
	}
	return windows, names
}

// logTimestamp splits the RFC 3339 timestamp that BranchSpire starts every log line with from the rest
// of line, and returns the zero time and the whole line when it has none.
func logTimestamp(line []byte) (time.Time, []byte) {
	end := bytes.IndexAny(line, " \r\n")
	if end < 0 {
		end = len(line)
	}
	stamp, err := time.Parse(time.RFC3339Nano, string(line[:end]))
	if err != nil {
		return time.Time{}, line
	}
	return stamp, bytes.TrimPrefix(line[end:], []byte(" "))
}

// linesWithin returns the lines of log that are stamped within one of windows; a line without a
// timestamp, such as the rest of a multi-line message, goes with the line before it.
func linesWithin(log []byte, windows []stepWindow) []byte {
	var kept []byte
	within := false
	for line := range bytes.Lines(log) {
		if stamp, _ := logTimestamp(line); !stamp.IsZero() {
			within = slices.ContainsFunc(windows, func(w stepWindow) bool {
				return !stamp.Before(w.start) && (w.end.IsZero() || stamp.Before(w.end))
			})
		}
		if within {
			kept = append(kept, line...)
		}
	}
	return kept
}

// copyLog writes the lines that r reads to w as opts shapes them, holding no more than the tail in
// memory. With steps it reads the log whole and writes only their lines, or every line when the
// log has none of theirs, so that a failure is never hidden.
func copyLog(w io.Writer, r io.Reader, opts logOptions, steps []stepWindow) error {
	if len(steps) > 0 {
		log, err := io.ReadAll(r)
		if err != nil {
			return err
		}
		if kept := linesWithin(log, steps); len(kept) > 0 {
			log = kept
		}
		r = bytes.NewReader(log)
	}
	if opts.timestamps && opts.tail <= 0 {
		_, err := io.Copy(w, r)
		return err
	}
	var tail [][]byte
	reader := bufio.NewReader(r)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			if !opts.timestamps {
				_, line = logTimestamp(line)
			}
			if opts.tail > 0 {
				if len(tail) == opts.tail {
					tail = tail[1:]
				}
				tail = append(tail, line)
			} else if _, err := w.Write(line); err != nil {
				return err
			}
		}
		if errors.Is(err, io.EOF) {
			_, err = w.Write(bytes.Join(tail, nil))
			return err
		}
		if err != nil {
			return err
		}
	}
}

// logWriter returns stdout for job logs: on a terminal it drops their escape sequences but keeps
// their colors when output is colored, which ResetSGR then ends, and drops them whole otherwise.
func (a *application) logWriter(cmd *cobra.Command) io.Writer {
	return terminal.SafeColorWriter(cmd.OutOrStdout(), a.deps.Streams.OutTTY, a.colorEnabled())
}

// followJobLogs follows the logs of jobs one after another until each completes, waiting for
// those that have not started. Following several jobs prefixes each line with its job's name, and
// lines lose their timestamp unless timestamps keeps it.
func (a *application) followJobLogs(ctx context.Context, cmd *cobra.Command, service services.Actions, run int64, jobs []services.Job, interval time.Duration, timestamps bool) (err error) {
	out := a.logWriter(cmd)
	defer func() { err = cmp.Or(err, terminal.ResetSGR(out)) }()
	printed := false
	for _, job := range jobs {
		label := fmt.Sprintf("%s (job %d)", terminal.SafeLine(job.Name), job.ID)
		if !job.Started() && job.Completed() {
			fmt.Fprintf(cmd.ErrOrStderr(), "%s: %s\n", label, unstarted(job))
			continue
		}
		if job.Started() {
			fmt.Fprintln(cmd.ErrOrStderr(), label)
		} else {
			fmt.Fprintf(cmd.ErrOrStderr(), "%s: waiting for it to start…\n", label)
		}
		lines := &prefixWriter{w: out, timestamps: timestamps}
		if len(jobs) > 1 {
			lines.prefix = []byte(terminal.SafeLine(job.Name) + "\t")
		}
		err := service.FollowJobLogs(ctx, run, job.ID, interval, func(chunk []byte) error {
			_, err := lines.Write(chunk)
			return err
		})
		if errors.Is(err, services.ErrJobNotStarted) {
			fmt.Fprintf(cmd.ErrOrStderr(), "%s: never started\n", label)
			continue
		}
		if errors.Is(err, services.ErrJobLogsUnavailable) {
			fmt.Fprintf(cmd.ErrOrStderr(), "%s: %s\n", label, services.ErrJobLogsUnavailable)
			continue
		}
		if err := cmp.Or(err, lines.Flush(), terminal.ResetSGR(out)); err != nil {
			return err
		}
		printed = true
	}
	if !printed {
		return noLogsError(run)
	}
	return nil
}

// prefixWriter writes prefix before every line it passes to w, without the line's timestamp unless
// timestamps keeps it; Flush ends a last unterminated line.
type prefixWriter struct {
	w          io.Writer
	prefix     []byte
	timestamps bool
	partial    bool
	// held is the start of a line whose timestamp may not have arrived in full.
	held []byte
}

func (p *prefixWriter) Write(data []byte) (int, error) {
	if len(p.prefix) == 0 && p.timestamps {
		return p.w.Write(data)
	}
	var b bytes.Buffer
	for line := range bytes.Lines(data) {
		whole := bytes.HasSuffix(line, []byte("\n"))
		if !p.timestamps {
			if p.held = append(p.held, line...); !whole {
				continue
			}
			_, line = logTimestamp(p.held)
		}
		if !p.partial {
			b.Write(p.prefix)
		}
		b.Write(line)
		p.partial, p.held = !whole, p.held[:0]
	}
	if _, err := p.w.Write(b.Bytes()); err != nil {
		return 0, err
	}
	return len(data), nil
}

// Flush writes a held line and terminates a last line that has no line break.
func (p *prefixWriter) Flush() error {
	var b bytes.Buffer
	if len(p.held) > 0 {
		_, line := logTimestamp(p.held)
		b.Write(p.prefix)
		b.Write(line)
		p.partial, p.held = true, nil
	}
	if !p.partial {
		return nil
	}
	p.partial = false
	b.WriteByte('\n')
	_, err := p.w.Write(b.Bytes())
	return err
}

// unstarted explains why a job has no logs: it is waiting for a runner, or it completed, such
// as by cancellation, before one started it.
func unstarted(job services.Job) string {
	if job.CompletedAt == nil {
		return "not started yet"
	}
	return "never started (" + cmp.Or(job.Conclusion, job.Status) + ")"
}
