package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

var loginURL = "https://github.com/login/device/code"
var authURL = "https://github.com/login/oauth/access_token"
var grantType = "urn:ietf:params:oauth:grant-type:device_code"

var clientID string

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

func authenticate() (PollResponse, error) {
	err := godotenv.Load("../.env")
	if err != nil {
		log.Fatal(err)
	}
	clientID = os.Getenv("GITHUB_CLIENT_ID")

	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("scope", "read:user")

	req, err := http.NewRequest(
		http.MethodPost,
		loginURL,
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return PollResponse{}, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("Error sending auth request: %v", err)
		return PollResponse{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		log.Printf("Unexpected status code: %d", res.StatusCode)
		body, _ := io.ReadAll(res.Body)
		return PollResponse{}, fmt.Errorf("unexpected status code: %d: %s", res.StatusCode, body)
	}

	var ghRes GHResponse
	if err = json.NewDecoder(res.Body).Decode(&ghRes); err != nil {
		log.Printf("Error decoding auth response: %v", err)
		return PollResponse{}, err
	}

	return pollForToken(
		clientID,
		ghRes.DeviceCode,
		time.Duration(ghRes.Interval),
		ghRes.ExpiresIn,
	)
}

func pollForToken(clientID string, deviceCode string, interval time.Duration, expiresIn int) (PollResponse, error) {
	expiresAt := time.Now().Add(time.Duration(expiresIn))

	for time.Now().Before(expiresAt) {
		time.Sleep(interval)

		form := url.Values{}
		form.Set("client_id", clientID)
		form.Set("device_code", deviceCode)
		form.Set("grant_type", grantType)

		req, err := http.NewRequest(
			http.MethodPost,
			authURL,
			strings.NewReader(form.Encode()),
		)
		if err != nil {
			log.Printf("Error marshaling poll request: %v", err)
			return PollResponse{}, err
		}

		req.Header.Set("Accept", "application/json")
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Printf("Error sending poll request: %v", err)
			return PollResponse{}, err
		}
		defer res.Body.Close()

		var pollRes PollResponse
		err = json.NewDecoder(res.Body).Decode(&pollRes)
		if err != nil {
			log.Printf("Error decoding poll response: %v", err)
			return PollResponse{}, err
		}
		if res.StatusCode != http.StatusOK {
			log.Printf("Unexpected status code: %d", res.StatusCode)
			return PollResponse{}, err
		}

		switch pollRes.Error {
		case "":
			if pollRes.AccessToken != "" {
				return pollRes, nil
			}

		case "authorization_pending":
			continue

		case "slow_down":
			interval += 5
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
