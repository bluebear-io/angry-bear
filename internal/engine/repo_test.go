// repo_test.go tests Git repository identity resolution and URL normalization.
package engine

import "testing"

func TestNormalizeRemoteURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{"HTTPS", "https://github.com/bluebear-io/blueden.git", "bluebear-io/blueden"},
		{"HTTPS no .git", "https://github.com/bluebear-io/blueden", "bluebear-io/blueden"},
		{"SSH", "git@github.com:bluebear-io/blueden.git", "bluebear-io/blueden"},
		{"SSH no .git", "git@github.com:bluebear-io/blueden", "bluebear-io/blueden"},
		{"SSH custom host", "git@github.com-bluebear:bluebear-io/blueden.git", "bluebear-io/blueden"},
		{"HTTPS with token", "https://x-access-token:gho_abc123@github.com/bluebear-io/blueden.git", "bluebear-io/blueden"},
		{"SSH protocol", "ssh://git@github.com/bluebear-io/blueden.git", "bluebear-io/blueden"},
		{"Empty", "", ""},
		{"Random string", "not-a-url", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeRemoteURL(tt.url)
			if got != tt.expected {
				t.Errorf("NormalizeRemoteURL(%q) = %q, want %q", tt.url, got, tt.expected)
			}
		})
	}
}

func TestNormalizeRemoteURL_SameRepo(t *testing.T) {
	// All these URLs point to the same repo — they should normalize to the same slug
	urls := []string{
		"https://github.com/bluebear-io/blueden.git",
		"git@github.com:bluebear-io/blueden.git",
		"git@github.com-bluebear:bluebear-io/blueden.git",
		"https://x-access-token:gho_oMP5bmMB3Brko@github.com/bluebear-io/blueden.git",
		"ssh://git@github.com/bluebear-io/blueden.git",
	}

	expected := "bluebear-io/blueden"
	for _, url := range urls {
		got := NormalizeRemoteURL(url)
		if got != expected {
			t.Errorf("NormalizeRemoteURL(%q) = %q, want %q", url, got, expected)
		}
	}
}

func TestRepoConfigDir(t *testing.T) {
	repo := &RepoIdentity{
		Slug: "bluebear-io/blueden",
		Hash: "5ce4353d",
	}

	dir := RepoConfigDir("/home/user", repo)
	expected := "/home/user/.angry-bear/repos/5ce4353d-bluebear-io-blueden"
	if dir != expected {
		t.Errorf("RepoConfigDir = %q, want %q", dir, expected)
	}
}

func TestShortHash_Deterministic(t *testing.T) {
	h1 := ShortHash("bluebear-io/blueden")
	h2 := ShortHash("bluebear-io/blueden")
	if h1 != h2 {
		t.Errorf("ShortHash not deterministic: %q != %q", h1, h2)
	}
	if len(h1) != 8 {
		t.Errorf("ShortHash length = %d, want 8", len(h1))
	}
}

func TestShortHash_Different(t *testing.T) {
	h1 := ShortHash("bluebear-io/blueden")
	h2 := ShortHash("bluebear-io/baloo")
	if h1 == h2 {
		t.Errorf("different inputs produced same hash: %q", h1)
	}
}
