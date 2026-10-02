package main

import (
	"context"
	"errors"

	"github.com/voxis/backend/internal/port"
)

func clamAVProbe(scanner port.MalwareScanner) func(context.Context) error {
	return func(ctx context.Context) error {
		if scanner == nil || !scanner.Available(ctx) {
			return errors.New("clamav is unavailable")
		}
		return nil
	}
}
