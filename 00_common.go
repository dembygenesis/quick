package main

// Concurrency lessons on the wallet domain from main.go. In-memory, no Postgres needed.
// Each lesson: NN_name.go (read) + NN_name_test.go (run). Go in order.
//
//	01 goroutines + WaitGroup    05 transactions
//	02 channels                  06 per-wallet locks, deadlock
//	03 mutex                     07 one goroutine owns the data
//	04 worker pool               08 idempotency key
//
// Run one:  go test -race -v -run Step3 ./...
// Run all:  go test -race -v ./...
// Tip: delete a Lock/Wait/close on purpose and re-run to see it break.
//
// Go <-> JS
//
//	go f()                  async call without await
//	sync.WaitGroup          Promise.all
//	chan                    async queue / stream
//	select                  Promise.race
//	defer                   finally
//	context.Context         AbortController
//	return val, err         throw / try-catch (Go has no exceptions)
//	interface               TS interface, but no `implements` needed
//
// Node is one thread, so data races mostly can't happen.
// Go runs goroutines in parallel, so shared data needs a lock or a channel.

import "errors"

var (
	ErrInsufficientFunds = errors.New("insufficient funds")
	ErrWalletNotFound    = errors.New("wallet not found")
	ErrInvalidAmount     = errors.New("invalid amount")
	ErrSameWallet        = errors.New("from and to wallet are the same")
	ErrTxDone            = errors.New("tx already committed or rolled back")
)

// Transferer: lessons 06 and 07 both implement it, so the same tests run on both.
type Transferer interface {
	Transfer(from, to, amount int64) error
	Balance(id int64) (int64, error)
}
