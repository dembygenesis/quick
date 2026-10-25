package main

// A ladder of concurrency concepts, each step built on the wallet domain
// from main.go. Everything here is in-memory so the tests need no Postgres.
//
//	Step1  goroutines + WaitGroup      run work in parallel, wait for all
//	Step2  channels                    get results back without a shared slice
//	Step3  Mutex                       many goroutines, ONE wallet
//	Step4  worker pool                 cap how many run at once (like sql.DB's pool)
//	Step5  Tx: commit / rollback       all-or-nothing (main.go topUp does this for real)
//	Step6  per-wallet locks            transfers + deadlock avoidance via lock order
//	Step7  channel as owner (actor)    no mutex: ONE goroutine owns the data
//	Step8  idempotency key             duplicate requests apply once (transfers.idempotency_key)
//
// Run: go test -race -v -run 'Step' ./...

import (
	"errors"
	"sync"
)

var (
	ErrInsufficientFunds = errors.New("insufficient funds")
	ErrWalletNotFound    = errors.New("wallet not found")
	ErrInvalidAmount     = errors.New("invalid amount")
	ErrSameWallet        = errors.New("from and to wallet are the same")
	ErrTxDone            = errors.New("tx already committed or rolled back")
)

// ---------------------------------------------------------------------------
// Step 1: goroutines + WaitGroup
//
// `go f()` starts f on its own goroutine and returns immediately, so main
// would race ahead and return before the work is done. A WaitGroup is a
// counter: Add(1) before starting, Done() when finished, Wait() blocks until
// the counter hits zero.
//
// Why no lock? Each goroutine writes ONLY to its own slot out[i]. Nobody
// shares a slot, so there is nothing to race on.
// ---------------------------------------------------------------------------

func Step1TopUpAll(wallets []Wallet, amount int64) []Wallet {
	out := make([]Wallet, len(wallets))

	var wg sync.WaitGroup
	for i, w := range wallets {
		wg.Add(1) // Add BEFORE `go`, never inside the goroutine
		go func() {
			defer wg.Done()
			w.Balance += amount // w is this iteration's own copy
			out[i] = w
		}()
	}
	wg.Wait()

	return out
}

// ---------------------------------------------------------------------------
// Step 2: channels
//
// A channel is a typed pipe between goroutines. Instead of writing into a
// shared slice, workers SEND results and the caller RECEIVES them.
//
// The classic shape (fan-out / fan-in):
//   - fan out: one goroutine per wallet sends on `results`
//   - fan in:  the caller ranges over `results`
//   - a tiny goroutine waits for the WaitGroup then close()s the channel,
//     which is what ends the caller's `range`. Forget the close -> deadlock.
//
// Only the caller touches the map, so the map needs no lock.
// ---------------------------------------------------------------------------

type TopUpResult struct {
	WalletID int64
	Balance  int64
}

func Step2TopUpAll(wallets []Wallet, amount int64) map[int64]int64 {
	results := make(chan TopUpResult) // unbuffered: a send waits for a receiver

	var wg sync.WaitGroup
	for _, w := range wallets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- TopUpResult{WalletID: w.ID, Balance: w.Balance + amount}
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	out := make(map[int64]int64, len(wallets))
	for r := range results {
		out[r.WalletID] = r.Balance
	}

	return out
}

// ---------------------------------------------------------------------------
// Step 3: Mutex
//
// Steps 1-2 avoided sharing. Now we share on purpose: many goroutines hit
// ONE wallet. `balance += n` is read, add, write: two goroutines can read the
// same old value and one update is lost. A Mutex lets one goroutine in at a
// time. `go test -race` flags the bug if you delete the Lock.
//
// The lock must cover check AND act. Withdraw checks the balance and subtracts
// under one Lock; locking each half separately would let two withdrawals both
// pass the check, and the wallet goes negative. (In Postgres the CHECK
// (balance >= 0) constraint is the last line of defence.)
// ---------------------------------------------------------------------------

type SafeWallet struct {
	mu      sync.Mutex
	balance int64
}

func (s *SafeWallet) Deposit(n int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.balance += n
}

func (s *SafeWallet) Withdraw(n int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.balance < n {
		return ErrInsufficientFunds
	}
	s.balance -= n
	return nil
}

func (s *SafeWallet) Balance() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.balance
}

// Step3Deposit: `goroutines` depositors, each depositing `each`.
func Step3Deposit(goroutines int, each int64) int64 {
	var w SafeWallet

	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.Deposit(each)
		}()
	}
	wg.Wait()

	return w.Balance()
}

