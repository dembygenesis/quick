package main

import "sync"

// ---------------------------------------------------------------------------
// LESSON 4: worker pool. N workers, any number of jobs.
//
// JS:  pLimit(4) from p-limit. Without it, jobs.map(fetch) fires everything at once.
// Go:  N goroutines all read from one jobs channel. close(queue) = no more jobs.
//
// database/sql does this for you: db.SetMaxOpenConns(n).
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
			for job := range queue {
				apply(job)
			}
		}()
	}

	for _, j := range jobs {
		queue <- j
	}
	close(queue)
	wg.Wait()
}
