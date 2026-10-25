package main

import "sync"

// ---------------------------------------------------------------------------
// Step 5: Tx, commit and rollback
//
// A transaction is "stage changes, then apply ALL or NONE". Same contract as
// db.BeginTx in main.go topUp:
//
//	tx, _ := db.BeginTx(...)
//	defer tx.Rollback()   // safe no-op after Commit; saves us on every early return
//	... statements ...
//	return tx.Commit()
//
// Here Tx stages balance deltas. Commit validates every wallet, then applies
// everything under the bank lock. Rollback just throws the deltas away.
// (Postgres would fail at the offending statement; we fail at Commit. Same
// outcome: nothing half-applied.)
//
// JS ANALOGY
//   The same shape with node-postgres:
//       const client = await pool.connect();
//       try {
//         await client.query('BEGIN');
//         await client.query('UPDATE ...');
//         await client.query('COMMIT');
//       } catch (e) {
//         await client.query('ROLLBACK'); throw e;
//       } finally { client.release(); }
//   defer tx.Rollback() ~ the finally/catch. Go has no exceptions: errors are
//   plain return values, so every step is `if err != nil { return err }`
//   instead of a throw that unwinds for you.
//
// INTERVIEW LINE: "Begin, defer Rollback, do the work, Commit. Rollback after a
// successful Commit is a no-op, so the defer covers every early return."
// ---------------------------------------------------------------------------

type Bank struct {
	mu       sync.Mutex
	balances map[int64]int64
}

func NewBank(balances map[int64]int64) *Bank {
	b := &Bank{balances: make(map[int64]int64, len(balances))}
	for id, bal := range balances {
		b.balances[id] = bal
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

type Tx struct {
	bank   *Bank
	deltas map[int64]int64
	done   bool
}

func (b *Bank) Begin() *Tx {
	return &Tx{bank: b, deltas: map[int64]int64{}}
}

func (t *Tx) Add(walletID, delta int64) {
	t.deltas[walletID] += delta
}

func (t *Tx) Commit() error {
	if t.done {
		return ErrTxDone
	}
	t.done = true

	b := t.bank
	b.mu.Lock()
	defer b.mu.Unlock()

	// Pass 1: validate everything. Touch nothing yet.
	for id, d := range t.deltas {
		bal, ok := b.balances[id]
		if !ok {
			return ErrWalletNotFound
		}
		if bal+d < 0 {
			return ErrInsufficientFunds
		}
	}

	// Pass 2: apply. Cannot fail, so we never end up half-applied.
	for id, d := range t.deltas {
		b.balances[id] += d
	}
	return nil
}

func (t *Tx) Rollback() {
	t.done = true
	t.deltas = nil
}

func Step5Transfer(b *Bank, from, to, amount int64) error {
	if amount <= 0 {
		return ErrInvalidAmount
	}

	tx := b.Begin()
	defer tx.Rollback()

	tx.Add(from, -amount)
	tx.Add(to, amount)

	return tx.Commit()
}
