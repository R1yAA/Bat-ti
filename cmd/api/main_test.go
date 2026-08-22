package main

import "testing"

// The AUTH_DISABLED escape hatch is only safe because it cannot reach a real
// database. These cases are the whole of that guarantee.
func TestIsLocalDatabase(t *testing.T) {
	testCases := []struct {
		name        string
		databaseURL string
		isLocal     bool
	}{
		{"the docker compose database", "postgres://batti:pw@localhost:5433/batti?sslmode=disable", true},
		{"the same by address", "postgres://batti:pw@127.0.0.1:5433/batti?sslmode=disable", true},
		{"a supabase pooler", "postgres://postgres.abc:pw@aws-0-ap-southeast-2.pooler.supabase.com:6543/postgres", false},
		{"a supabase direct connection", "postgres://postgres:pw@db.abc.supabase.co:5432/postgres", false},
		{"no url at all", "", false},
		// A hostname that merely mentions localhost is not localhost. Matching
		// on the credential separator is what keeps this from being fooled.
		{"a remote host named after localhost", "postgres://u:pw@localhost.evil.com:5432/postgres", false},
		// The reverse: localhost in the password must not make a remote host
		// look local.
		{"localhost inside the password", "postgres://u:localhost@db.abc.supabase.co:5432/postgres", false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := isLocalDatabase(testCase.databaseURL); got != testCase.isLocal {
				t.Errorf("isLocalDatabase(%q) = %v, want %v",
					testCase.databaseURL, got, testCase.isLocal)
			}
		})
	}
}
