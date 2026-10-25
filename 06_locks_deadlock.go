package main

import "sync"

// ---------------------------------------------------------------------------
// LESSON 6: one lock per wallet, and the DEADLOCK trap
//
// THE STORY
//   Lesson 5 used ONE lock for the whole bank. Safe, but if Alice pays Bob,
//   Carol has to wait in line even though she's not involved. Better: one
//   lock PER wallet, so unrelated transfers run at the same time.
//   (Postgres does this with row locks: SELECT ... FOR UPDATE.)
//
// THE NEW DANGER: a transfer needs TWO locks (sender's and receiver's)
//
//   Alice -> Bob: grabs Alice's lock, then waits for Bob's lock.
//   Bob -> Alice: grabs Bob's lock,   then waits for Alice's lock.
//
//   Each is holding the key the other one needs. Both wait forever.
//   That's a DEADLOCK. Two people each holding one chopstick, each waiting
//   for the other's. Nobody moves, no error, the program just hangs.
//
// THE FIX: everyone grabs locks in the SAME ORDER
//   Always lock the LOWER wallet id first, no matter who's sending.
//   Now both Alice->Bob and Bob->Alice try Alice's lock first. One wins, the
//   other waits, and nobody holds a lock the other needs. No circle, no hang.
//   (In SQL: ... WHERE id IN ($1,$2) ORDER BY id FOR UPDATE)
//
// JS HAS THIS TOO, JUST RARELY
//   await lockA.acquire(); await lockB.acquire();   // request 1
//   await lockB.acquire(); await lockA.acquire();   // request 2: waits forever
//   Same cure: agree on one order. Think of `locks` as a Map<walletId, Mutex>.
//
// ANOTHER TRAP HANDLED BELOW
//   from == to would lock the same lock twice and freeze on yourself. So we
//   reject it up front.
//
// WORDS YOU'LL HEAR
//   deadlock       everyone's waiting on someone else; nothing ever moves
//   lock ordering  the "always lowest id first" trick
//
// SAY THIS IN THE INTERVIEW
//   "Deadlock happens when goroutines wait on each other in a circle. I break
//   the circle by always acquiring locks in a consistent order, like sorting
//   by id."
// ---------------------------------------------------------------------------

type lockedWallet struct {
	mu      sync.Mutex // this wallet's own lock
	balance int64
}

type LockedBank struct {
	// Built once at the start and never added to later, so reading the map
	// itself needs no lock. Only each wallet's balance needs its lock.
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
		return ErrSameWallet // locking the same lock twice would freeze us
	}
	src, ok := lb.wallets[from]
	dst, ok2 := lb.wallets[to]
	if !ok || !ok2 {
		return ErrWalletNotFound
	}

	// THE TRICK: lock the lower id first, whichever direction the money goes.
	first, second := src, dst
	if from > to {
		first, second = dst, src
	}
	first.mu.Lock()
	defer first.mu.Unlock()
	second.mu.Lock()
	defer second.mu.Unlock()

	// Both wallets are ours now. Check and move, safely.
	if src.balance < amount {
		return ErrInsufficientFunds
	}
	src.balance -= amount
	dst.balance += amount
	return nil
}
