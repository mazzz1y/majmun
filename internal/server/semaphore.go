package server

import (
	"context"
	"fmt"
	"majmun/internal/app"
	"majmun/internal/ctxutil"
	"majmun/internal/metrics"
	"majmun/internal/utils"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
)

const (
	semaphoreTimeout = 3 * time.Second
)

func (s *Server) acquireSemaphores(ctx context.Context) bool {
	c := ctxutil.Client(ctx).(*app.Client)

	g, gCtx := errgroup.WithContext(ctx)

	acquireSem := func(sem *semaphore.Weighted, reason string) func() error {
		return func() error {
			if sem == nil || utils.AcquireSemaphore(gCtx, sem, semaphoreTimeout, reason) {
				return nil
			}
			metrics.IncStreamsFailures(gCtx, reason)
			return fmt.Errorf("failed to acquire semaphore: %s", reason)
		}
	}

	if managerSem := s.manager.Semaphore(); managerSem != nil {
		g.Go(acquireSem(managerSem, metrics.FailureReasonGlobalLimit))
	}

	if clientSem := c.Semaphore(); clientSem != nil {
		g.Go(acquireSem(clientSem, metrics.FailureReasonClientLimit))
	}

	return g.Wait() == nil
}

func (s *Server) releaseSemaphores(ctx context.Context) {
	c := ctxutil.Client(ctx).(*app.Client)

	if managerSem := s.manager.Semaphore(); managerSem != nil {
		managerSem.Release(1)
	}

	if clientSem := c.Semaphore(); clientSem != nil {
		clientSem.Release(1)
	}
}
