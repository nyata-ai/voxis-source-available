package worker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/voxis/backend/internal/crypto"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

// ScanMediaJobArgs holds the arguments for a malware scan job.
type ScanMediaJobArgs struct {
	MediaID string `json:"media_id"`
	OrgID   string `json:"org_id"`
}

// Kind returns the unique job kind identifier.
func (ScanMediaJobArgs) Kind() string { return "scan_media" }

// InsertOpts provides default insertion options for scan jobs.
func (ScanMediaJobArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       "scanning",
		MaxAttempts: 10,
		UniqueOpts: river.UniqueOpts{
			ByArgs: true,
			ByState: []rivertype.JobState{
				rivertype.JobStatePending,
				rivertype.JobStateAvailable,
				rivertype.JobStateRunning,
				rivertype.JobStateScheduled,
			},
		},
	}
}

// ScanMediaWorker scans uploaded media files for malware using ClamAV.
type ScanMediaWorker struct {
	river.WorkerDefaults[ScanMediaJobArgs]
	mediaRepo port.MediaRepository
	storage   port.StorageClient
	envelope  *crypto.EnvelopeService
	scanner   port.MalwareScanner
	logger    *slog.Logger
}

// NewScanMediaWorker creates a new ScanMediaWorker with the given dependencies.
func NewScanMediaWorker(
	mediaRepo port.MediaRepository,
	storage port.StorageClient,
	envelope *crypto.EnvelopeService,
	scanner port.MalwareScanner,
	logger *slog.Logger,
) *ScanMediaWorker {
	if logger == nil {
		logger = slog.Default()
	}
	return &ScanMediaWorker{
		mediaRepo: mediaRepo,
		storage:   storage,
		envelope:  envelope,
		scanner:   scanner,
		logger:    logger,
	}
}

// scanTimeout is the maximum duration for a single scan operation.
const scanTimeout = 15 * time.Minute

// Timeout returns the maximum duration a scan job may run.
func (w *ScanMediaWorker) Timeout(*river.Job[ScanMediaJobArgs]) time.Duration {
	return scanTimeout + time.Minute // extra buffer for download/decrypt
}

// Work executes the malware scan pipeline:
//  1. Load media from DB
//  2. Validate org isolation
//  3. Skip deleted/failed media
//  4. If no scanner configured, mark scan_skipped
//  5. Check scanner availability (retry if unavailable)
//  6. Download encrypted file, decrypt, and stream to scanner
//  7. Update scan status based on result
func (w *ScanMediaWorker) Work(ctx context.Context, job *river.Job[ScanMediaJobArgs]) error {
	args := job.Args
	logger := w.logger.With("media_id", args.MediaID, "org_id", args.OrgID)

	// 1–3. Load media record and validate it is scannable.
	media, err := w.loadScanTarget(ctx, args, logger)
	if err != nil {
		return err
	}

	// 4. If scanner is nil (not configured), mark as skipped.
	if w.scanner == nil {
		if updateErr := w.mediaRepo.UpdateScanStatus(ctx, args.MediaID, domain.ScanStatusSkipped); updateErr != nil {
			logger.Warn("failed to set scan_skipped", "error", updateErr)
		}
		logger.Info("no scanner configured, marking scan_skipped")
		return nil
	}

	// 5. Check scanner availability — transient retry if unavailable.
	if availErr := w.checkScannerAvailable(ctx, job, logger); availErr != nil {
		return availErr
	}

	// 6–7. Download, decrypt, and stream to the scanner.
	result, err := w.decryptAndScan(ctx, args, media, logger)
	if err != nil {
		return err
	}

	// 8. Update status based on result.
	return w.recordScanResult(ctx, args, media, result, logger)
}

// loadScanTarget loads the media row and enforces org isolation plus a
// scannable status. All failures are JobCancel except transient load errors.
func (w *ScanMediaWorker) loadScanTarget(ctx context.Context, args ScanMediaJobArgs, logger *slog.Logger) (*domain.Media, error) {
	media, err := w.mediaRepo.GetByID(ctx, args.MediaID)
	if err != nil {
		if isNotFound(err) {
			logger.Warn("media not found, canceling scan job")
			return nil, river.JobCancel(fmt.Errorf("media %s not found", args.MediaID))
		}
		return nil, fmt.Errorf("load media: %w", err)
	}

	// Validate org isolation.
	if media.OrganizationID != args.OrgID {
		logger.Error("org mismatch between job args and media",
			"expected_org", args.OrgID,
			"actual_org", media.OrganizationID,
		)
		return nil, river.JobCancel(fmt.Errorf("org mismatch for media %s", args.MediaID))
	}

	// Skip deleted/failed media — no point scanning.
	if media.Status == domain.MediaStatusDeleted || media.Status == domain.MediaStatusFailed {
		logger.Info("media is deleted/failed, skipping scan", "media_status", media.Status)
		return nil, river.JobCancel(fmt.Errorf("media %s is %s", args.MediaID, media.Status))
	}
	return media, nil
}

