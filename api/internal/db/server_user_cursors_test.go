package db

import (
	"errors"
	"testing"
)

func TestInvitationClaimRetry(t *testing.T) {
	for _, test := range []struct {
		name        string
		storedCode  string
		requestCode string
		allowed     bool
	}{
		{"same invite can finish after a failed batch", "invite-one", "invite-one", true},
		{"second invite cannot duplicate membership", "invite-one", "invite-two", false},
		{"owner record cannot be claimed", "", "invite-one", false},
		{"empty invite rejected", "", "", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateInvitationClaim(ServerUserCursor{JoinCode: test.storedCode}, test.requestCode)
			if test.allowed && err != nil {
				t.Fatal(err)
			}
			if !test.allowed && !errors.Is(err, ErrAlreadyServerMember) {
				t.Fatalf("expected membership conflict, got %v", err)
			}
		})
	}
}
