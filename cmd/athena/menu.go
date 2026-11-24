package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"
)

// terminalDetect reports whether f is a terminal. It is a variable so
// tests can stub it; production uses golang.org/x/term.
var terminalDetect = func(f *os.File) bool { return term.IsTerminal(int(f.Fd())) }

// isInteractive reports whether bare athena should open the menu: only
// when both input and output are terminals. Piped or redirected use
// keeps the old behavior of printing the usage text.
func isInteractive(in, out *os.File) bool {
	return terminalDetect(in) && terminalDetect(out)
}

// runMenu shows the numbered command menu and dispatches the chosen
// entries through the same handlers as the command line. The list is
// built from the command registry, so newly registered commands appear
// with no other change. It returns when the user quits or input ends.
func runMenu(in io.Reader, out io.Writer) error {
	reader := bufio.NewReader(in)
	for {
		printMenu(out)
		choice, err := readLine(reader, out, "Choose a number, a name, or q to quit: ")
		if err != nil {
			fmt.Fprintln(out)
			return nil
		}
		if lower := strings.ToLower(choice); lower == "" || lower == "q" || lower == "quit" {
			return nil
		}
		cmd := menuPick(choice)
		if cmd == nil {
			fmt.Fprintf(out, "No menu entry %q. Pick a number or a name from the list.\n\n", choice)
			continue
		}
		argsLine, err := readLine(reader, out, fmt.Sprintf("Arguments for %s (Enter for none, q goes back): ", cmd.name))
		if err != nil {
			fmt.Fprintln(out)
			return nil
		}
		if lower := strings.ToLower(argsLine); lower == "q" || lower == "quit" {
			continue
		}
		if err := cmd.run(strings.Fields(argsLine), out); err != nil {
			fmt.Fprintf(out, "athena: %v\n", err)
		}
		fmt.Fprintln(out)
	}
}

func printMenu(out io.Writer) {
	fmt.Fprintf(out, "Athena ERP %s\n\n", version)
	for i := range commands {
		fmt.Fprintf(out, "  %2d  %-14s %s\n", i+1, commands[i].name, commands[i].short)
	}
	fmt.Fprint(out, "   q  quit           Leave the menu.\n\n")
}

// menuPick resolves a menu choice: a 1-based number, a command name, or
// the h and v shortcuts for help and version.
func menuPick(choice string) *cliCommand {
	if n, err := strconv.Atoi(choice); err == nil {
		if n >= 1 && n <= len(commands) {
			return &commands[n-1]
		}
		return nil
	}
	switch lower := strings.ToLower(choice); lower {
	case "h":
		return lookupCommand("help")
	case "v":
		return lookupCommand("version")
	default:
		return lookupCommand(lower)
	}
}

func readLine(reader *bufio.Reader, out io.Writer, prompt string) (string, error) {
	fmt.Fprint(out, prompt)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}
