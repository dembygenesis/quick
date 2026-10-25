package main

import (
	"maps"
	"testing"
	"time"
)

func TestStep2TopUpAll(t *testing.T) {
	tests := []struct {
		name    string
		wallets []Wallet
		amount  int64
		want    map[int64]int64
	}{
		{
			name:    "no wallets: channel is closed straight away, range ends",
			wallets: nil,
			amount:  50,
			want:    map[int64]int64{},
		},
		{
			name:    "every wallet reports back exactly once",
			wallets: []Wallet{{ID: 1, Balance: 0}, {ID: 2, Balance: 10}, {ID: 3, Balance: 99}},
			amount:  50,
			want:    map[int64]int64{1: 50, 2: 60, 3: 149},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got map[int64]int64
			mustFinish(t, 2*time.Second, func() { got = Step2TopUpAll(tt.wallets, tt.amount) })

			if !maps.Equal(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}
