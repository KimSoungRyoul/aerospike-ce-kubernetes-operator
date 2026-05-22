package controller

import (
	aero "github.com/aerospike/aerospike-client-go/v8"
)

// retryOnTransient calls fn once. If it returns a transient Aerospike error
// (connection reset, timeout, etc.), fn is retried exactly once. On the second
// failure the retry error is returned. Non-transient errors are returned
// immediately without retry.
func retryOnTransient(fn func() error) error {
	err := fn()
	if err == nil {
		return nil
	}
	if !isTransientAeroError(err) {
		return err
	}
	return fn()
}

// retryOnTransientWithReconnect calls fn with an Aerospike client obtained from
// getClient. If fn returns a transient error (connection reset, timeout, etc.),
// a fresh client is obtained via getClient and fn is retried exactly once with
// it. Reusing a stale client whose connection is broken is pointless — the retry
// would fail identically — so the retry always runs against a freshly-acquired
// connection.
//
// getClient(forRetry) returns the client to use: forRetry is false on the first
// attempt and true when acquiring the client for the retry. The caller owns the
// lifecycle of every client getClient hands out (closing the first-attempt
// client and any retry client), which keeps connection cleanup in one place and
// avoids leaks. If getClient fails when acquiring the retry client, the original
// transient error is returned so the caller still sees a meaningful failure.
func retryOnTransientWithReconnect(
	getClient func(forRetry bool) (*aero.Client, error),
	fn func(*aero.Client) error,
) error {
	initial, err := getClient(false)
	if err != nil {
		return err
	}
	err = fn(initial)
	if err == nil {
		return nil
	}
	if !isTransientAeroError(err) {
		return err
	}

	// Transient failure: the connection is likely broken. Acquire a fresh
	// client before retrying so the retry has a real chance of succeeding.
	fresh, newErr := getClient(true)
	if newErr != nil {
		// Could not get a fresh connection; surface the original transient error.
		return err
	}
	return fn(fresh)
}