// Step3Withdraw: `attempts` goroutines race to withdraw `amount` from a wallet
// holding `start`. Returns how many succeeded and what is left.
func Step3Withdraw(start int64, attempts int, amount int64) (succeeded int, balance int64) {
	w := SafeWallet{balance: start}

	var (
		wg sync.WaitGroup
		mu sync.Mutex // guards `succeeded`, which many goroutines bump
	)
	for range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if w.Withdraw(amount) == nil {
				mu.Lock()
				succeeded++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	return succeeded, w.Balance()
}

// ---------------------------------------------------------------------------
// Step 4: worker pool
//
// One goroutine per job is fine for 10 jobs and a disaster for 10 million (or
// for a DB that allows 10 connections). A pool starts a FIXED number of
// workers that pull jobs off a channel until it is closed.
//
// This is what sql.DB does with connections: SetMaxOpenConns(n) is "n workers".
// ---------------------------------------------------------------------------

type TopUpJob struct {
	WalletID int64
	Amount   int64
}

func Step4WorkerPool(workers int, jobs []TopUpJob, apply func(TopUpJob)) {
	queue := make(chan TopUpJob)

	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range queue { // loop ends when queue is closed and drained
				apply(job)
			}
		}()
	}

	for _, j := range jobs {
		queue <- j
	}
	close(queue) // "no more jobs": lets the workers' range loops finish
	wg.Wait()
}

// ---------------------------------------------------------------------------
// Step 5: Tx, commit and rollback
//
// A transaction is "stage changes, then apply ALL or NONE". Same contract as
// db.BeginTx in main.go topUp:
//
//	tx, _ := db.BeginTx(...)
//	defer tx.Rollback()   // safe no-op after Commit; saves us on every early return
//	... statements ...
//	return tx.Commit()
//
// Here Tx stages balance deltas. Commit validates every wallet, then applies
// everything under the bank lock. Rollback just throws the deltas away.
// (Postgres would fail at the offending statement; we fail at Commit. Same
// outcome: nothing half-applied.)
// ---------------------------------------------------------------------------

type Bank struct {
	mu       sync.Mutex
	balances map[int64]int64
}

func NewBank(balances map[int64]int64) *Bank {
	b := &Bank{balances: make(map[int64]int64, len(balances))}
	for id, bal := range balances {
		b.balances[id] = bal
	}
	return b
}

func (b *Bank) Balance(id int64) (int64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	bal, ok := b.balances[id]
	if !ok {
		return 0, ErrWalletNotFound
	}
	return bal, nil
}

type Tx struct {
	bank   *Bank
	deltas map[int64]int64
	done   bool
}

func (b *Bank) Begin() *Tx {
	return &Tx{bank: b, deltas: map[int64]int64{}}
}

func (t *Tx) Add(walletID, delta int64) {
	t.deltas[walletID] += delta
}

func (t *Tx) Commit() error {
	if t.done {
		return ErrTxDone
	}
	t.done = true

	b := t.bank
	b.mu.Lock()
	defer b.mu.Unlock()

	// Pass 1: validate everything. Touch nothing yet.
	for id, d := range t.deltas {
		bal, ok := b.balances[id]
		if !ok {
			return ErrWalletNotFound
		}
		if bal+d < 0 {
			return ErrInsufficientFunds
		}
	}

	// Pass 2: apply. Cannot fail, so we never end up half-applied.
	for id, d := range t.deltas {
		b.balances[id] += d
	}
	return nil
}

func (t *Tx) Rollback() {
	t.done = true
	t.deltas = nil
}

func Step5Transfer(b *Bank, from, to, amount int64) error {
	if amount <= 0 {
		return ErrInvalidAmount
	}

	tx := b.Begin()
	defer tx.Rollback()

	tx.Add(from, -amount)
	tx.Add(to, amount)

	return tx.Commit()
}

// ---------------------------------------------------------------------------
// Step 6: per-wallet locks + deadlock avoidance
//
// Step 5 used ONE lock for the whole bank: correct, but every transfer waits
// for every other. Real systems lock per row (Postgres: SELECT ... FOR UPDATE),
// so unrelated transfers run in parallel.
//
// New danger: a transfer needs TWO locks.
//
//	A -> B locks A, then wants B
//	B -> A locks B, then wants A    => each holds what the other needs. Deadlock.
//
// Fix: every transfer takes locks in the same global order (lowest id first).
// Then nobody can hold the higher lock while waiting on the lower one.
// In SQL this is: SELECT ... WHERE id IN ($1,$2) ORDER BY id FOR UPDATE.
// ---------------------------------------------------------------------------

// Transferer is what Steps 6 and 7 both offer: same behaviour, two designs.
type Transferer interface {
	Transfer(from, to, amount int64) error
	Balance(id int64) (int64, error)
}

type lockedWallet struct {
	mu      sync.Mutex
	balance int64
}

type LockedBank struct {
	wallets map[int64]*lockedWallet // built once, never mutated: safe to read unlocked
}

func NewLockedBank(balances map[int64]int64) *LockedBank {
	lb := &LockedBank{wallets: make(map[int64]*lockedWallet, len(balances))}
	for id, bal := range balances {
		lb.wallets[id] = &lockedWallet{balance: bal}
	}
	return lb
}

