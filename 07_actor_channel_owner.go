package main

// ---------------------------------------------------------------------------
// LESSON 7: one goroutine owns the data. No locks.
//
// JS:  this is Node's event loop. One thread owns state, tasks queue up.
// Go:  callers send a func down `ops`; the owner goroutine runs them one by one.
//
// Mutex for a simple counter. Owner goroutine for bigger logic.
// Call Close() when done, or the owner goroutine leaks.
// ---------------------------------------------------------------------------

type ActorBank struct {
	ops      chan func(map[int64]int64)
	finished chan struct{}
}

func NewActorBank(balances map[int64]int64) *ActorBank {
	owned := make(map[int64]int64, len(balances)) // only the owner goroutine touches this
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

// do sends a task to the owner and waits for it.
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
	// runs inside the owner goroutine: no lock needed
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
