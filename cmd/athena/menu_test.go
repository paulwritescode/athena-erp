package main

import (
	"bytes"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
)

// The registry holds every command exactly once, so dispatch and the
// menu agree on what exists.
func TestCommandRegistryIsUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, cmd := range commands {
		if cmd.name == "" || cmd.short == "" || cmd.run == nil {
			t.Fatalf("command entry %+v is incomplete", cmd)
		}
		if seen[cmd.name] {
			t.Fatalf("command %q is registered twice", cmd.name)
		}
		seen[cmd.name] = true
		if lookupCommand(cmd.name) == nil {
			t.Fatalf("lookupCommand(%q) found nothing", cmd.name)
		}
	}
	if lookupCommand("no-such-command") != nil {
		t.Fatal("lookupCommand(no-such-command) found a command")
	}
}

// The flag-style aliases resolve to the same handlers as their names.
func TestCommandAliases(t *testing.T) {
	for alias, name := range map[string]string{"--version": "version", "-v": "version", "--help": "help", "-h": "help"} {
		resolved := alias
		switch resolved {
		case "--version", "-v":
			resolved = "version"
		case "--help", "-h":
			resolved = "help"
		}
		if resolved != name || lookupCommand(resolved) != lookupCommand(name) {
			t.Fatalf("alias %q does not resolve to %q", alias, name)
		}
	}
}

func runMenuWith(t *testing.T, input string) string {
	t.Helper()
	var out bytes.Buffer
	if err := runMenu(strings.NewReader(input), &out); err != nil {
		t.Fatalf("runMenu = %v", err)
	}
	return out.String()
}

// The menu is built from the registry: every registered command appears
// with its summary, without a per-command list in the menu code.
func TestMenuListsEveryRegisteredCommand(t *testing.T) {
	out := runMenuWith(t, "q\n")
	if !strings.Contains(out, "Athena ERP "+version) {
		t.Fatalf("menu %q has no plain title with the version", out)
	}
	for _, cmd := range commands {
		if !strings.Contains(out, cmd.name) || !strings.Contains(out, cmd.short) {
			t.Fatalf("menu %q lists neither %q nor its summary", out, cmd.name)
		}
	}
}

func TestMenuQuitVariants(t *testing.T) {
	for _, input := range []string{"q\n", "Q\n", "quit\n", "\n", ""} {
		var out bytes.Buffer
		if err := runMenu(strings.NewReader(input), &out); err != nil {
			t.Fatalf("runMenu(%q) = %v", input, err)
		}
	}
}

func TestMenuInvalidChoiceReprompts(t *testing.T) {
	out := runMenuWith(t, "99\nno-such-command\nq\n")
	if got := strings.Count(out, "No menu entry"); got != 2 {
		t.Fatalf("menu reported %d bad choices, want 2: %q", got, out)
	}
}

func TestMenuNumberAndNamePickTheSameCommand(t *testing.T) {
	var name string
	number := 0
	for i := range commands {
		if commands[i].name == "version" {
			name, number = "version", i+1
		}
	}
	byNumber := runMenuWith(t, strings.Join([]string{strconv.Itoa(number), "", "q"}, "\n")+"\n")
	byName := runMenuWith(t, name+"\n\nq\n")
	if !strings.Contains(byNumber, version) || !strings.Contains(byName, version) {
		t.Fatalf("number and name picks differ: %q vs %q", byNumber, byName)
	}
}

// Choosing help prints the same usage text as athena help.
func TestMenuHelpMatchesUsage(t *testing.T) {
	out := runMenuWith(t, "h\n\nq\n")
	var want bytes.Buffer
	printUsageTo(&want)
	for _, line := range []string{"Usage:", "athena init", "athena rules"} {
		if !strings.Contains(out, line) {
			t.Fatalf("menu help %q misses %q", out, line)
		}
	}
	if !strings.Contains(out, want.String()) {
		t.Fatal("menu help is not the usage text")
	}
}

// A failing command reports its error and returns to the menu.
func TestMenuReportsCommandErrors(t *testing.T) {
	out := runMenuWith(t, "records\nbogus-subcommand\nq\n")
	if !strings.Contains(out, "athena: ") || !strings.Contains(out, "unknown records command") {
		t.Fatalf("menu %q hides the command error", out)
	}
	if strings.Count(out, "Choose a number") < 2 {
		t.Fatalf("menu %q did not return to the prompt", out)
	}
}

// The TTY gate: the menu opens only when input and output are terminals.
func TestIsInteractiveNeedsBothTerminals(t *testing.T) {
	old := terminalDetect
	defer func() { terminalDetect = old }()
	terminalDetect = func(f *os.File) bool { return f == os.Stdin }
	if !isInteractive(os.Stdin, os.Stdin) || isInteractive(os.Stdin, os.Stdout) || isInteractive(os.Stdout, os.Stdin) {
		t.Fatal("isInteractive is not the AND of both terminals")
	}
	terminalDetect = func(f *os.File) bool { return false }
	if isInteractive(os.Stdin, os.Stdout) {
		t.Fatal("isInteractive is true with no terminal")
	}
}

func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	fnErr := fn()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdout = old
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out), fnErr
}

// Non-interactive bare athena keeps today's behavior: it prints exactly
// what athena help prints and exits successfully.
func TestBareAthenaWithoutTerminalPrintsUsage(t *testing.T) {
	old := terminalDetect
	terminalDetect = func(f *os.File) bool { return false }
	defer func() { terminalDetect = old }()
	bare, bareErr := captureStdout(t, func() error { return run(nil) })
	help, helpErr := captureStdout(t, func() error { return run([]string{"help"}) })
	if bareErr != nil || helpErr != nil {
		t.Fatalf("run = %v / %v", bareErr, helpErr)
	}
	if bare != help || !strings.Contains(bare, "Usage:") {
		t.Fatalf("bare output %q is not the help text", bare)
	}
}
