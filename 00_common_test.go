package main

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// mustFinish fails the test if fn hangs (a deadlock hangs, it does not crash).
// JS: await Promise.race([fn(), timeout(d)])
func mustFinish(t *testing.T, d time.Duration, fn func()) {
	t.Helper()

	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()

	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("did not finish within %s: deadlock?", d)
	}
}

// lessons 06 and 07 share one contract, so they share these tests
type newTransferer func(t *testing.T, balances map[int64]int64) Transferer

func runTransferCases(t *testing.T, newBank newTransferer) {
	tests := []struct {
		name    string
		from    int64
		to      int64
		amount  int64
		wantErr error
		want    map[int64]int64
	}{
		{name: "moves money low id to high id", from: 1, to: 2, amount: 40, want: map[int64]int64{1: 60, 2: 90}},
		{name: "moves money high id to low id (reverse lock order path)", from: 2, to: 1, amount: 20, want: map[int64]int64{1: 120, 2: 30}},
		{name: "insufficient funds", from: 2, to: 1, amount: 60, wantErr: ErrInsufficientFunds, want: map[int64]int64{1: 100, 2: 50}},
		{name: "invalid amount", from: 1, to: 2, amount: 0, wantErr: ErrInvalidAmount, want: map[int64]int64{1: 100, 2: 50}},
		{name: "same wallet", from: 1, to: 1, amount: 5, wantErr: ErrSameWallet, want: map[int64]int64{1: 100, 2: 50}},
		{name: "unknown wallet", from: 1, to: 9, amount: 5, wantErr: ErrWalletNotFound, want: map[int64]int64{1: 100, 2: 50}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bank := newBank(t, map[int64]int64{1: 100, 2: 50})

			err := bank.Transfer(tt.from, tt.to, tt.amount)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			for id, want := range tt.want {
				if got, _ := bank.Balance(id); got != want {
					t.Fatalf("wallet %d = %d, want %d", id, got, want)
				}
			}
		})
	}
}

// Hundreds of transfers in a circle and back. Each wallet must end where it started.
// Wrong lock order would deadlock; mustFinish catches that.
func runConcurrentRing(t *testing.T, newBank newTransferer) {
	tests := []struct {
		name   string
		rounds int
	}{
		{name: "100 rounds each way", rounds: 100},
		{name: "500 rounds each way", rounds: 500},
	}

	ring := [][2]int64{{1, 2}, {2, 3}, {3, 1}, {2, 1}, {3, 2}, {1, 3}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bank := newBank(t, map[int64]int64{1: 1000, 2: 1000, 3: 1000})

			var failed atomic.Int64
			mustFinish(t, 10*time.Second, func() {
				var wg sync.WaitGroup
				for _, leg := range ring {
					for range tt.rounds {
						wg.Add(1)
						go func() {
							defer wg.Done()
							if bank.Transfer(leg[0], leg[1], 1) != nil {
								failed.Add(1)
							}
						}()
					}
				}
				wg.Wait()
			})

			if failed.Load() != 0 {
				t.Fatalf("%d transfers failed, want 0", failed.Load())
			}
			// Every leg has a mirror leg, so each wallet must end where it began.
			for id := int64(1); id <= 3; id++ {
				if got, _ := bank.Balance(id); got != 1000 {
					t.Fatalf("wallet %d = %d, want 1000 (money created or destroyed)", id, got)
				}
			}
		})
	}
}
