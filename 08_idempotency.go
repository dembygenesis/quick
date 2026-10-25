package main

import "sync"

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
//
// JS ANALOGY
//   Request de-duplication with a Map of in-flight promises:
//       const inflight = new Map();
//       function transfer(key, ...args) {
//         if (inflight.has(key)) return inflight.get(key);      // wait on same promise
//         const p = doTransfer(...args).catch(e => { inflight.delete(key); throw e; });
//         inflight.set(key, p);
//         return p;
//       }
//   close(c.done) ~ resolving that promise: EVERY awaiter wakes up at once.
//   Idempotency-Key header ~ what Stripe's API does for safe retries.
//   JS needs no mutex around the Map (single thread); Go does, because two
//   goroutines could both see "key missing" at the same moment.
//
// INTERVIEW LINE: "Retries must be safe. First caller does the work, duplicates
// wait for its result. A failure forgets the key so a retry can try again."
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
