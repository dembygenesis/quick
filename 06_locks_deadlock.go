package main

import "sync"

// ---------------------------------------------------------------------------
// LESSON 6: one lock per wallet, and deadlock.
//
// One lock for the whole bank (lesson 5) makes everyone queue.
// Per-wallet locks let unrelated transfers run together.
//
// Trap: a transfer needs two locks.
//   Alice->Bob locks Alice, waits for Bob.
//   Bob->Alice locks Bob, waits for Alice. Both wait forever.
// Fix: always lock the lower id first.
//
// SQL version: SELECT ... WHERE id IN ($1,$2) ORDER BY id FOR UPDATE
// ---------------------------------------------------------------------------

type lockedWallet struct {
	mu      sync.Mutex
	balance int64
}

type LockedBank struct {
	// never modified after creation, so reads need no lock
	wallets map[int64]*lockedWallet
}

func NewLockedBank(balances map[int64]int64) *LockedBank {
	lb := &LockedBank{wallets: make(map[int64]*lockedWallet, len(balances))}
	for id, bal := range balances {
		lb.wallets[id] = &lockedWallet{balance: bal}
	}
	return lb
}

func (lb *LockedBank) Balance(id int64) (int64, error) {
	w, ok := lb.wallets[id]
	if !ok {
		return 0, ErrWalletNotFound
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.balance, nil
}

func (lb *LockedBank) Transfer(from, to, amount int64) error {
	if amount <= 0 {
		return ErrInvalidAmount
	}
	if from == to {
		return ErrSameWallet // would lock itself twice
	}
	src, ok := lb.wallets[from]
	dst, ok2 := lb.wallets[to]
	if !ok || !ok2 {
		return ErrWalletNotFound
	}

	// lower id first, whichever direction the money goes
	first, second := src, dst
	if from > to {
		first, second = dst, src
	}
	first.mu.Lock()
	defer first.mu.Unlock()
	second.mu.Lock()
	defer second.mu.Unlock()

	if src.balance < amount {
		return ErrInsufficientFunds
	}
	src.balance -= amount
	dst.balance += amount
	return nil
}
