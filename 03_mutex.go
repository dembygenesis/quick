package main

import "sync"

// ---------------------------------------------------------------------------
// LESSON 3: Mutex. One goroutine at a time through the door.
//
// balance += 1 is read, add, write. Two goroutines can read the same old value
// and one deposit vanishes. Lock() / Unlock() prevents that.
//
// Lock the whole "check then change", not each half (see Withdraw).
// JS has the same bug around await: read balance, await, write balance.
// Try deleting a Lock and run: go test -race
// ---------------------------------------------------------------------------

type SafeWallet struct {
	mu      sync.Mutex // guards balance
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

// Step3Deposit: many goroutines deposit into ONE wallet.
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

// Step3Withdraw: many goroutines race to withdraw. Returns how many won.
func Step3Withdraw(start int64, attempts int, amount int64) (succeeded int, balance int64) {
	w := SafeWallet{balance: start}

	var (
		wg sync.WaitGroup
		mu sync.Mutex // guards succeeded
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
