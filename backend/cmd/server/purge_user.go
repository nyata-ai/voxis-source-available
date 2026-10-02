package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/voxis/backend/internal/adapter/localfs"
	"github.com/voxis/backend/internal/adapter/postgres"
	"github.com/voxis/backend/internal/service"
)

const (
	purgeUserCommand = "purge-user"
	purgeUserUsage   = "usage: voxis-api purge-user --subject <keycloak-subject> [--confirm]"
	purgeUserTimeout = 30 * time.Minute
)

// purgeUserOptions are the parsed purge-user arguments.
type purgeUserOptions struct {
	subject string
	confirm bool
}

// runPurgeUser deletes every piece of content one person stored, for an
// operator honoring a deletion request. Without --confirm it only reports
// what would be deleted. It needs DATABASE_URL and LOCAL_STORAGE_DIR, which
// the API container already has, and returns the process exit code.
func runPurgeUser(args []string, stdout, stderr io.Writer) int {
	opts, err := parsePurgeUserArgs(args, stderr)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, purgeUserUsage) //nolint:errcheck // best-effort usage output
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), purgeUserTimeout)
	defer cancel()
	logger := slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	purger, closeDeps, err := newDataPurgeService(ctx, logger)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "purge-user: %v\n", err) //nolint:errcheck // best-effort error output
		return 1
	}
	defer closeDeps()
	if err := executePurgeUser(ctx, purger, opts, stdout); err != nil {
		_, _ = fmt.Fprintf(stderr, "purge-user: %v\n", err) //nolint:errcheck // best-effort error output
		return 1
	}
	return 0
}

func parsePurgeUserArgs(args []string, stderr io.Writer) (purgeUserOptions, error) {
	flags := flag.NewFlagSet(purgeUserCommand, flag.ContinueOnError)
	flags.SetOutput(stderr)
	subject := flags.String("subject", "", "Keycloak subject (the sub claim) of the person whose data to delete")
	confirm := flags.Bool("confirm", false, "delete the data; without it the command only reports what it would delete")
	if err := flags.Parse(args); err != nil {
		return purgeUserOptions{}, err
	}
	opts := purgeUserOptions{subject: strings.TrimSpace(*subject), confirm: *confirm}
	if opts.subject == "" || flags.NArg() != 0 {
		return purgeUserOptions{}, errors.New("a --subject and no other arguments are required")
	}
	return opts, nil
}

// dataPurger is the part of service.DataPurgeService the command uses.
type dataPurger interface {
	Plan(ctx context.Context, userID string) (service.PurgePlan, error)
	Purge(ctx context.Context, userID string) (service.PurgeReport, error)
}

func executePurgeUser(ctx context.Context, purger dataPurger, opts purgeUserOptions, stdout io.Writer) error {
	if !opts.confirm {
		plan, err := purger.Plan(ctx, opts.subject)
		if err != nil {
			return err
		}
		writePurgePlan(stdout, plan)
		_, err = fmt.Fprintln(stdout, "Dry run: nothing was deleted. Re-run with --confirm to delete.")
		return err
	}
	report, err := purger.Purge(ctx, opts.subject)
	if err != nil {
		return err
	}
	writePurgePlan(stdout, report.PurgePlan)
	return writePurgeReport(stdout, report)
}

func writePurgePlan(w io.Writer, plan service.PurgePlan) {
	c := plan.Counts
	//nolint:errcheck // terminal output; a write failure has no recovery
	fmt.Fprintf(w, "User %s, personal organization %s\n"+
		"  media: %d\n  transcriptions: %d\n  summaries: %d\n  recording sessions: %d\n"+
		"  recording chunks: %d\n  collections: %d\n  API keys: %d\n  provider jobs awaiting deletion: %d\n",
		plan.UserID, plan.OrganizationID, c.Media, c.Transcriptions, c.Summaries, c.RecordingSessions,
		c.RecordingChunks, c.Collections, c.APIKeys, c.PendingProviderDeletions)
}

func writePurgeReport(w io.Writer, report service.PurgeReport) error {
	if _, err := fmt.Fprintf(w, "Deleted: %d media purged with their files, transcripts and summaries; "+
		"recordings, collections and API keys removed.\n", report.MediaPurged); err != nil {
		return err
	}
	if !report.OrganizationDeleted {
		_, err := fmt.Fprintf(w, "The transcription provider has not yet confirmed deletion of %d job(s). "+
			"The organization's names and email addresses were scrubbed; its content-free records stay "+
			"until you re-run this command after the provider cleanup (every 5 minutes) has finished.\n",
			report.PendingProviderDeletions)
		return err
	}
	// Every backup also holds an OpenBao snapshot with this organization's
	// transit key, so deleting the live key would not make backups unreadable.
	_, err := fmt.Fprintln(w, "Organization and user records deleted. Backups made before this purge "+
		"still hold this person's data, with the key to read it, until they rotate out.")
	return err
}

// newDataPurgeService connects to PostgreSQL and local storage only. The
// purge never decrypts, so the transit key service is not needed.
func newDataPurgeService(ctx context.Context, logger *slog.Logger) (*service.DataPurgeService, func(), error) {
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	localDir := strings.TrimSpace(os.Getenv("LOCAL_STORAGE_DIR"))
	if databaseURL == "" || localDir == "" {
		return nil, nil, errors.New("DATABASE_URL and LOCAL_STORAGE_DIR are required")
	}
	pool, err := postgres.NewPool(ctx, postgres.DefaultPoolConfig(databaseURL))
	if err != nil {
		return nil, nil, fmt.Errorf("connect PostgreSQL: %w", err)
	}
	storage, err := localfs.New(localDir, logger)
	if err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("open local storage: %w", err)
	}
	media := service.NewMediaService(postgres.NewMediaRepository(pool.Pool), postgres.NewTranscriptionRepository(pool.Pool),
		postgres.NewSummaryRepository(pool.Pool), storage, nil, nil, logger)
	purger := service.NewDataPurgeService(postgres.NewDataPurgeRepository(pool.Pool), media, storage, logger)
	return purger, func() {
		closeWithWarning(logger, "local storage", storage)
		pool.Close()
	}, nil
}
