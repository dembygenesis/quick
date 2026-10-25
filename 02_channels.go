package main

import "sync"

// ---------------------------------------------------------------------------
// LESSON 2: channels  ("a pipe goroutines use to hand each other stuff")
//
// THE STORY
//   In lesson 1 every goroutine wrote into a slot of a shared slice. That
//   works, but it's fragile. A nicer way: each goroutine SENDS its answer
//   into a pipe, and the caller RECEIVES answers as they arrive.
//
// THE JS PICTURE (no perfect match, this is the closest)
//   Imagine an async queue:
//     ch <- value          like  queue.push(value)   (but it waits for a taker!)
//     v := <-ch            like  const v = await queue.next()
//     for r := range ch    like  for await (const r of stream)
//     close(ch)            like  the stream ending ("no more data")
//
//   A Promise gives you ONE value, once. A channel can carry MANY values.
//
// HOW THIS FUNCTION WORKS (3 moves)
//   1. Start one goroutine per wallet. Each sends its result into `results`.
//   2. Start one extra helper goroutine that waits for them ALL to finish
//      (wg.Wait) and then closes the channel = "that's everyone, no more".
//   3. The main code loops `for r := range results` and keeps reading until
//      the channel is closed.
//   If you forget close(), the loop waits forever for more data. That's a
//   classic hang. (Like a for-await over a stream that never ends.)
//
// WHY NO LOCK FOR THE MAP
//   Only the main code ever writes to `out`. The goroutines just send. One
//   writer = no fight.
//
// WORDS YOU'LL HEAR
//   unbuffered channel   make(chan T). A send WAITS until someone receives.
//                        Like handing a coffee to someone: both must be there.
//   buffered channel     make(chan T, 10). Holds up to 10 items before a send
//                        has to wait. Like a mailbox.
//   blocked              "stuck waiting". Not a crash, just paused.
//   deadlock             everything is blocked waiting on everything else.
//                        Go notices and crashes with "all goroutines are
//                        asleep - deadlock!". Better than silently hanging.
//
// RULES TO MEMORISE
//   - The SENDER closes the channel, never the receiver.
//   - Sending on a closed channel crashes (panic).
//   - Reading from a closed channel gives what's left, then zero values.
//
// SAY THIS IN THE INTERVIEW
//   "Channels let goroutines pass data safely instead of sharing variables.
//   The goroutine that produces the data closes the channel when it's done."
// ---------------------------------------------------------------------------

type TopUpResult struct {
	WalletID int64
	Balance  int64
}

func Step2TopUpAll(wallets []Wallet, amount int64) map[int64]int64 {
	results := make(chan TopUpResult) // the pipe. Unbuffered: a send waits for a receiver.

	// Move 1: one goroutine per wallet, each sends its answer into the pipe.
	var wg sync.WaitGroup
	for _, w := range wallets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- TopUpResult{WalletID: w.ID, Balance: w.Balance + amount}
		}()
	}

	// Move 2: when everyone is done, close the pipe so the loop below can end.
	// This must be its own goroutine, because the main code below is busy reading.
	go func() {
		wg.Wait()
		close(results)
	}()

	// Move 3: read answers as they arrive, until the pipe is closed.
	out := make(map[int64]int64, len(wallets))
	for r := range results {
		out[r.WalletID] = r.Balance
	}

	return out
}
