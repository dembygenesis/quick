package main

import "sync"

// ---------------------------------------------------------------------------
// LESSON 8: idempotency key. Retries must not pay twice.
//
// JS:  const inflight = new Map(); same key returns the same promise.
// Go:  mutex-guarded map of key -> call. Duplicates wait on call.done.
//      close(done) wakes every waiter at once.
//
// A failed transfer deletes its key so the client can retry.
// Unlock BEFORE waiting on a channel, never wait while holding the lock.
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
		ib.mu.Unlock() // unlock before waiting
		<-c.done
		return c.err
	}
	c := &call{done: make(chan struct{})}
	ib.calls[key] = c
	ib.mu.Unlock()

	c.err = ib.bank.Transfer(from, to, amount)

	if c.err != nil {
		ib.mu.Lock()
		delete(ib.calls, key) // failed: allow retry
		ib.mu.Unlock()
	}
	close(c.done) // wakes all waiters
	return c.err
}
