package main

import "sync"

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
//
// JS ANALOGY
//   go f()        ~ calling an async function and NOT awaiting it (fire it off)
//   WaitGroup     ~ Promise.all([...])   Add(1) = "one more promise", Done() = "it resolved"
//   wg.Wait()     ~ await Promise.all(...)
// Big difference: Node runs your JS on ONE thread, so two callbacks never touch
// memory at the same instant. Goroutines run on real CPU cores IN PARALLEL, so
// shared memory can be corrupted (a "data race"). That is why Steps 3+ exist.
// Gotcha you'd know from JS: the old `for (var i...) setTimeout(() => i)` bug.
// Go before 1.22 had it too (all closures shared one `i`). Now each iteration
// gets its own copy, like `let`.
//
// INTERVIEW LINE: "A goroutine is a cheap (~2KB stack) function running
// concurrently, scheduled by the Go runtime, not an OS thread. I use a
// WaitGroup to wait for a known number of them."
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
