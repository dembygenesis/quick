package main

import "sync"

// ---------------------------------------------------------------------------
// LESSON 1: goroutines + WaitGroup. Do many things at once, wait for all.
//
// JS:  await Promise.all(wallets.map(async w => ...))
// Go:  go func(){...}()  = call an async fn without await
//      wg.Add / Done / Wait = Promise.all, done by hand
//
// Each goroutine writes only its own out[i], so no lock needed.
// Node is single-threaded; goroutines run truly in parallel (lessons 3+ cover the danger).
// ---------------------------------------------------------------------------

func Step1TopUpAll(wallets []Wallet, amount int64) []Wallet {
	out := make([]Wallet, len(wallets))

	var wg sync.WaitGroup
	for i, w := range wallets {
		wg.Add(1) // before `go`, not inside
		go func() {
			defer wg.Done()
			w.Balance += amount
			out[i] = w
		}()
	}
	wg.Wait()

	return out
}
