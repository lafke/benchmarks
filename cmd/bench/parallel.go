package main

import (
	"fmt"
	"strings"
	"sync"
)

// parallel runs f for every index in [0, n) on at most workers goroutines and returns the
// errors of the calls that failed, joined.
func parallel(n, workers int, f func(i int) error) error {
	if workers < 1 {
		workers = 1
	}
	idx := make(chan int)
	var mu sync.Mutex
	var errs []string
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range idx {
				if err := f(i); err != nil {
					mu.Lock()
					errs = append(errs, err.Error())
					mu.Unlock()
				}
			}
		}()
	}
	for i := 0; i < n; i++ {
		idx <- i
	}
	close(idx)
	wg.Wait()
	if len(errs) > 0 {
		return fmt.Errorf("%d of %d failed:\n  %s", len(errs), n, strings.Join(errs, "\n  "))
	}
	return nil
}
