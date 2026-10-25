package main

import (
	"errors"
	"testing"
)

func TestStep5Transfer(t *testing.T) {
	tests := []struct {
		name    string
		from    int64
		to      int64
		amount  int64
		wantErr error
		want    map[int64]int64
	}{
		{
			name: "commit: both sides move", from: 1, to: 2, amount: 30,
			want: map[int64]int64{1: 70, 2: 30},
		},
		{
			name: "rollback: insufficient funds leaves BOTH untouched", from: 1, to: 2, amount: 101,
			wantErr: ErrInsufficientFunds,
			want:    map[int64]int64{1: 100, 2: 0},
		},
		{
			name: "rollback: unknown destination must not debit the source", from: 1, to: 99, amount: 10,
			wantErr: ErrWalletNotFound,
			want:    map[int64]int64{1: 100, 2: 0},
		},
		{
			name: "rollback: invalid amount", from: 1, to: 2, amount: 0,
			wantErr: ErrInvalidAmount,
			want:    map[int64]int64{1: 100, 2: 0},
		},
		{
			name: "transfer to self nets zero", from: 1, to: 1, amount: 10,
			want: map[int64]int64{1: 100, 2: 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bank := NewBank(map[int64]int64{1: 100, 2: 0})

			err := Step5Transfer(bank, tt.from, tt.to, tt.amount)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			for id, want := range tt.want {
				if got, _ := bank.Balance(id); got != want {
					t.Fatalf("wallet %d = %d, want %d", id, got, want)
				}
			}
		})
	}
}

func TestStep5Tx_DoneIsFinal(t *testing.T) {
	tests := []struct {
		name string
		run  func(tx *Tx) error
		want error
	}{
		{name: "commit twice", run: func(tx *Tx) error { _ = tx.Commit(); return tx.Commit() }, want: ErrTxDone},
		{name: "commit after rollback", run: func(tx *Tx) error { tx.Rollback(); return tx.Commit() }, want: ErrTxDone},
		{name: "rollback after commit is a harmless no-op (defer-safe)", run: func(tx *Tx) error { err := tx.Commit(); tx.Rollback(); return err }, want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := NewBank(map[int64]int64{1: 10}).Begin()

			if err := tt.run(tx); !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}
}
