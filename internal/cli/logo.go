// logo.go handles the angry-bear logo display.
// The logo is embedded in the binary so it works on every machine.
package cli

import (
	_ "embed"
	"encoding/base64"
	"fmt"
	"os"
)

//go:embed logo.png
var logoPNG []byte

// printLogo displays the angry-bear logo before the project picker.
// Uses iTerm2/Kitty inline image protocol for supported terminals,
// falls back to gradient text for basic terminals.
func printLogo() {
	h := "\033[38;5;204m" // pink
	g := "\033[38;5;245m" // gray
	r := "\033[0m"        // reset

	if printInlineImage() {
		fmt.Fprintln(os.Stderr)
		return
	}

	// Fallback: gradient text
	printGradientLogo()
	fmt.Fprintf(os.Stderr, "\n    %s\u2665%s Skill enforcement for AI coding agents\n", h, r)
	fmt.Fprintf(os.Stderr, "    %s%s%s\n\n", g, version, r)
}

// printInlineImage displays the embedded logo using iTerm2 inline image protocol.
func printInlineImage() bool {
	term := os.Getenv("TERM_PROGRAM")
	if term != "iTerm.app" && term != "WezTerm" && term != "WarpTerminal" {
		lc := os.Getenv("LC_TERMINAL")
		if lc != "iTerm2" {
			return false
		}
	}

	encoded := base64.StdEncoding.EncodeToString(logoPNG)
	fmt.Fprintf(os.Stderr, "\033]1337;File=inline=1;width=50;preserveAspectRatio=1:%s\a\n", encoded)
	return true
}

// printGradientLogo prints gradient text "angry-bear" for basic terminals.
func printGradientLogo() {
	colors := []int{69, 69, 75, 75, 81, 81, 117, 153, 177, 204}
	text := " angry-bear"
	fmt.Fprint(os.Stderr, "\n    ")
	for i, ch := range text {
		ci := i
		if ci >= len(colors) {
			ci = len(colors) - 1
		}
		fmt.Fprintf(os.Stderr, "\033[1;38;5;%dm%c", colors[ci], ch)
	}
	fmt.Fprint(os.Stderr, "\033[0m\n")
}
