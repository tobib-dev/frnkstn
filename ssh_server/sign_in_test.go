package main

import (
	"errors"
	"strings"
	"testing"
)

func TestSignInDisplaysDeviceInstructionsWhilePolling(t *testing.T) {
	m := newSignInModel(80, 24, "test-client", "7789")
	device := GHResponse{
		DeviceCode:      "private-device-code",
		UserCode:        "ABCD-EFGH",
		VerificationURI: "https://github.com/login/device",
		Interval:        5,
		ExpiresIn:       900,
	}
	m, cmd := m.Update(signInDeviceAuthMsg{response: device})
	if cmd == nil {
		t.Fatal("expected a polling command")
	}
	view := m.View().Content
	for _, instruction := range []string{device.VerificationURI, device.UserCode} {
		if !strings.Contains(view, instruction) {
			t.Errorf("view is missing %q", instruction)
		}
	}
	if strings.Contains(view, device.DeviceCode) {
		t.Error("view exposes the private device code")
	}

	m, _ = m.Update(signInFailureMsg{error: errors.New("access denied")})
	if m.deviceAuth != (GHResponse{}) {
		t.Error("failed sign-in retained device credentials")
	}
	if !strings.Contains(m.View().Content, "login failed") {
		t.Error("view is missing the sign-in failure message")
	}
	if strings.Contains(m.View().Content, "access denied") {
		t.Error("view exposes the login error")
	}
}
