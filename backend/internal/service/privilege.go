package service

import (
	"context"

	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

// PrivilegeService is a fail-closed compatibility stub for shared services
// that retain their generic read guards. Voxis-OSS never constructs it and
// exports no privilege routes, jobs, schema, or provider implementation.
// Keeping the narrow type prevents an unsupported imported row from becoming
// readable if a future composition accidentally wires a guard.
type PrivilegeService struct{}

// IsAudioStreamAllowed blocks an unsupported special-recording media stream.
func (*PrivilegeService) IsAudioStreamAllowed(context.Context, string, string) error {
	return domain.ErrPrivilegeReadBlocked
}

// IsTranscriptReadAllowed blocks an unsupported special-recording transcript.
func (*PrivilegeService) IsTranscriptReadAllowed(context.Context, string, string, port.ReadContext) error {
	return domain.ErrPrivilegeReadBlocked
}

// IsRegularSummaryAllowed blocks a summary tied to unsupported recording data.
func (*PrivilegeService) IsRegularSummaryAllowed(context.Context, string, string) error {
	return domain.ErrPrivilegeOnly
}

// OnRecordingStitchedByMediaID rejects the omitted special-recording workflow.
func (*PrivilegeService) OnRecordingStitchedByMediaID(context.Context, string) error {
	return domain.ErrPrivilegeOnly
}

// OnTranscriptionCompleted rejects completion handling for omitted workflow data.
func (*PrivilegeService) OnTranscriptionCompleted(context.Context, string, string) error {
	return domain.ErrPrivilegeOnly
}

// OnTranscriptionFailed rejects failure handling for omitted workflow data.
func (*PrivilegeService) OnTranscriptionFailed(context.Context, string, string) error {
	return domain.ErrPrivilegeOnly
}

// OnSummaryCompleted rejects summary handling for omitted workflow data.
func (*PrivilegeService) OnSummaryCompleted(context.Context, string, string) error {
	return domain.ErrPrivilegeOnly
}
