package controller

import (
	"errors"
	"testing"

	aero "github.com/aerospike/aerospike-client-go/v8"
)

func TestRetryOnTransient_Success(t *testing.T) {
	calls := 0
	err := retryOnTransient(func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestRetryOnTransient_PermanentError(t *testing.T) {
	calls := 0
	permanent := errors.New("invalid privilege")
	err := retryOnTransient(func() error {
		calls++
		return permanent
	})
	if err != permanent {
		t.Fatalf("expected permanent error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call (no retry for permanent error), got %d", calls)
	}
}

func TestRetryOnTransient_TransientThenSuccess(t *testing.T) {
	calls := 0
	err := retryOnTransient(func() error {
		calls++
		if calls == 1 {
			return errors.New("connection reset by peer")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil error after retry, got %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
}

func TestRetryOnTransient_TransientThenTransient(t *testing.T) {
	calls := 0
	err := retryOnTransient(func() error {
		calls++
		return errors.New("connection reset by peer")
	})
	if err == nil {
		t.Fatal("expected error after both calls fail")
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
}

func TestRetryOnTransient_TransientThenPermanent(t *testing.T) {
	calls := 0
	err := retryOnTransient(func() error {
		calls++
		if calls == 1 {
			return errors.New("read tcp 10.0.0.1:3000: timeout")
		}
		return errors.New("invalid privilege")
	})
	if err == nil {
		t.Fatal("expected error after retry returns permanent error")
	}
	if err.Error() != "invalid privilege" {
		t.Fatalf("expected permanent error from retry, got %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
}

// --- retryOnTransientWithReconnect tests ---
//
// These use distinct non-nil *aero.Client values purely as identity tokens;
// the callbacks never dereference them, so no real connection is needed.

func TestRetryOnTransientWithReconnect_Success(t *testing.T) {
	initial := &aero.Client{}
	fnCalls := 0
	getClientCalls := 0
	err := retryOnTransientWithReconnect(
		func(forRetry bool) (*aero.Client, error) {
			getClientCalls++
			if forRetry {
				t.Errorf("getClient should not be called for retry on success")
			}
			return initial, nil
		},
		func(c *aero.Client) error {
			fnCalls++
			if c != initial {
				t.Errorf("expected initial client on first call")
			}
			return nil
		})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if fnCalls != 1 {
		t.Fatalf("expected 1 fn call, got %d", fnCalls)
	}
	if getClientCalls != 1 {
		t.Fatalf("expected 1 getClient call on success, got %d", getClientCalls)
	}
}

func TestRetryOnTransientWithReconnect_FirstGetClientFails(t *testing.T) {
	fnCalls := 0
	connErr := errors.New("connecting to cluster failed")
	err := retryOnTransientWithReconnect(
		func(forRetry bool) (*aero.Client, error) { return nil, connErr },
		func(c *aero.Client) error { fnCalls++; return nil })
	if err != connErr {
		t.Fatalf("expected connection error, got %v", err)
	}
	if fnCalls != 0 {
		t.Fatalf("expected 0 fn calls when getClient fails, got %d", fnCalls)
	}
}

func TestRetryOnTransientWithReconnect_PermanentErrorNoRetry(t *testing.T) {
	initial := &aero.Client{}
	fnCalls := 0
	getClientCalls := 0
	permanent := errors.New("invalid privilege")
	err := retryOnTransientWithReconnect(
		func(forRetry bool) (*aero.Client, error) { getClientCalls++; return initial, nil },
		func(c *aero.Client) error { fnCalls++; return permanent })
	if err != permanent {
		t.Fatalf("expected permanent error, got %v", err)
	}
	if fnCalls != 1 {
		t.Fatalf("expected 1 fn call (no retry), got %d", fnCalls)
	}
	if getClientCalls != 1 {
		t.Fatalf("expected 1 getClient call (no reconnect) for permanent error, got %d", getClientCalls)
	}
}

// TestRetryOnTransientWithReconnect_RetryUsesFreshClient is the core regression
// test: on a transient error the retry MUST receive a freshly-acquired client,
// not the stale one. Before the fix the retry reused the stale client and would
// fail identically.
func TestRetryOnTransientWithReconnect_RetryUsesFreshClient(t *testing.T) {
	initial := &aero.Client{}
	fresh := &aero.Client{}
	fnCalls := 0
	getClientCalls := 0
	err := retryOnTransientWithReconnect(
		func(forRetry bool) (*aero.Client, error) {
			getClientCalls++
			if forRetry {
				return fresh, nil
			}
			return initial, nil
		},
		func(c *aero.Client) error {
			fnCalls++
			if fnCalls == 1 {
				if c != initial {
					t.Errorf("first call: expected initial client")
				}
				return errors.New("connection reset by peer")
			}
			// Second (retry) call must use the fresh client.
			if c != fresh {
				t.Errorf("retry call: expected fresh client, got stale client")
			}
			return nil
		})
	if err != nil {
		t.Fatalf("expected nil error after retry with fresh client, got %v", err)
	}
	if fnCalls != 2 {
		t.Fatalf("expected 2 fn calls, got %d", fnCalls)
	}
	if getClientCalls != 2 {
		t.Fatalf("expected 2 getClient calls (initial + reconnect), got %d", getClientCalls)
	}
}

func TestRetryOnTransientWithReconnect_ReconnectFailsReturnsOriginalError(t *testing.T) {
	initial := &aero.Client{}
	fnCalls := 0
	transient := errors.New("i/o timeout")
	err := retryOnTransientWithReconnect(
		func(forRetry bool) (*aero.Client, error) {
			if forRetry {
				return nil, errors.New("dns lookup failed")
			}
			return initial, nil
		},
		func(c *aero.Client) error { fnCalls++; return transient })
	if err != transient {
		t.Fatalf("expected original transient error when reconnect fails, got %v", err)
	}
	if fnCalls != 1 {
		t.Fatalf("expected 1 fn call (retry skipped because reconnect failed), got %d", fnCalls)
	}
}
