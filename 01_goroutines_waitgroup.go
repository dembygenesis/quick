package main

import "sync"

// ---------------------------------------------------------------------------
// LESSON 1: goroutines + WaitGroup  ("do many things at once, wait for all")
//
// THE STORY
//   You have 3 wallets and want to top up each one. You could loop through
//   them one by one. But what if each top-up took 1 second (a DB call)?
//   One by one = 3 seconds. All at once = 1 second. That is the whole point.
//
// IN JS YOU'D WRITE
//   const results = await Promise.all(
//     wallets.map(async (w) => ({ ...w, balance: w.balance + amount }))
//   );
//
// IN GO, THE SAME IDEA
//   go func() { ... }()   -> start this function in the background (a "goroutine")
//                            and move on immediately. Like calling an async
//                            function WITHOUT `await`.
//   sync.WaitGroup        -> a counter. Like Promise.all, but you do it by hand:
//        wg.Add(1)           "one more thing to wait for"   (new promise)
//        wg.Done()           "this one finished"            (promise resolved)
//        wg.Wait()           "block here until the counter is 0"  (await Promise.all)
//
// WHY WE NEED wg.Wait()
//   Without it, this function returns while the goroutines are still running.
//   You'd get back an empty slice. (Same bug as forgetting `await` in JS.)
//
// WHY NO LOCK HERE
//   Each goroutine writes only into its OWN box: out[i]. Nobody shares a box,
//   so nobody can step on anyone. Lessons 3+ cover what happens when they share.
//
// WORDS YOU'LL HEAR
//   goroutine    a function running in the background. Very cheap: you can
//                start thousands. (Not the same as an OS thread.)
//   concurrent   many things in progress at once
//   parallel     many things literally running at the same instant on
//                different CPU cores. Go does this. Node mostly doesn't.
//
// BIGGEST DIFFERENCE FROM NODE
//   Node runs your code on ONE thread, so two pieces of your JS never touch
//   the same variable at the same instant. Go really runs goroutines in
//   parallel, so two of them CAN touch the same variable at the same instant
//   and corrupt it. That bug is called a "data race". Lessons 3-8 are the cures.
//
// JS BUG YOU ALREADY KNOW
//   for (var i = 0; i < 3; i++) setTimeout(() => console.log(i))  // 3, 3, 3
//   Old Go had the same trap (closures shared one loop variable). Modern Go
//   gives every loop round its own copy, like `let`. So it's safe here.
//
// SAY THIS IN THE INTERVIEW
//   "A goroutine is a lightweight function that runs concurrently. I use a
//   WaitGroup to wait until a known number of them have finished."
// ---------------------------------------------------------------------------

func Step1TopUpAll(wallets []Wallet, amount int64) []Wallet {
	out := make([]Wallet, len(wallets))

	var wg sync.WaitGroup
	for i, w := range wallets {
		wg.Add(1) // count it BEFORE starting the goroutine, not inside it
		go func() {
			defer wg.Done()     // like `finally`: runs when this function ends, no matter what
			w.Balance += amount // w is this round's own copy of the wallet
			out[i] = w          // write into our own box i
		}()
	}
	wg.Wait() // pause here until every goroutine has called Done()

	return out
}
