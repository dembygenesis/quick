package main

// Concurrency ladder, built on the wallet domain from main.go.
// Everything is in-memory, so no Postgres is needed to run the tests.
//
// Each lesson is a pair of files: NN_name.go (read this) + NN_name_test.go
// (run this). Lessons build on each other. Go in order.
//
// HOW TO STUDY EACH LESSON (10 min each)
//   1. Read the big comment at the top of NN_name.go. It's a story + JS version.
//   2. Read the code under it.
//   3. Run its test with -v and watch it pass.
//   4. Break something on purpose (delete a Lock, a Wait, a close) and re-run
//      with -race. Seeing it fail teaches more than seeing it pass.
//
//	01_goroutines_waitgroup   run work in parallel, wait for all
//	02_channels               get results back without a shared slice
//	03_mutex                  many goroutines, ONE wallet
//	04_worker_pool            cap how many run at once (like sql.DB's pool)
//	05_transactions           commit / rollback (main.go topUp does this for real)
//	06_locks_deadlock         transfers + deadlock avoidance via lock order
//	07_actor_channel_owner    no mutex: ONE goroutine owns the data
//	08_idempotency            duplicate requests apply once (transfers.idempotency_key)
//
// RUN ONE LESSON:   go test -race -v -run 'Step3' ./...
// RUN THEM ALL:     go test -race -v ./...
// (Always -race. The race detector is half the lesson.)
//
// ---------------------------------------------------------------------------
// GO <-> JS CHEAT SHEET (read this the night before)
//
//	Go                           JavaScript / TypeScript
//	---------------------------  ------------------------------------------
//	go f()                       calling an async fn without await
//	sync.WaitGroup               Promise.all
//	chan T                       an async queue / async generator
//	select { case <-a: ... }     Promise.race (plus default / timeout cases)
//	defer f()                    try { } finally { f() }
//	context.Context              AbortController + signal (cancel / deadline)
//	sync.Mutex                   async-mutex (Node needs it far less: 1 thread)
//	return val, err              throw / try-catch (Go has NO exceptions)
//	interface{ Foo() }           TS interface, but any type with a Foo() method
//	                             counts automatically (no `implements` keyword)
//	struct + methods             class (no inheritance; compose instead)
//	panic / recover              an uncaught throw. Only for real bugs, not normal errors
//	zero values (0, "", nil)     like undefined, but every type has a safe default:
//	                             int is 0, string is "", pointer/map/slice is nil
//
// KEY DIFFERENCE: Node = one thread + event loop, so data races mostly cannot
// happen. Go = real parallelism, so shared memory needs locks or channels.
// ---------------------------------------------------------------------------

import "errors"

var (
	ErrInsufficientFunds = errors.New("insufficient funds")
	ErrWalletNotFound    = errors.New("wallet not found")
	ErrInvalidAmount     = errors.New("invalid amount")
	ErrSameWallet        = errors.New("from and to wallet are the same")
	ErrTxDone            = errors.New("tx already committed or rolled back")
)

// Transferer is a promise of "I can Transfer and tell you a Balance".
// Lessons 06 and 07 both keep that promise in different ways (locks vs owner
// goroutine), so the same tests run against both.
// JS analogy: a TS interface. Any type with these two methods counts; no
// `implements` keyword needed.
type Transferer interface {
	Transfer(from, to, amount int64) error
	Balance(id int64) (int64, error)
}
