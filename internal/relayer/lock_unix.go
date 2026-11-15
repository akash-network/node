//go:build unix

package relayer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

var errPayerBusy = errors.New("payer lock unavailable")

// Each acquisition opens the original persistent inode. Never unlink a lock:
// another process may already have it open, including the legacy TS helper.
func withFileLock(ctx context.Context, path string, wait time.Duration, action func() error) (bool, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return false, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return false, err
	}
	defer file.Close()

	deadline := time.Now().Add(wait)
	for {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			break
		}
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			return false, err
		}
		if !time.Now().Before(deadline) {
			return false, nil
		}
		timer := time.NewTimer(min(50*time.Millisecond, time.Until(deadline)))
		select {
		case <-ctx.Done():
			timer.Stop()
			return false, ctx.Err()
		case <-timer.C:
		}
	}
	defer unix.Flock(int(file.Fd()), unix.LOCK_UN)
	return true, action()
}

func (s *service) withPayer(ctx context.Context, action func(*payerSession) error) (bool, error) {
	return withFileLock(ctx, s.store.payerLock, 0, func() error {
		return action(&payerSession{service: s})
	})
}

func (s *service) withRequiredPayer(ctx context.Context, action func(*payerSession) error) error {
	acquired, err := s.withPayer(ctx, action)
	if err != nil {
		return err
	}
	if !acquired {
		return errPayerBusy
	}
	return nil
}
