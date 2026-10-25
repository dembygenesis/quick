package main

import "sync"

// ---------------------------------------------------------------------------
// LESSON 5: transactions, commit and rollback  ("all or nothing")
//
// THE STORY
//   Move $30 from wallet 1 to wallet 2. Two steps: take $30 from 1, give $30
//   to 2. If the program dies BETWEEN the steps, $30 vanished into thin air.
//   A transaction groups the steps so they either BOTH happen or NEITHER does.
//
//   COMMIT   = "yes, make all of it real"
//   ROLLBACK = "undo everything, pretend it never happened"
//
// YOU'VE SEEN THIS IN NODE (node-postgres)
//   const client = await pool.connect();
//   try {
//     await client.query('BEGIN');
//     await client.query('UPDATE wallets SET balance = balance - 30 WHERE id = 1');
//     await client.query('UPDATE wallets SET balance = balance + 30 WHERE id = 2');
//     await client.query('COMMIT');
//   } catch (e) {
//     await client.query('ROLLBACK');
//     throw e;
//   } finally {
//     client.release();
//   }
//
// THE GO VERSION (see topUp in main.go, it does exactly this with real SQL)
//   tx, err := db.BeginTx(...)
//   defer tx.Rollback()      <- the safety net. Runs when the function ends.
//   ...do the work...
//   return tx.Commit()
//
//   "But we rollback even on success?" Yes, and it's harmless: after a
//   successful Commit, Rollback does nothing. So you write it ONCE at the top
//   and every early `return` is automatically safe. No try/catch needed.
//
// NO EXCEPTIONS IN GO
//   Go doesn't throw. Functions RETURN an error as a normal value, and you
//   check it: `if err != nil { return err }`. That's why you see it everywhere.
//   Think of it as: every function returns { result, error } and you must look.
//
// WHAT THIS CODE DOES
//   Our fake "database" is a map of wallet id -> balance.
//   Tx doesn't touch the map when you call Add(). It just writes down the
//   changes (like a shopping cart). Commit() then:
//     pass 1: checks every change is allowed (wallet exists, balance won't go
//             negative). Changes nothing yet.
//     pass 2: applies them all. It can't fail now, so nothing is half-done.
//   Rollback() just throws the shopping cart away.
//
// WORDS YOU'LL HEAR
//   atomic   "all or nothing, indivisible". The A in ACID.
//   delta    a change amount: -30 for "take 30", +30 for "give 30".
//
// SAY THIS IN THE INTERVIEW
//   "Begin, defer Rollback, do the work, Commit. If anything fails along the
//   way, the deferred Rollback undoes everything. After a successful Commit
//   the Rollback is a no-op."
// ---------------------------------------------------------------------------

type Bank struct {
	mu       sync.Mutex // one lock for the whole bank (lesson 6 makes this smarter)
	balances map[int64]int64
}

func NewBank(balances map[int64]int64) *Bank {
	b := &Bank{balances: make(map[int64]int64, len(balances))}
	for id, bal := range balances {
		b.balances[id] = bal // copy, so the caller's map isn't shared with us
	}
	return b
}

func (b *Bank) Balance(id int64) (int64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	bal, ok := b.balances[id]
	if !ok {
		return 0, ErrWalletNotFound
	}
	return bal, nil
}

// Tx is the "shopping cart" of pending changes.
type Tx struct {
	bank   *Bank
	deltas map[int64]int64 // wallet id -> pending change
	done   bool            // true once committed or rolled back
}

func (b *Bank) Begin() *Tx {
	return &Tx{bank: b, deltas: map[int64]int64{}}
}

// Add writes down a change. Nothing is applied yet.
func (t *Tx) Add(walletID, delta int64) {
	t.deltas[walletID] += delta
}

func (t *Tx) Commit() error {
	if t.done {
		return ErrTxDone // can't commit twice
	}
	t.done = true

	b := t.bank
	b.mu.Lock()
	defer b.mu.Unlock()

	// Pass 1: check everything. Change nothing.
	for id, d := range t.deltas {
		bal, ok := b.balances[id]
		if !ok {
			return ErrWalletNotFound
		}
		if bal+d < 0 {
			return ErrInsufficientFunds
		}
	}

	// Pass 2: apply everything. Safe, because pass 1 already said yes.
	for id, d := range t.deltas {
		b.balances[id] += d
	}
	return nil
}

func (t *Tx) Rollback() {
	t.done = true
	t.deltas = nil // throw the cart away
}

// Step5Transfer is the "move money" recipe: begin, defer rollback, work, commit.
func Step5Transfer(b *Bank, from, to, amount int64) error {
	if amount <= 0 {
		return ErrInvalidAmount
	}

	tx := b.Begin()
	defer tx.Rollback() // the safety net, covers every return below

	tx.Add(from, -amount)
	tx.Add(to, amount)

	return tx.Commit()
}
