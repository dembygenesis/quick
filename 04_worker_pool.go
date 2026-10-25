package main

import "sync"

// ---------------------------------------------------------------------------
// LESSON 4: worker pool  ("only N workers, however many jobs")
//
// THE STORY
//   Starting one goroutine per job is great for 10 jobs. For 10 million jobs,
//   or when your database only allows 10 connections, it's a disaster.
//   Fix: hire a FIXED team of workers. Jobs wait in a line; each worker takes
//   the next job when free. Like a restaurant with 4 cooks and 100 orders.
//
// YOU'VE SEEN THIS IN JS
//   const jobs = Array.from({ length: 100000 }, ...);
//   await Promise.all(jobs.map(j => fetch(j)));   // 100k requests at ONCE. Boom.
//   The fix is a limiter:
//   const limit = pLimit(4);                       // npm: p-limit
//   await Promise.all(jobs.map(j => limit(() => fetch(j))));  // max 4 at a time
//   Same idea here, built from raw parts.
//
// HOW IT WORKS
//   1. Make a channel called `queue` (the line of jobs).
//   2. Start `workers` goroutines. Each runs `for job := range queue`, which
//      means "take the next job, do it, repeat".
//   3. The main code puts every job into the queue.
//   4. close(queue) = "no more jobs". Each worker's loop finishes when the
//      queue is empty AND closed, so the workers go home.
//   5. wg.Wait() waits for all workers to go home.
//
// REAL-LIFE LINK
//   Go's database/sql does this for you: db.SetMaxOpenConns(10) means "at
//   most 10 workers talking to Postgres at once"; extra requests wait.
//
// SAY THIS IN THE INTERVIEW
//   "I start a fixed number of workers that read jobs from a channel. That
//   bounds concurrency so I don't overwhelm the DB or run out of memory.
//   Closing the channel tells the workers there's no more work."
// ---------------------------------------------------------------------------

type TopUpJob struct {
	WalletID int64
	Amount   int64
}

// apply is the work to do for one job. Passing it in lets the tests spy on it.
func Step4WorkerPool(workers int, jobs []TopUpJob, apply func(TopUpJob)) {
	queue := make(chan TopUpJob)

	// Hire the team.
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range queue { // keep taking jobs until the queue is closed and empty
				apply(job)
			}
		}()
	}

	// Hand out the work. Each send waits until some worker is free to take it.
	for _, j := range jobs {
		queue <- j
	}
	close(queue) // "that's all the jobs": lets the workers' loops end
	wg.Wait()    // wait for the team to finish
}
