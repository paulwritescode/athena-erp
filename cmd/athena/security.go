package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"slices"
	"strconv"
)

const securityUsage = "usage: athena security show [--workspace slug] | set <key> <integer> [--workspace slug]"

func securityCommand(args []string, out io.Writer) error {
	if isHelp(args) {
		fmt.Fprintln(out, securityUsage)
		return nil
	}
	sub, n := args[0], 0
	switch sub {
	case "show":
	case "set":
		n = 2
	default:
		return errors.New(securityUsage)
	}
	pos, args, err := leadingArgs(args[1:], n)
	if err != nil {
		return errors.New(securityUsage)
	}
	flags := flag.NewFlagSet("security "+sub, flag.ContinueOnError)
	slug := flags.String("workspace", "", "workspace slug")
	if ok, err := parse(flags, args); !ok {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New(securityUsage)
	}
	var value int
	if sub == "set" {
		value, err = strconv.Atoi(pos[1])
		if err != nil {
			return errors.New("the setting needs an integer")
		}
	}
	kernel, ctx, closeKernel, err := openKernel(*slug)
	if err != nil {
		return err
	}
	defer closeKernel()
	if sub == "set" {
		if err := kernel.Users.SetSecurity(ctx, pos[0], value); err != nil {
			return err
		}
		fmt.Fprintf(out, "%s = %d\n", pos[0], value)
		return nil
	}
	settings, err := kernel.Users.Security(ctx)
	if err != nil {
		return err
	}
	for _, key := range slices.Sorted(maps.Keys(settings)) {
		fmt.Fprintf(out, "%s = %d\n", key, settings[key])
	}
	return nil
}