func (lb *LockedBank) Balance(id int64) (int64, error) {
	w, ok := lb.wallets[id]
	if !ok {
		return 0, ErrWalletNotFound
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.balance, nil
}

func (lb *LockedBank) Transfer(from, to, amount int64) error {
	if amount <= 0 {
		return ErrInvalidAmount
	}
	if from == to {
		return ErrSameWallet // would Lock the same mutex twice: instant self-deadlock
	}
	src, ok := lb.wallets[from]
	dst, ok2 := lb.wallets[to]
	if !ok || !ok2 {
		return ErrWalletNotFound
	}

	first, second := src, dst
	if from > to {
		first, second = dst, src // lock the lower id first, whatever the direction
	}
	first.mu.Lock()
	defer first.mu.Unlock()
	second.mu.Lock()
	defer second.mu.Unlock()

	if src.balance < amount {
		return ErrInsufficientFunds
	}
	src.balance -= amount
	dst.balance += amount
	return nil
}

// ---------------------------------------------------------------------------
// Step 7: channel as owner (actor)
//
// Go proverb: "don't communicate by sharing memory; share memory by
// communicating". Instead of N goroutines fighting over locks, ONE goroutine
// owns the map and everyone else sends it requests over a channel. Requests
// are handled one at a time, so no mutex exists anywhere in this type.
//
// A request is just a func to run inside the owner goroutine; do() blocks
// until it has run.
//
// Call Close() when done. Using the bank after Close panics (send on closed
// channel), which is a feature: it is a bug in the caller.
// ---------------------------------------------------------------------------

type ActorBank struct {
	ops      chan func(map[int64]int64)
	finished chan struct{}
}

func NewActorBank(balances map[int64]int64) *ActorBank {
	owned := make(map[int64]int64, len(balances)) // ONLY the loop goroutine touches this
	for id, bal := range balances {
		owned[id] = bal
	}

	a := &ActorBank{
		ops:      make(chan func(map[int64]int64)),
		finished: make(chan struct{}),
	}
	go func() {
		defer close(a.finished)
		for op := range a.ops {
			op(owned)
		}
	}()
	return a
}

func (a *ActorBank) do(op func(map[int64]int64)) {
	done := make(chan struct{})
	a.ops <- func(m map[int64]int64) {
		op(m)
		close(done)
	}
	<-done
}

func (a *ActorBank) Close() {
	close(a.ops)
	<-a.finished
}

func (a *ActorBank) Balance(id int64) (bal int64, err error) {
	a.do(func(m map[int64]int64) {
		var ok bool
		if bal, ok = m[id]; !ok {
			err = ErrWalletNotFound
		}
	})
	return bal, err
}

func (a *ActorBank) Transfer(from, to, amount int64) (err error) {
	if amount <= 0 {
		return ErrInvalidAmount
	}
	if from == to {
		return ErrSameWallet
	}
	a.do(func(m map[int64]int64) {
		src, ok := m[from]
		_, ok2 := m[to]
		switch {
		case !ok || !ok2:
			err = ErrWalletNotFound
		case src < amount:
			err = ErrInsufficientFunds
		default:
			m[from] -= amount
			m[to] += amount
		}
	})
	return err
}

// ---------------------------------------------------------------------------
// Step 8: idempotency key (mutex + channel together)
//
// Clients retry. A timeout does not tell them whether the transfer happened,
// so they resend with the same idempotency key, and the money must move ONCE.
// The transfers table enforces this with UNIQUE (idempotency_key).
//
// In memory: a mutex-guarded map of key -> in-flight call.
//   - first caller with a key runs the transfer
//   - concurrent duplicates find the entry and wait on its `done` channel
//     (close() wakes ALL waiters at once), then reuse the result
//   - a FAILED transfer forgets the key, like a rolled-back INSERT would,
//     so the client may retry with the same key
// ---------------------------------------------------------------------------

type call struct {
	done chan struct{}
	err  error
}

type IdempotentBank struct {
	bank  Transferer
	mu    sync.Mutex
	calls map[string]*call
}

func NewIdempotentBank(bank Transferer) *IdempotentBank {
	return &IdempotentBank{bank: bank, calls: map[string]*call{}}
}

func (ib *IdempotentBank) Transfer(key string, from, to, amount int64) error {
	ib.mu.Lock()
	if c, ok := ib.calls[key]; ok {
		ib.mu.Unlock() // never wait on a channel while holding the lock
		<-c.done
		return c.err
	}
	c := &call{done: make(chan struct{})}
	ib.calls[key] = c
	ib.mu.Unlock()

	c.err = ib.bank.Transfer(from, to, amount)

	if c.err != nil {
		ib.mu.Lock()
		delete(ib.calls, key)
		ib.mu.Unlock()
	}
	close(c.done) // happens-before every waiter's <-c.done, so reading c.err is safe
	return c.err
}
