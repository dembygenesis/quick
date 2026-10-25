package main

import "sync"

// ---------------------------------------------------------------------------
// Step 6: per-wallet locks + deadlock avoidance
//
// Step 5 used ONE lock for the whole bank: correct, but every transfer waits
// for every other. Real systems lock per row (Postgres: SELECT ... FOR UPDATE),
// so unrelated transfers run in parallel.
//
// New danger: a transfer needs TWO locks.
//
//	A -> B locks A, then wants B
//	B -> A locks B, then wants A    => each holds what the other needs. Deadlock.
//
// Fix: every transfer takes locks in the same global order (lowest id first).
// Then nobody can hold the higher lock while waiting on the lower one.
// In SQL this is: SELECT ... WHERE id IN ($1,$2) ORDER BY id FOR UPDATE.
//
// JS ANALOGY
//   Node can deadlock too, just rarely, when you use async locks:
//       await lockA.acquire(); await lockB.acquire();   // request 1
//       await lockB.acquire(); await lockA.acquire();   // request 2: waits forever
//   Same cure in both languages: always acquire locks in one agreed order.
//   Per-wallet locks ~ a Map<walletId, Mutex>, so transfers on different
//   wallets don't queue behind each other.
//
// INTERVIEW LINE: "Deadlock needs circular wait. I break the circle by
// ordering lock acquisition, here by wallet id. Same as ORDER BY id FOR UPDATE."
// ---------------------------------------------------------------------------

type lockedWallet struct {
	mu      sync.Mutex
	balance int64
}

type LockedBank struct {
	wallets map[int64]*lockedWallet // built once, never mutated: safe to read unlocked
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
		return ErrSameWallet // would Lock the same mutex twice: instant self-deadlock
	}
	src, ok := lb.wallets[from]
	dst, ok2 := lb.wallets[to]
	if !ok || !ok2 {
		return ErrWalletNotFound
	}

	first, second := src, dst
	if from > to {
		first, second = dst, src // lock the lower id first, whatever the direction
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
