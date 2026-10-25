package main

import "sync"

// ---------------------------------------------------------------------------
// LESSON 2: channels. A pipe goroutines send results through.
//
// JS:  closest is an async queue / stream
//      ch <- v = push,  <-ch = await next,  range ch = for await,  close = stream ends
//
// Sender closes the channel, never the receiver.
// Forget close() and the range loop waits forever.
// ---------------------------------------------------------------------------

type TopUpResult struct {
	WalletID int64
	Balance  int64
}

func Step2TopUpAll(wallets []Wallet, amount int64) map[int64]int64 {
	results := make(chan TopUpResult)

	var wg sync.WaitGroup
	for _, w := range wallets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- TopUpResult{WalletID: w.ID, Balance: w.Balance + amount}
		}()
	}

	// close once everyone is done; must be its own goroutine
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
