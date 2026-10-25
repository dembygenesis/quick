package main

import "testing"

func newActorBank(t *testing.T, b map[int64]int64) Transferer {
	a := NewActorBank(b)
	t.Cleanup(a.Close)
	return a
}

func TestStep7Transfer(t *testing.T) {
	runTransferCases(t, newActorBank)
}

func TestStep7Transfer_ConcurrentRing(t *testing.T) {
	runConcurrentRing(t, newActorBank)
}
