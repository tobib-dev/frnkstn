package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var loginURL = "https://github.com/login/device/code"
var authURL = "https://github.com/login/oauth/access_token"
var grantType = "urn:ietf:params:oauth:grant-type:device_code"

const authRequestTimeout = 15 * time.Second

type GHAuth struct {
	ClientID string `json:"client_id"`
	Scope    string `json:"scope"`
}

type GHResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type PollRequest struct {
	ClientId   string `json:"client_id"`
	DeviceCode string `json:"device_code"`
	GrantType  string `json:"grant_type"`
}

type PollResponse struct {
	AccessToken      string `json:"access_token"`
	ExpiresIn        int    `json:"expires_in"`
	RefreshToken     string `json:"refresh_token"`
	RefreshExpiresIn int    `json:"refresh_expires_in"`
	TokenType        string `json:"token_type"`
	Scope            string `json:"scope"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

func authenticate(clientID string) (GHResponse, error) {
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("scope", "read:user")

	ctx, cancel := context.WithTimeout(context.Background(), authRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		loginURL,
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return GHResponse{}, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return GHResponse{}, fmt.Errorf("error sending auth request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		return GHResponse{}, fmt.Errorf("unexpected status code: %d: %s", res.StatusCode, body)
	}

	var ghRes GHResponse
	if err = json.NewDecoder(res.Body).Decode(&ghRes); err != nil {
		return GHResponse{}, fmt.Errorf("error decoding auth response: %w", err)
	}

	return ghRes, nil
}

func pollForToken(clientID string, deviceCode string, interval time.Duration, expiresIn int) (PollResponse, error) {
	expiresAt := time.Now().Add(time.Duration(expiresIn) * time.Second)
	ctx, cancel := context.WithDeadline(context.Background(), expiresAt)
	defer cancel()

	for time.Now().Before(expiresAt) {
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return PollResponse{}, fmt.Errorf("polling timed out")
		case <-timer.C:
		}

		form := url.Values{}
		form.Set("client_id", clientID)
		form.Set("device_code", deviceCode)
		form.Set("grant_type", grantType)

		requestCtx, cancelRequest := context.WithTimeout(ctx, authRequestTimeout)
		req, err := http.NewRequestWithContext(
			requestCtx,
			http.MethodPost,
			authURL,
			strings.NewReader(form.Encode()),
		)
		if err != nil {
			cancelRequest()
			return PollResponse{}, fmt.Errorf("error marshaling poll request: %w", err)
		}

		req.Header.Set("Accept", "application/json")
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			cancelRequest()
			return PollResponse{}, fmt.Errorf("error sending poll request: %w", err)
		}
		var pollRes PollResponse
		err = json.NewDecoder(res.Body).Decode(&pollRes)
		res.Body.Close()
		cancelRequest()
		if res.StatusCode != http.StatusOK {
			return PollResponse{}, fmt.Errorf("token request failed (HTTP %d): %s %s", res.StatusCode, pollRes.Error, pollRes.ErrorDescription)
		}
		if err != nil {
			return PollResponse{}, fmt.Errorf("error decoding poll response: %w", err)
		}

		switch pollRes.Error {
		case "":
			if pollRes.AccessToken != "" {
				return pollRes, nil
			}
			return PollResponse{}, fmt.Errorf("token response contains neither an access token nor an OAuth status")

		case "authorization_pending":
			continue

		case "slow_down":
			interval += 5 * time.Second
			continue

		case "access_denied":
			return PollResponse{}, fmt.Errorf("github authentication denied: %s", pollRes.ErrorDescription)

		case "expired_token":
			return PollResponse{}, fmt.Errorf("github device code expired")

		default:
			return PollResponse{}, fmt.Errorf("oauth error: %s", pollRes.Error)
		}
	}
	return PollResponse{}, fmt.Errorf("polling timed out")
}
