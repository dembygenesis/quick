package main

// ---------------------------------------------------------------------------
// Step 7: channel as owner (actor)
//
// Go proverb: "don't communicate by sharing memory; share memory by
// communicating". Instead of N goroutines fighting over locks, ONE goroutine
// owns the map and everyone else sends it requests over a channel. Requests
// are handled one at a time, so no mutex exists anywhere in this type.
//
// A request is just a func to run inside the owner goroutine; do() blocks
// until it has run.
//
// Call Close() when done. Using the bank after Close panics (send on closed
// channel), which is a feature: it is a bug in the caller.
//
// JS ANALOGY
//   You already know this pattern: it IS Node's event loop.
//   One thread owns all state, everyone else enqueues work, tasks run one at a
//   time, so no locks are needed.
//       ops channel     ~ the task queue (setImmediate / queueMicrotask)
//       owner goroutine ~ the event loop thread
//       reply wait      ~ await the result
//   Go lets you build that on purpose for one piece of state, while the rest of
//   the program stays parallel. Pick this when logic is complex; pick a mutex
//   when you just need to guard a counter.
//
// INTERVIEW LINE: "Share memory by communicating: one goroutine owns the data,
// callers send it requests over a channel."
// ---------------------------------------------------------------------------

type ActorBank struct {
	ops      chan func(map[int64]int64)
	finished chan struct{}
}

func NewActorBank(balances map[int64]int64) *ActorBank {
	owned := make(map[int64]int64, len(balances)) // ONLY the loop goroutine touches this
	for id, bal := range balances {
		owned[id] = bal
	}

	a := &ActorBank{
		ops:      make(chan func(map[int64]int64)),
		finished: make(chan struct{}),
	}
	go func() {
		defer close(a.finished)
		for op := range a.ops {
			op(owned)
		}
	}()
	return a
}

func (a *ActorBank) do(op func(map[int64]int64)) {
	done := make(chan struct{})
	a.ops <- func(m map[int64]int64) {
		op(m)
		close(done)
	}
	<-done
}

func (a *ActorBank) Close() {
	close(a.ops)
	<-a.finished
}

func (a *ActorBank) Balance(id int64) (bal int64, err error) {
	a.do(func(m map[int64]int64) {
		var ok bool
		if bal, ok = m[id]; !ok {
			err = ErrWalletNotFound
		}
	})
	return bal, err
}

func (a *ActorBank) Transfer(from, to, amount int64) (err error) {
	if amount <= 0 {
		return ErrInvalidAmount
	}
	if from == to {
		return ErrSameWallet
	}
	a.do(func(m map[int64]int64) {
		src, ok := m[from]
		_, ok2 := m[to]
		switch {
		case !ok || !ok2:
			err = ErrWalletNotFound
		case src < amount:
			err = ErrInsufficientFunds
		default:
			m[from] -= amount
			m[to] += amount
		}
	})
	return err
}
