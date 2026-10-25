package main

import "sync"

// ---------------------------------------------------------------------------
// LESSON 5: transactions. All or nothing.
//
// JS (node-postgres):  BEGIN ... COMMIT, ROLLBACK in catch, release in finally
// Go:  tx := Begin()
//      defer tx.Rollback()   // no-op after a successful Commit
//      ...
//      return tx.Commit()
//
// Same shape as topUp in main.go.
// Go has no exceptions: functions return err and you check it.
//
// Here Add() only notes the change. Commit() checks all, then applies all.
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

	// pass 1: validate
	for id, d := range t.deltas {
		bal, ok := b.balances[id]
		if !ok {
			return ErrWalletNotFound
		}
		if bal+d < 0 {
			return ErrInsufficientFunds
		}
	}

	// pass 2: apply
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
