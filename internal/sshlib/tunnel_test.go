package sshlib

import (
	"testing"
)

func TestTunnelRuleNormalize(t *testing.T) {
	tests := []struct {
		input  TunnelRule
		expTyp string
		expLoc string
	}{
		{
			input:  TunnelRule{Type: "-L", Local: "5900", Remote: "10.20.13.115:5900"},
			expTyp: "-l",
			expLoc: "127.0.0.1:5900",
		},
		{
			input:  TunnelRule{Type: "dynamic", Local: "1080"},
			expTyp: "dynamic",
			expLoc: "127.0.0.1:1080",
		},
		{
			input:  TunnelRule{Type: "remote", Local: "0.0.0.0:8080", Remote: "localhost:8080"},
			expTyp: "remote",
			expLoc: "0.0.0.0:8080",
		},
	}

	for _, tc := range tests {
		tc.input.Normalize()
		if tc.input.Type != tc.expTyp {
			t.Errorf("expected type %s, got %s", tc.expTyp, tc.input.Type)
		}
		if tc.input.Local != tc.expLoc {
			t.Errorf("expected local %s, got %s", tc.expLoc, tc.input.Local)
		}
	}
}
