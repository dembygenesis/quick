package main

import "sync"

// ---------------------------------------------------------------------------
// Step 4: worker pool
//
// One goroutine per job is fine for 10 jobs and a disaster for 10 million (or
// for a DB that allows 10 connections). A pool starts a FIXED number of
// workers that pull jobs off a channel until it is closed.
//
// This is what sql.DB does with connections: SetMaxOpenConns(n) is "n workers".
//
// JS ANALOGY
//   This is p-limit / p-queue / a "promise pool":
//       const limit = pLimit(4);
//       await Promise.all(jobs.map(j => limit(() => apply(j))));
//   Or Node's worker_threads pool (piscina). Same idea: N workers, a queue.
//   Why it matters in an interview: `jobs.map(j => fetch(j))` over 100k items
//   opens 100k sockets at once. Bounded concurrency is the fix, in both languages.
//
// INTERVIEW LINE: "I fan jobs into a channel and start a fixed number of
// workers that range over it. Closing the channel is the shutdown signal."
// ---------------------------------------------------------------------------

type TopUpJob struct {
	WalletID int64
	Amount   int64
}

func Step4WorkerPool(workers int, jobs []TopUpJob, apply func(TopUpJob)) {
	queue := make(chan TopUpJob)

	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range queue { // loop ends when queue is closed and drained
				apply(job)
			}
		}()
	}

	for _, j := range jobs {
		queue <- j
	}
	close(queue) // "no more jobs": lets the workers' range loops finish
	wg.Wait()
}
