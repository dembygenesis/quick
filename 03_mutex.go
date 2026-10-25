package main

import "sync"

// ---------------------------------------------------------------------------
// LESSON 3: Mutex  ("only one at a time through this door")
//
// THE STORY
//   Lessons 1-2 dodged the problem by never sharing. Now 1000 goroutines all
//   add $1 to the SAME wallet. `balance += 1` looks like one step but it's
//   three: READ the number, ADD one, WRITE it back.
//
//     goroutine A reads 100
//     goroutine B reads 100      <- B read before A wrote!
//     goroutine A writes 101
//     goroutine B writes 101     <- should be 102. One deposit vanished.
//
//   That is a "data race" (or "race condition"). Money disappears.
//
// THE FIX: a Mutex (short for "mutual exclusion")
//   Think of a bathroom with a lock on the door.
//     mu.Lock()     take the key. If someone has it, wait outside.
//     mu.Unlock()   give the key back.
//   Only one goroutine is inside at a time, so READ-ADD-WRITE can't be
//   interleaved. `defer mu.Unlock()` means "give the key back when this
//   function ends", like `finally`, so you can't forget on an early return.
//
// THE SECOND TRAP: check, then change (see Withdraw)
//   "If balance >= 10, then subtract 10". If the check and the subtract are
//   locked SEPARATELY, two goroutines can both pass the check (balance 10),
//   then both subtract, and you're at -10. The lock has to cover BOTH steps
//   together. Rule: lock around the whole "look then change" unit.
//   (In Postgres your CHECK (balance >= 0) is the safety net at the DB level.)
//
// DOES JS HAVE THIS?
//   Plain Node: rarely, because one thread can't be interrupted mid-line.
//   But the same bug hides around `await`:
//       const bal = await getBalance();   // read
//       await save(bal - 10);             // write. Someone else may have changed it between!
//   People fix it with a lock library (e.g. `async-mutex`) or DB row locks.
//
// FINDING THESE BUGS
//   Run tests with:  go test -race
//   It watches memory and shouts if two goroutines hit the same variable
//   without a lock. Try deleting a Lock() below and watch it complain.
//
// WORDS YOU'LL HEAR
//   critical section   the few lines between Lock() and Unlock(). Keep it short.
//   thread-safe        safe to call from many goroutines at once.
//
// SAY THIS IN THE INTERVIEW
//   "A mutex makes sure only one goroutine touches shared data at a time. The
//   lock has to wrap the whole check-and-update, not just one half. I use
//   `go test -race` to catch mistakes."
// ---------------------------------------------------------------------------

type SafeWallet struct {
	mu      sync.Mutex // the door lock. Protects `balance` below.
	balance int64
}

func (s *SafeWallet) Deposit(n int64) {
	s.mu.Lock()
	defer s.mu.Unlock() // give the key back when we leave
	s.balance += n
}

func (s *SafeWallet) Withdraw(n int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// The CHECK and the SUBTRACT both happen while we hold the key.
	if s.balance < n {
		return ErrInsufficientFunds
	}
	s.balance -= n
	return nil
}

func (s *SafeWallet) Balance() int64 {
	s.mu.Lock() // even reading needs the key, or you might read mid-write
	defer s.mu.Unlock()
	return s.balance
}

// Step3Deposit: `goroutines` people each deposit `each` into ONE wallet at once.
func Step3Deposit(goroutines int, each int64) int64 {
	var w SafeWallet

	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.Deposit(each)
		}()
	}
	wg.Wait()

	return w.Balance()
}

// Step3Withdraw: `attempts` goroutines all try to withdraw `amount` from a
// wallet holding `start`. Returns how many got money and what's left.
func Step3Withdraw(start int64, attempts int, amount int64) (succeeded int, balance int64) {
	w := SafeWallet{balance: start}

	var (
		wg sync.WaitGroup
		mu sync.Mutex // a second lock, just for the `succeeded` counter (many goroutines add to it)
	)
	for range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if w.Withdraw(amount) == nil {
				mu.Lock()
				succeeded++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	return succeeded, w.Balance()
}
