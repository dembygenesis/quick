package main

import "sync"

// ---------------------------------------------------------------------------
// LESSON 8: idempotency keys  ("safe to retry, money moves ONCE")
//
// THE STORY
//   You tap "Pay $10". The network hiccups. Did it go through? You don't
//   know, so the app sends the same request again. Without protection you've
//   paid $20. Nightmare.
//
//   Fix: every payment request carries a unique ID, the "idempotency key".
//   If the server sees the same key twice, it does NOT do the work twice; it
//   just returns the first result. (Stripe's API works exactly like this.)
//   "Idempotent" = doing it twice has the same effect as doing it once.
//   In our database this is the column:  idempotency_key TEXT UNIQUE
//
// THE HARD PART: both retries arrive at the SAME MOMENT
//   Two copies of the same request hit the server together. Both look and see
//   "no one has done this key yet". Both go ahead. Double payment!
//   So we need a lock (lesson 3) on the "who's doing what" notebook, plus a
//   way for the second request to wait for the first one's answer (lesson 2).
//
// IN JS YOU'D DO IT WITH A MAP OF PROMISES
//   const inflight = new Map();
//   function transfer(key, ...args) {
//     if (inflight.has(key)) return inflight.get(key);   // join the first request
//     const p = doTransfer(...args).catch(e => { inflight.delete(key); throw e; });
//     inflight.set(key, p);
//     return p;
//   }
//   JS needs no lock around that Map (one thread, nothing can interrupt).
//   Go does, because two goroutines could both see "key missing" together.
//
// HOW THE GO VERSION WORKS
//   `calls` is the notebook: key -> a "call" record.
//   1. Lock the notebook. Is this key already in it?
//   2. YES: unlock, then wait on that record's `done` channel. When the first
//      request finishes it close()s `done`, which wakes EVERYONE waiting at
//      once (like a promise resolving for many .then()s). Return its result.
//   3. NO: write our record in the notebook, unlock, do the transfer, store
//      the result, close(done) so waiters wake up.
//   4. If the transfer FAILED, erase the key from the notebook. A failed
//      payment shouldn't block the user from trying again. (Same as a SQL
//      transaction rolling back: the unique key insert is undone too.)
//
// RULE WORTH REMEMBERING
//   Never wait on a channel while holding a lock. Notice we Unlock() BEFORE
//   `<-c.done`. Otherwise the first request can't get the lock to finish, and
//   everyone is stuck. (Hello again, deadlock.)
//
// WORDS YOU'LL HEAR
//   idempotent        same result no matter how many times you do it
//   in-flight         started but not finished yet
//   dedupe            remove duplicates
//
// SAY THIS IN THE INTERVIEW
//   "Clients retry, so writes must be safe to repeat. I require an idempotency
//   key. The first request does the work; concurrent duplicates wait and share
//   its result. If it fails I forget the key so a retry can try again."
// ---------------------------------------------------------------------------

type call struct {
	done chan struct{} // closed when the first request is finished
	err  error         // its result (nil = success)
}

type IdempotentBank struct {
	bank  Transferer
	mu    sync.Mutex       // protects `calls`
	calls map[string]*call // the notebook: key -> in-flight or finished request
}

func NewIdempotentBank(bank Transferer) *IdempotentBank {
	return &IdempotentBank{bank: bank, calls: map[string]*call{}}
}

func (ib *IdempotentBank) Transfer(key string, from, to, amount int64) error {
	ib.mu.Lock()
	if c, ok := ib.calls[key]; ok {
		// Someone already started this key. Don't redo it; wait for their answer.
		ib.mu.Unlock() // IMPORTANT: unlock before waiting
		<-c.done
		return c.err
	}
	// First time we've seen this key: write it in the notebook.
	c := &call{done: make(chan struct{})}
	ib.calls[key] = c
	ib.mu.Unlock()

	c.err = ib.bank.Transfer(from, to, amount) // the real work, done once

	if c.err != nil {
		ib.mu.Lock()
		delete(ib.calls, key) // failed: allow a retry with the same key
		ib.mu.Unlock()
	}
	close(c.done) // wake up everyone waiting on us
	return c.err
}
