package main

import "sync"

// ---------------------------------------------------------------------------
// Step 3: Mutex
//
// Steps 1-2 avoided sharing. Now we share on purpose: many goroutines hit
// ONE wallet. `balance += n` is read, add, write: two goroutines can read the
// same old value and one update is lost. A Mutex lets one goroutine in at a
// time. `go test -race` flags the bug if you delete the Lock.
//
// The lock must cover check AND act. Withdraw checks the balance and subtracts
// under one Lock; locking each half separately would let two withdrawals both
// pass the check, and the wallet goes negative. (In Postgres the CHECK
// (balance >= 0) constraint is the last line of defence.)
//
// JS ANALOGY
//   Node has no Mutex in everyday code BECAUSE it is single-threaded: a
//   synchronous `balance += n` can never be interrupted.
//   But the SAME bug shows up in JS around an `await`:
//       const bal = await getBalance();      // read
//       await save(bal - 10);                // write: someone else may have written between
//   That is check-then-act, and the fix is the same idea: a lock
//   (npm `async-mutex`, or a promise chain used as a queue).
//   defer mu.Unlock()  ~ try { ... } finally { release() }
//
// INTERVIEW LINE: "A mutex protects shared state. The critical section must
// include the check AND the write, otherwise two goroutines both pass the
// check. I find these with `go test -race`."
// ---------------------------------------------------------------------------

type SafeWallet struct {
	mu      sync.Mutex
	balance int64
}

func (s *SafeWallet) Deposit(n int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.balance += n
}

func (s *SafeWallet) Withdraw(n int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.balance < n {
		return ErrInsufficientFunds
	}
	s.balance -= n
	return nil
}

func (s *SafeWallet) Balance() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.balance
}

// Step3Deposit: `goroutines` depositors, each depositing `each`.
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

// Step3Withdraw: `attempts` goroutines race to withdraw `amount` from a wallet
// holding `start`. Returns how many succeeded and what is left.
func Step3Withdraw(start int64, attempts int, amount int64) (succeeded int, balance int64) {
	w := SafeWallet{balance: start}

	var (
		wg sync.WaitGroup
		mu sync.Mutex // guards `succeeded`, which many goroutines bump
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
