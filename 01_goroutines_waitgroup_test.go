package main

import (
	"slices"
	"testing"
)

func TestStep1TopUpAll(t *testing.T) {
	tests := []struct {
		name    string
		wallets []Wallet
		amount  int64
		want    []Wallet
	}{
		{
			name:    "no wallets",
			wallets: []Wallet{},
			amount:  50,
			want:    []Wallet{},
		},
		{
			name:    "order is preserved even though goroutines finish in any order",
			wallets: []Wallet{{ID: 1, Owner: "a", Balance: 0}, {ID: 2, Owner: "b", Balance: 10}, {ID: 3, Owner: "c", Balance: 99}},
			amount:  50,
			want:    []Wallet{{ID: 1, Owner: "a", Balance: 50}, {ID: 2, Owner: "b", Balance: 60}, {ID: 3, Owner: "c", Balance: 149}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Step1TopUpAll(tt.wallets, tt.amount)

			if !slices.Equal(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}
