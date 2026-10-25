package main

// ---------------------------------------------------------------------------
// LESSON 7: one goroutine owns the data  ("no locks, just a queue")
//
// THE STORY
//   Lesson 6 used locks and we had to be careful (ordering, double-locking).
//   Different idea: don't let anyone else touch the data at all. ONE goroutine
//   owns the wallets. Everyone else sends it a message: "please do this for
//   me". It handles messages one at a time. No two things ever touch the data
//   together, so no locks needed.
//
//   Go has a saying for this:
//   "Don't communicate by sharing memory; share memory by communicating."
//   In plain words: instead of everyone grabbing the same variable, pass
//   messages to the one who owns it.
//
// YOU ALREADY KNOW THIS: IT'S NODE'S EVENT LOOP
//   Node has ONE thread that owns your state. Everything else (requests,
//   timers) queues up and gets run one at a time. That's why Node code rarely
//   needs locks. This lesson rebuilds that on purpose for one piece of data,
//   while the rest of your Go program stays parallel.
//
//        Node                         This lesson
//        -------------------------    -------------------------
//        task queue                   the `ops` channel
//        the single JS thread         the one owner goroutine (the loop in NewActorBank)
//        await result                 <-done (wait for the owner to finish our task)
//
// HOW IT WORKS
//   - The `owned` map lives inside ONE goroutine. Nobody else has a reference.
//   - To do anything, you send a FUNCTION down `ops` (a "run this for me").
//   - The owner runs each function in order, passing it the map.
//   - do() sends the function and then waits until it has run, like `await`.
//
// WHEN TO USE WHICH (lesson 6 vs 7)
//   Mutex:         simple things, like guarding one counter
//   Owner/channel: bigger logic where many steps must happen together
//   Both are fine. Interviewers like hearing you can explain the tradeoff.
//
// HEADS UP
//   Call Close() when finished, or the owner goroutine sticks around forever
//   (a "goroutine leak"). Using the bank after Close() crashes on purpose:
//   that's a bug in the caller, and a loud crash beats a silent hang.
//
// WORDS YOU'LL HEAR
//   actor / owner goroutine   the one goroutine in charge of the data
//   goroutine leak            a goroutine nobody ever stops; wastes memory
//
// SAY THIS IN THE INTERVIEW
//   "Instead of locking shared data, I give it to one goroutine and send it
//   requests over a channel. Requests run one at a time, so there are no
//   races. It's like Node's event loop for one piece of state."
// ---------------------------------------------------------------------------

type ActorBank struct {
	ops      chan func(map[int64]int64) // the inbox: "run this function on the data"
	finished chan struct{}              // closed when the owner goroutine has exited
}

func NewActorBank(balances map[int64]int64) *ActorBank {
	owned := make(map[int64]int64, len(balances)) // ONLY the owner goroutine below touches this
	for id, bal := range balances {
		owned[id] = bal
	}

	a := &ActorBank{
		ops:      make(chan func(map[int64]int64)),
		finished: make(chan struct{}),
	}

	// The owner: reads tasks from the inbox, one at a time, until it's closed.
	go func() {
		defer close(a.finished)
		for op := range a.ops {
			op(owned)
		}
	}()
	return a
}

// do sends a task to the owner and waits until it has run (like await).
func (a *ActorBank) do(op func(map[int64]int64)) {
	done := make(chan struct{})
	a.ops <- func(m map[int64]int64) {
		op(m)
		close(done) // tell the waiting caller "finished"
	}
	<-done
}

// Close shuts the owner down and waits for it to exit.
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
	// This function body runs INSIDE the owner goroutine, alone with the data.
	// No lock needed, nobody else can be in here.
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
