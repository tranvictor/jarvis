package common

import (
	"errors"
	"sync"
)

// RunParallel takes multiple functions that each return an error,
// runs them in parallel using goroutines, then aggregates any
// errors using errors.Join (Go 1.20+).
func RunParallel(funcs ...func() error) (error, int) {
	var wg sync.WaitGroup
	errs := make(chan error, len(funcs)) // buffered channel

	for _, fn := range funcs {
		wg.Add(1)
		go func(fn func() error) {
			defer wg.Done()
			err := fn()
			if err != nil {
				errs <- err
			}
		}(fn)
	}

	// Wait for all goroutines to finish
	wg.Wait()
	close(errs)

	// Collect all errors
	var allErrs []error
	for err := range errs {
		allErrs = append(allErrs, err)
	}

	// If there are no errors, errors.Join returns nil
	return errors.Join(allErrs...), len(allErrs)
}
