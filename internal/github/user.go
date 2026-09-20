// Package github resolves the identity authenticated by a GitHub access token.
package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

var ErrUnauthorized = errors.New("GitHub access token is invalid")

type User struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
	Name  string `json:"name"`
}

// GetUser fetches the token owner's identity, rather than trusting a supplied ID.
func GetUser(ctx context.Context, accessToken string) (User, error) {
	return getUser(ctx, http.DefaultClient, "https://api.github.com/user", accessToken)
}

func getUser(ctx context.Context, client *http.Client, endpoint, accessToken string) (User, error) {
	if accessToken == "" {
		return User{}, ErrUnauthorized
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return User{}, fmt.Errorf("create GitHub user request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "frnkstn")
	resp, err := client.Do(req)
	if err != nil {
		return User{}, fmt.Errorf("fetch GitHub user: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return User{}, ErrUnauthorized
	}
	if resp.StatusCode != http.StatusOK {
		return User{}, fmt.Errorf("GitHub user request failed (HTTP %d)", resp.StatusCode)
	}
	var user User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return User{}, fmt.Errorf("decode GitHub user: %w", err)
	}
	if user.ID <= 0 {
		return User{}, errors.New("GitHub response contains no valid user ID")
	}
	return user, nil
}
