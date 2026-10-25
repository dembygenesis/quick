package main

import "sync"

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
//
// JS ANALOGY
//   A channel has no exact JS twin. Closest: an async queue you `await` on.
//     ch <- v      ~ queue.push(v), but it BLOCKS until someone is receiving
//     v := <-ch    ~ const v = await queue.next()
//     for r := range ch  ~ for await (const r of asyncGenerator())
//     close(ch)    ~ the generator returning / a stream ending
//   A Promise resolves ONCE with ONE value. A channel carries MANY values.
//   Unbuffered channel = a handshake: sender and receiver must meet.
//   make(chan T, 10) = buffered: sender only blocks when 10 are waiting.
//
// INTERVIEW LINE: "Channels let goroutines hand data to each other safely.
// The sender closes, never the receiver. Sending on a closed channel panics;
// receiving from one just drains what is left then returns the zero value."
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
