package ingest

import "testing"

func TestPseudonymizeAuthor(t *testing.T) {
	tests := []struct {
		name     string
		salt     string
		username string
	}{
		{name: "simple username", salt: "salt1", username: "octocat"},
		{name: "empty username", salt: "salt1", username: ""},
		{name: "unicode username", salt: "salt1", username: "üser-ñame"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PseudonymizeAuthor(tt.salt, tt.username)

			if got == tt.username {
				t.Fatalf("PseudonymizeAuthor() returned the raw username unchanged")
			}
			if len(got) != 64 {
				t.Fatalf("PseudonymizeAuthor() = %q, want 64 hex chars (sha256)", got)
			}

			again := PseudonymizeAuthor(tt.salt, tt.username)
			if got != again {
				t.Fatalf("PseudonymizeAuthor() is not deterministic: %q != %q", got, again)
			}
		})
	}
}

func TestPseudonymizeAuthorDifferentSaltsDiffer(t *testing.T) {
	a := PseudonymizeAuthor("salt-a", "octocat")
	b := PseudonymizeAuthor("salt-b", "octocat")

	if a == b {
		t.Fatal("PseudonymizeAuthor() with different salts produced the same hash")
	}
}

func TestPseudonymizeAuthorDifferentUsersDiffer(t *testing.T) {
	a := PseudonymizeAuthor("salt", "octocat")
	b := PseudonymizeAuthor("salt", "hubot")

	if a == b {
		t.Fatal("PseudonymizeAuthor() with different usernames produced the same hash")
	}
}
