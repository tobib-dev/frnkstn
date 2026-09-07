package main

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type authTestTransport func(*http.Request) (*http.Response, error)

func TestPollForTokenBoundsStalledRequest(t *testing.T) {
	originalClient := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = originalClient })
	http.DefaultClient = &http.Client{Transport: authTestTransport(func(req *http.Request) (*http.Response, error) {
		deadline, ok := req.Context().Deadline()
		if !ok || time.Until(deadline) > time.Second {
			t.Fatal("request must be bounded by device code expiry")
		}
		<-req.Context().Done()
		return nil, req.Context().Err()
	})}
	_, err := pollForToken("test-client", "test-device", 0, 1)
	if err == nil {
		t.Fatal("expected stalled request to fail")
	}
}

func TestPollForTokenStopsOnFailure(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"denied", http.StatusOK, `{"error":"access_denied"}`},
		{"expired", http.StatusOK, `{"error":"expired_token"}`},
		{"http failure", http.StatusInternalServerError, `{}`},
		{"empty response", http.StatusOK, `{}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			originalClient := http.DefaultClient
			t.Cleanup(func() { http.DefaultClient = originalClient })
			calls := 0
			http.DefaultClient = &http.Client{Transport: authTestTransport(func(req *http.Request) (*http.Response, error) {
				calls++
				if calls > 1 {
					t.Fatal("polled again after a terminal failure")
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})}
			_, err := pollForToken("test-client", "test-device", 0, 1)
			if err == nil {
				t.Fatal("expected a sign-in error")
			}
			if calls != 1 || err.Error() == "polling timed out" {
				t.Fatalf("expected the first response to fail, got %d requests and %v", calls, err)
			}
		})
	}
}

func (f authTestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestPollForTokenExpiryUsesSeconds(t *testing.T) {
	originalClient := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = originalClient })
	calls := 0
	http.DefaultClient = &http.Client{Transport: authTestTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		body := `{"error":"authorization_pending"}`
		if calls == 2 {
			body = `{"access_token":"test-token"}`
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})}

	response, err := pollForToken("test-client", "test-device", 10*time.Millisecond, 1)
	if err != nil {
		t.Fatalf("polling expired before authorization completed: %v", err)
	}
	if response.AccessToken != "test-token" || calls != 2 {
		t.Fatalf("got token %q after %d requests; want test-token after 2", response.AccessToken, calls)
	}
}
