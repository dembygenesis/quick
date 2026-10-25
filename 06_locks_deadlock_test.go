package main

import "testing"

func newLockedBank(t *testing.T, b map[int64]int64) Transferer { return NewLockedBank(b) }

func TestStep6Transfer(t *testing.T) {
	runTransferCases(t, newLockedBank)
}

func TestStep6Transfer_ConcurrentRing(t *testing.T) {
	runConcurrentRing(t, newLockedBank)
}
