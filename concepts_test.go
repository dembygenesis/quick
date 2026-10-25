package main

import (
	"errors"
	"maps"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Run with the race detector, it is half the lesson:
//
//	go test -race -v ./...

// mustFinish fails the test instead of hanging forever if fn deadlocks.
// It uses select: wait on whichever channel fires first.
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

func TestStep1TopUpAll(t *testing.T) {
	tests := []struct {
		name    string
		wallets []Wallet
		amount  int64
		want    []Wallet
	}{
		{
			name:    "no wallets",
			wallets: []Wallet{},
			amount:  50,
			want:    []Wallet{},
		},
		{
			name:    "order is preserved even though goroutines finish in any order",
			wallets: []Wallet{{ID: 1, Owner: "a", Balance: 0}, {ID: 2, Owner: "b", Balance: 10}, {ID: 3, Owner: "c", Balance: 99}},
			amount:  50,
			want:    []Wallet{{ID: 1, Owner: "a", Balance: 50}, {ID: 2, Owner: "b", Balance: 60}, {ID: 3, Owner: "c", Balance: 149}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Step1TopUpAll(tt.wallets, tt.amount)

			if !slices.Equal(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStep2TopUpAll(t *testing.T) {
	tests := []struct {
		name    string
		wallets []Wallet
		amount  int64
		want    map[int64]int64
	}{
		{
			name:    "no wallets: channel is closed straight away, range ends",
			wallets: nil,
			amount:  50,
			want:    map[int64]int64{},
		},
		{
			name:    "every wallet reports back exactly once",
			wallets: []Wallet{{ID: 1, Balance: 0}, {ID: 2, Balance: 10}, {ID: 3, Balance: 99}},
			amount:  50,
			want:    map[int64]int64{1: 50, 2: 60, 3: 149},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got map[int64]int64
			mustFinish(t, 2*time.Second, func() { got = Step2TopUpAll(tt.wallets, tt.amount) })

			if !maps.Equal(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStep3Deposit(t *testing.T) {
	tests := []struct {
		name       string
		goroutines int
		each       int64
		want       int64
	}{
		{name: "one goroutine", goroutines: 1, each: 5, want: 5},
		{name: "1000 goroutines, no lost updates", goroutines: 1000, each: 1, want: 1000},
		{name: "big amounts", goroutines: 200, each: 250, want: 50_000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Step3Deposit(tt.goroutines, tt.each)

			if got != tt.want {
				t.Fatalf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestStep3Withdraw(t *testing.T) {
	tests := []struct {
		name          string
		start         int64
		attempts      int
		amount        int64
		wantSucceeded int
		wantBalance   int64
	}{
		{name: "enough for everyone", start: 100, attempts: 10, amount: 5, wantSucceeded: 10, wantBalance: 50},
		{name: "exactly drained", start: 100, attempts: 100, amount: 1, wantSucceeded: 100, wantBalance: 0},
		{name: "200 race for 100: exactly 100 win, never negative", start: 100, attempts: 200, amount: 1, wantSucceeded: 100, wantBalance: 0},
		{name: "nothing to withdraw", start: 0, attempts: 50, amount: 1, wantSucceeded: 0, wantBalance: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSucceeded, gotBalance := Step3Withdraw(tt.start, tt.attempts, tt.amount)

			if gotSucceeded != tt.wantSucceeded || gotBalance != tt.wantBalance {
				t.Fatalf("got (%d, %d), want (%d, %d)", gotSucceeded, gotBalance, tt.wantSucceeded, tt.wantBalance)
			}
		})
	}
}

func TestStep4WorkerPool(t *testing.T) {
	tests := []struct {
		name    string
		workers int
		jobs    int
	}{
		{name: "one worker is fully serial", workers: 1, jobs: 10},
		{name: "4 workers, 20 jobs", workers: 4, jobs: 20},
		{name: "more workers than jobs", workers: 10, jobs: 3},
		{name: "no jobs: workers still exit cleanly", workers: 3, jobs: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jobs := make([]TopUpJob, tt.jobs)
			var wantSum int64
			for i := range jobs {
				jobs[i] = TopUpJob{WalletID: int64(i), Amount: int64(i + 1)}
				wantSum += jobs[i].Amount
			}

			var inFlight, peak atomic.Int32
			var gotSum, gotCount atomic.Int64
			apply := func(j TopUpJob) {
				cur := inFlight.Add(1)
				for { // record the highest concurrency we ever saw
					p := peak.Load()
					if cur <= p || peak.CompareAndSwap(p, cur) {
						break
					}
				}
				time.Sleep(5 * time.Millisecond) // hold the slot so workers overlap
				inFlight.Add(-1)

				gotSum.Add(j.Amount)
				gotCount.Add(1)
			}

			mustFinish(t, 5*time.Second, func() { Step4WorkerPool(tt.workers, jobs, apply) })

			if int(gotCount.Load()) != tt.jobs {
				t.Fatalf("ran %d jobs, want %d (each exactly once)", gotCount.Load(), tt.jobs)
			}
			if gotSum.Load() != wantSum {
				t.Fatalf("sum of amounts %d, want %d", gotSum.Load(), wantSum)
			}
			if int(peak.Load()) > tt.workers {
				t.Fatalf("peak concurrency %d exceeded %d workers", peak.Load(), tt.workers)
			}
		})
	}
}

func TestStep5Transfer(t *testing.T) {
	tests := []struct {
		name    string
		from    int64
		to      int64
		amount  int64
		wantErr error
		want    map[int64]int64
	}{
		{
			name: "commit: both sides move", from: 1, to: 2, amount: 30,
			want: map[int64]int64{1: 70, 2: 30},
		},
		{
			name: "rollback: insufficient funds leaves BOTH untouched", from: 1, to: 2, amount: 101,
			wantErr: ErrInsufficientFunds,
			want:    map[int64]int64{1: 100, 2: 0},
		},
		{
			name: "rollback: unknown destination must not debit the source", from: 1, to: 99, amount: 10,
			wantErr: ErrWalletNotFound,
			want:    map[int64]int64{1: 100, 2: 0},
		},
		{
			name: "rollback: invalid amount", from: 1, to: 2, amount: 0,
			wantErr: ErrInvalidAmount,
			want:    map[int64]int64{1: 100, 2: 0},
		},
		{
			name: "transfer to self nets zero", from: 1, to: 1, amount: 10,
			want: map[int64]int64{1: 100, 2: 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bank := NewBank(map[int64]int64{1: 100, 2: 0})

			err := Step5Transfer(bank, tt.from, tt.to, tt.amount)

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

func TestTx_DoneIsFinal(t *testing.T) {
	tests := []struct {
		name string
		run  func(tx *Tx) error
		want error
	}{
		{name: "commit twice", run: func(tx *Tx) error { _ = tx.Commit(); return tx.Commit() }, want: ErrTxDone},
		{name: "commit after rollback", run: func(tx *Tx) error { tx.Rollback(); return tx.Commit() }, want: ErrTxDone},
		{name: "rollback after commit is a harmless no-op (defer-safe)", run: func(tx *Tx) error { err := tx.Commit(); tx.Rollback(); return err }, want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := NewBank(map[int64]int64{1: 10}).Begin()

			if err := tt.run(tx); !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

// transferers runs Step 6 (locks) and Step 7 (channel owner) through the same
// tests: two designs, one contract.
var transferers = []struct {
	name string
	make func(t *testing.T, balances map[int64]int64) Transferer
}{
	{
		name: "locked bank (mutex per wallet)",
		make: func(t *testing.T, b map[int64]int64) Transferer { return NewLockedBank(b) },
	},
	{
		name: "actor bank (channel owner)",
		make: func(t *testing.T, b map[int64]int64) Transferer {
			a := NewActorBank(b)
			t.Cleanup(a.Close)
			return a
		},
	},
}

func TestStep6And7Transfer(t *testing.T) {
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

	for _, impl := range transferers {
		for _, tt := range tests {
			t.Run(impl.name+"/"+tt.name, func(t *testing.T) {
				bank := impl.make(t, map[int64]int64{1: 100, 2: 50})

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
}

// Ring of transfers 1->2, 2->3, 3->1 plus the reverse, all at once.
// Without lock ordering this is the textbook deadlock; mustFinish catches it.
func TestStep6And7Transfer_ConcurrentRing(t *testing.T) {
	tests := []struct {
		name   string
		rounds int
	}{
		{name: "100 rounds each way", rounds: 100},
		{name: "500 rounds each way", rounds: 500},
	}

	ring := [][2]int64{{1, 2}, {2, 3}, {3, 1}, {2, 1}, {3, 2}, {1, 3}}

	for _, impl := range transferers {
		for _, tt := range tests {
			t.Run(impl.name+"/"+tt.name, func(t *testing.T) {
				bank := impl.make(t, map[int64]int64{1: 1000, 2: 1000, 3: 1000})

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
}

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