// checkScannerAvailable returns a transient error to trigger a retry when the
// scanner is unreachable, escalating to scan_error + JobCancel on the final
// attempt so media cannot stay quarantined in scan_pending forever.
func (w *ScanMediaWorker) checkScannerAvailable(ctx context.Context, job *river.Job[ScanMediaJobArgs], logger *slog.Logger) error {
	if w.scanner.Available(ctx) {
		return nil
	}
	// On the final attempt, transition out of scan_pending to avoid
	// permanent quarantine with no worker left to process it.
	if job.MaxAttempts > 0 && job.Attempt >= job.MaxAttempts {
		if updateErr := w.mediaRepo.UpdateScanStatus(ctx, job.Args.MediaID, domain.ScanStatusError); updateErr != nil {
			logger.Warn("failed to set scan_error after scanner unavailable on final attempt", "error", updateErr)
		}
		return river.JobCancel(fmt.Errorf("scanner not available after max attempts"))
	}
	logger.Warn("scanner not available, will retry")
	return fmt.Errorf("scanner not available")
}

// decryptAndScan downloads the encrypted object, decrypts it through an
// io.Pipe, and streams the plaintext to the scanner under scanTimeout.
func (w *ScanMediaWorker) decryptAndScan(
	ctx context.Context,
	args ScanMediaJobArgs,
	media *domain.Media,
	logger *slog.Logger,
) (*port.ScanResult, error) {
	encrypted, err := w.storage.Download(ctx, media.StorageKey)
	if err != nil {
		return nil, fmt.Errorf("download encrypted media: %w", err)
	}
	defer func() {
		if closeErr := encrypted.Close(); closeErr != nil {
			logger.Warn("close encrypted reader", "error", closeErr)
		}
	}()

	sealedMeta := &crypto.SealedMeta{
		EncryptionMeta: crypto.EncryptionMeta{
			Algorithm:     media.EncryptionAlgo,
			ChunkSize:     media.ChunkSize,
			ChunkCount:    media.ChunkCount,
			PlaintextSize: media.Size,
		},
		WrappedDEK:    media.WrappedDEK,
		WrappingNonce: media.WrappingNonce,
	}

	// Decrypt via io.Pipe and stream to scanner.
	pr, pw := io.Pipe()
	decryptErrCh := make(chan error, 1)
	go func() {
		dErr := w.envelope.OpenStream(ctx, args.OrgID, encrypted, pw, sealedMeta)
		if dErr != nil {
			_ = pw.CloseWithError(dErr) //nolint:errcheck // pipe close cannot fail
			decryptErrCh <- dErr
			return
		}
		_ = pw.Close() //nolint:errcheck // pipe close cannot fail
		decryptErrCh <- nil
	}()

	// Scan with timeout.
	scanCtx, scanCancel := context.WithTimeout(ctx, scanTimeout)
	defer scanCancel()

	result, scanErr := w.scanner.Scan(scanCtx, pr)
	_ = pr.Close() //nolint:errcheck // ensure goroutine completes
	decryptErr := <-decryptErrCh

	if decryptErr != nil {
		logger.Error("decryption failed during scan", "error", decryptErr)
		if updateErr := w.mediaRepo.UpdateScanStatus(ctx, args.MediaID, domain.ScanStatusError); updateErr != nil {
			logger.Warn("failed to set scan_error after decrypt failure", "error", updateErr)
		}
		return nil, river.JobCancel(fmt.Errorf("decrypt media for scan: %w", decryptErr))
	}

	if scanErr != nil {
		logger.Error("scan failed", "error", scanErr)
		if updateErr := w.mediaRepo.UpdateScanStatus(ctx, args.MediaID, domain.ScanStatusError); updateErr != nil {
			logger.Warn("failed to set scan_error", "error", updateErr)
		}
		return nil, fmt.Errorf("scan media: %w", scanErr)
	}
	return result, nil
}

// recordScanResult persists the terminal scan outcome for the media row.
func (w *ScanMediaWorker) recordScanResult(
	ctx context.Context,
	args ScanMediaJobArgs,
	media *domain.Media,
	result *port.ScanResult,
	logger *slog.Logger,
) error {
	if result.Clean {
		if updateErr := w.mediaRepo.UpdateScanStatus(ctx, args.MediaID, domain.ScanStatusClean); updateErr != nil {
			return fmt.Errorf("update scan status to clean: %w", updateErr)
		}
		logger.Info("media scan clean")
		return nil
	}

	logger.Warn("media scan detected threat", "threat_name", result.ThreatName)
	// Mark media status as failed to prevent transcription.
	// Update() maps to UpdateMediaEncryption which does NOT write scan_status,
	// so we call UpdateScanStatus separately after.
	media.Status = domain.MediaStatusFailed
	if updateErr := w.mediaRepo.Update(ctx, media); updateErr != nil {
		return fmt.Errorf("update media status to failed after infection: %w", updateErr)
	}
	// Mark scan_status as infected via dedicated query.
	if updateErr := w.mediaRepo.UpdateScanStatus(ctx, args.MediaID, domain.ScanStatusInfected); updateErr != nil {
		return fmt.Errorf("update scan status to infected: %w", updateErr)
	}
	return nil
}

// isNotFound checks whether the error is a domain.ErrNotFound.
func isNotFound(err error) bool {
	return errors.Is(err, domain.ErrNotFound)
}
