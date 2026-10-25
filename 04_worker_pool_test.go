package main

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestStep4WorkerPool(t *testing.T) {
	tests := []struct {
		name    string
		workers int
		jobs    int
	}{
		{name: "one worker is fully serial", workers: 1, jobs: 10},
		{name: "4 workers, 20 jobs", workers: 4, jobs: 20},
		{name: "more workers than jobs", workers: 10, jobs: 3},
		{name: "no jobs: workers still exit cleanly", workers: 3, jobs: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jobs := make([]TopUpJob, tt.jobs)
			var wantSum int64
			for i := range jobs {
				jobs[i] = TopUpJob{WalletID: int64(i), Amount: int64(i + 1)}
				wantSum += jobs[i].Amount
			}

			var inFlight, peak atomic.Int32
			var gotSum, gotCount atomic.Int64
			apply := func(j TopUpJob) {
				cur := inFlight.Add(1)
				for { // record the highest concurrency we ever saw
					p := peak.Load()
					if cur <= p || peak.CompareAndSwap(p, cur) {
						break
					}
				}
				time.Sleep(5 * time.Millisecond) // hold the slot so workers overlap
				inFlight.Add(-1)

				gotSum.Add(j.Amount)
				gotCount.Add(1)
			}

			mustFinish(t, 5*time.Second, func() { Step4WorkerPool(tt.workers, jobs, apply) })

			if int(gotCount.Load()) != tt.jobs {
				t.Fatalf("ran %d jobs, want %d (each exactly once)", gotCount.Load(), tt.jobs)
			}
			if gotSum.Load() != wantSum {
				t.Fatalf("sum of amounts %d, want %d", gotSum.Load(), wantSum)
			}
			if int(peak.Load()) > tt.workers {
				t.Fatalf("peak concurrency %d exceeded %d workers", peak.Load(), tt.workers)
			}
		})
	}
}
