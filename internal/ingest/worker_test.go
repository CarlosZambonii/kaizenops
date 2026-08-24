package ingest

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestPoolRunsAllSubmittedTasks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var processed atomic.Int32
	pool := NewPool(ctx, 4, 8, nil)

	const total = 50
	for i := 0; i < total; i++ {
		if err := pool.Submit(ctx, func(ctx context.Context) error {
			processed.Add(1)
			return nil
		}); err != nil {
			t.Fatalf("Submit() unexpected error: %v", err)
		}
	}

	pool.Shutdown()

	if got := processed.Load(); got != total {
		t.Fatalf("processed = %d, want %d", got, total)
	}
}

func TestPoolCollectsTaskErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var errCount atomic.Int32
	pool := NewPool(ctx, 2, 4, func(err error) {
		errCount.Add(1)
	})

	boom := errors.New("boom")
	for i := 0; i < 3; i++ {
		_ = pool.Submit(ctx, func(ctx context.Context) error {
			return boom
		})
	}

	pool.Shutdown()

	if got := errCount.Load(); got != 3 {
		t.Fatalf("errCount = %d, want 3", got)
	}
}

func TestPoolSubmitRespectsContextCancellation(t *testing.T) {
	ctx := context.Background()
	pool := NewPool(ctx, 1, 1, nil)

	block := make(chan struct{})
	// Ocupa o único worker e enche o buffer de 1, para forçar Submit a
	// bloquear no terceiro envio.
	_ = pool.Submit(ctx, func(ctx context.Context) error {
		<-block
		return nil
	})
	_ = pool.Submit(ctx, func(ctx context.Context) error {
		return nil
	})

	submitCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()

	err := pool.Submit(submitCtx, func(ctx context.Context) error {
		return nil
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Submit() error = %v, want context.DeadlineExceeded", err)
	}

	close(block)
	pool.Shutdown()
}
