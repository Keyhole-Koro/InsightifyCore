package ui

import (
	"testing"

	actdomain "insightify/internal/domain/act"
)

func TestIsCreateNodeActorAllowed(t *testing.T) {
	cases := []struct {
		actor string
		ok    bool
	}{
		{actor: "act", ok: true},
		{actor: "worker", ok: true},
		{actor: "system", ok: true},
		{actor: "user", ok: false},
		{actor: "", ok: false},
	}
	for _, tc := range cases {
		if got := actdomain.IsNodeCreateActorAllowed(tc.actor); got != tc.ok {
			t.Fatalf("actor=%q: got %v want %v", tc.actor, got, tc.ok)
		}
	}
}
