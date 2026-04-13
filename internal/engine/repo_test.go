// repo_test.go tests Git repository identity resolution and URL normalization.
package engine

import "testing"

func TestNormalizeRemoteURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{"HTTPS", "https://github.com/Blue-Bear-Security/blueden.git", "Blue-Bear-Security/blueden"},
		{"HTTPS no .git", "https://github.com/Blue-Bear-Security/blueden", "Blue-Bear-Security/blueden"},
		{"SSH", "git@github.com:Blue-Bear-Security/blueden.git", "Blue-Bear-Security/blueden"},
		{"SSH no .git", "git@github.com:Blue-Bear-Security/blueden", "Blue-Bear-Security/blueden"},
		{"SSH custom host", "git@github.com-bluebear:Blue-Bear-Security/blueden.git", "Blue-Bear-Security/blueden"},
		{"HTTPS with token", "https://x-access-token:gho_abc123@github.com/Blue-Bear-Security/blueden.git", "Blue-Bear-Security/blueden"},
		{"SSH protocol", "ssh://git@github.com/Blue-Bear-Security/blueden.git", "Blue-Bear-Security/blueden"},
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
		"https://github.com/Blue-Bear-Security/blueden.git",
		"git@github.com:Blue-Bear-Security/blueden.git",
		"git@github.com-bluebear:Blue-Bear-Security/blueden.git",
		"https://x-access-token:gho_oMP5bmMB3Brko@github.com/Blue-Bear-Security/blueden.git",
		"ssh://git@github.com/Blue-Bear-Security/blueden.git",
	}

	expected := "Blue-Bear-Security/blueden"
	for _, url := range urls {
		got := NormalizeRemoteURL(url)
		if got != expected {
			t.Errorf("NormalizeRemoteURL(%q) = %q, want %q", url, got, expected)
		}
	}
}

func TestRepoConfigDir(t *testing.T) {
	repo := &RepoIdentity{
		Slug: "Blue-Bear-Security/blueden",
		Hash: "5ce4353d",
	}

	dir := RepoConfigDir("/home/user", repo)
	expected := "/home/user/.care-bear/repos/5ce4353d-Blue-Bear-Security-blueden"
	if dir != expected {
		t.Errorf("RepoConfigDir = %q, want %q", dir, expected)
	}
}

func TestShortHash_Deterministic(t *testing.T) {
	h1 := ShortHash("Blue-Bear-Security/blueden")
	h2 := ShortHash("Blue-Bear-Security/blueden")
	if h1 != h2 {
		t.Errorf("ShortHash not deterministic: %q != %q", h1, h2)
	}
	if len(h1) != 8 {
		t.Errorf("ShortHash length = %d, want 8", len(h1))
	}
}

func TestShortHash_Different(t *testing.T) {
	h1 := ShortHash("Blue-Bear-Security/blueden")
	h2 := ShortHash("Blue-Bear-Security/baloo")
	if h1 == h2 {
		t.Errorf("different inputs produced same hash: %q", h1)
	}
}
