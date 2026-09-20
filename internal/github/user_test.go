package github

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetUser(t *testing.T) {
	for _, tt := range []struct {
		name    string
		status  int
		body    string
		wantErr bool
	}{
		{"valid", 200, `{"id":123,"login":"alice","name":"Alice"}`, false},
		{"unauthorized", 401, "secret response", true},
		{"forbidden", 403, "secret response", true},
		{"server failure", 500, "secret response", true},
		{"invalid JSON", 200, "invalid", true},
		{"missing ID", 200, `{}`, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer test-token" || r.Method != http.MethodGet {
					t.Error("missing authentication or wrong method")
				}
				w.WriteHeader(tt.status)
				w.Write([]byte(tt.body))
			}))
			defer server.Close()
			user, err := getUser(context.Background(), server.Client(), server.URL, "test-token")
			if (err != nil) != tt.wantErr {
				t.Fatalf("user=%+v err=%v", user, err)
			}
			if err != nil && strings.Contains(err.Error(), "secret response") {
				t.Fatal("error exposes response body")
			}
			if tt.status == 401 && !errors.Is(err, ErrUnauthorized) {
				t.Fatal("expected unauthorized error")
			}
			if !tt.wantErr && (user.ID != 123 || user.Login != "alice") {
				t.Fatalf("unexpected user: %+v", user)
			}
		})
	}
}
