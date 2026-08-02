// command_test.go contains table-driven tests for command-string matching used
// by command-based enforcement rules.
package engine

import "testing"

func TestMatchCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
		command string
		want    bool
		wantErr bool
	}{
		{name: "empty pattern matches any command", pattern: "", command: "git push", want: true},
		{name: "star matches any command", pattern: "*", command: "git push", want: true},
		{name: "doublestar matches any command", pattern: "**", command: "aws s3 ls", want: true},
		{name: "empty command never matches a real pattern", pattern: "git", command: "", want: false},
		{name: "whitespace-only command never matches", pattern: "git", command: "   ", want: false},

		{name: "literal single word matches leading token", pattern: "git", command: "git commit -m wip", want: true},
		{name: "literal single word no match", pattern: "aws", command: "git status", want: false},
		{name: "literal does not match substring of another word", pattern: "git", command: "gitleaks detect", want: false},

		{name: "multi-word pattern matches token prefix", pattern: "git push", command: "git push --force", want: true},
		{name: "multi-word pattern mismatched subcommand", pattern: "git push", command: "git commit", want: false},
		{name: "multi-word pattern longer than command", pattern: "git push origin main extra", command: "git push", want: false},

		{name: "glob suffix matches", pattern: "terraform*", command: "terraform apply", want: true},
		{name: "glob matches whole segment", pattern: "aws s3*", command: "aws s3 cp a b", want: true},
		{name: "glob no match", pattern: "kubectl*", command: "docker ps", want: false},

		{name: "strips leading env assignment", pattern: "git", command: "GIT_TRACE=1 git push", want: true},
		{name: "strips multiple env assignments", pattern: "aws", command: "AWS_PROFILE=prod DEBUG=1 aws s3 ls", want: true},
		{name: "strips leading sudo", pattern: "docker", command: "sudo docker ps", want: true},
		{name: "strips sudo and env", pattern: "docker", command: "sudo FOO=bar docker ps", want: true},
		{name: "non-identifier equals is not env assignment", pattern: "./x=y", command: "./x=y run", want: true},

		{name: "command of only env assignments matches nothing", pattern: "git", command: "FOO=bar BAZ=qux", want: false},
		{name: "command of only sudo matches nothing", pattern: "git", command: "sudo", want: false},

		{name: "matches segment after &&", pattern: "git", command: "npm run build && git commit -am ci", want: true},
		{name: "matches segment after pipe", pattern: "grep", command: "cat f | grep foo", want: true},
		{name: "matches segment after semicolon", pattern: "aws", command: "cd /tmp ; aws s3 ls", want: true},
		{name: "no false match across segments", pattern: "helm", command: "git add . && git commit", want: false},

		{name: "malformed glob returns error", pattern: "[", command: "git push", want: false, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := MatchCommand(tt.pattern, tt.command)
			if (err != nil) != tt.wantErr {
				t.Fatalf("MatchCommand(%q, %q) error = %v, wantErr %v", tt.pattern, tt.command, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("MatchCommand(%q, %q) = %v, want %v", tt.pattern, tt.command, got, tt.want)
			}
		})
	}
}
