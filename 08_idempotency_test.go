package main

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestStep8Idempotent_Sequential(t *testing.T) {
	type req struct {
		key     string
		amount  int64
		wantErr error
	}

	tests := []struct {
		name     string
		reqs     []req
		wantFrom int64
	}{
		{
			name:     "same key twice moves money once",
			reqs:     []req{{key: "k1", amount: 10}, {key: "k1", amount: 10}},
			wantFrom: 90,
		},
		{
			name:     "different keys each move money",
			reqs:     []req{{key: "k1", amount: 10}, {key: "k2", amount: 10}},
			wantFrom: 80,
		},
		{
			name: "failed attempt forgets the key, retry may succeed",
			reqs: []req{
				{key: "k1", amount: 5000, wantErr: ErrInsufficientFunds},
				{key: "k1", amount: 10},
			},
			wantFrom: 90,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ib := NewIdempotentBank(NewLockedBank(map[int64]int64{1: 100, 2: 0}))

			for i, r := range tt.reqs {
				if err := ib.Transfer(r.key, 1, 2, r.amount); !errors.Is(err, r.wantErr) {
					t.Fatalf("req %d: err = %v, want %v", i, err, r.wantErr)
				}
			}

			if got, _ := ib.bank.Balance(1); got != tt.wantFrom {
				t.Fatalf("wallet 1 = %d, want %d", got, tt.wantFrom)
			}
		})
	}
}

func TestStep8Idempotent_ConcurrentDuplicates(t *testing.T) {
	tests := []struct {
		name       string
		duplicates int
		wantFrom   int64
	}{
		{name: "50 simultaneous retries of one request", duplicates: 50, wantFrom: 90},
		{name: "500 simultaneous retries of one request", duplicates: 500, wantFrom: 90},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ib := NewIdempotentBank(NewLockedBank(map[int64]int64{1: 100, 2: 0}))

			var failed atomic.Int64
			mustFinish(t, 5*time.Second, func() {
				var wg sync.WaitGroup
				for range tt.duplicates {
					wg.Add(1)
					go func() {
						defer wg.Done()
						if ib.Transfer("same-key", 1, 2, 10) != nil {
							failed.Add(1)
						}
					}()
				}
				wg.Wait()
			})

			if failed.Load() != 0 {
				t.Fatalf("%d callers got an error, want 0 (duplicates share the first result)", failed.Load())
			}
			if got, _ := ib.bank.Balance(1); got != tt.wantFrom {
				t.Fatalf("wallet 1 = %d, want %d: money moved more than once", got, tt.wantFrom)
			}
		})
	}
}
