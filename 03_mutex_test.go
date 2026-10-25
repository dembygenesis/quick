package main

import (
	"testing"
)

func TestStep3Deposit(t *testing.T) {
	tests := []struct {
		name       string
		goroutines int
		each       int64
		want       int64
	}{
		{name: "one goroutine", goroutines: 1, each: 5, want: 5},
		{name: "1000 goroutines, no lost updates", goroutines: 1000, each: 1, want: 1000},
		{name: "big amounts", goroutines: 200, each: 250, want: 50_000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Step3Deposit(tt.goroutines, tt.each)

			if got != tt.want {
				t.Fatalf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestStep3Withdraw(t *testing.T) {
	tests := []struct {
		name          string
		start         int64
		attempts      int
		amount        int64
		wantSucceeded int
		wantBalance   int64
	}{
		{name: "enough for everyone", start: 100, attempts: 10, amount: 5, wantSucceeded: 10, wantBalance: 50},
		{name: "exactly drained", start: 100, attempts: 100, amount: 1, wantSucceeded: 100, wantBalance: 0},
		{name: "200 race for 100: exactly 100 win, never negative", start: 100, attempts: 200, amount: 1, wantSucceeded: 100, wantBalance: 0},
		{name: "nothing to withdraw", start: 0, attempts: 50, amount: 1, wantSucceeded: 0, wantBalance: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSucceeded, gotBalance := Step3Withdraw(tt.start, tt.attempts, tt.amount)

			if gotSucceeded != tt.wantSucceeded || gotBalance != tt.wantBalance {
				t.Fatalf("got (%d, %d), want (%d, %d)", gotSucceeded, gotBalance, tt.wantSucceeded, tt.wantBalance)
			}
		})
	}
}
