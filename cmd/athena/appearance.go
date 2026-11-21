package main

import (
	"errors"
	"flag"
	"fmt"
	"io"

	v1 "athena-erp/gen/athena/v1"
)

const appearanceUsage = "usage: athena workspace appearance show | set (--preset ink|red | --accent 'oklch(...)' [--chart1 ...] ... [--chart5 ...]) [--workspace slug]"

const identityUsage = "usage: athena workspace identity show | set [--name text] [--logo-light file] [--logo-dark file] [--workspace slug]"

func workspaceCommand(args []string, out io.Writer) error {
	if isHelp(args) {
		fmt.Fprintln(out, appearanceUsage)
		fmt.Fprintln(out, identityUsage)
		return nil
	}
	switch args[0] {
	case "appearance":
		return appearanceCommand(args[1:], out)
	case "identity":
		return identityCommand(args[1:], out)
	default:
		return errors.New(appearanceUsage + "\n" + identityUsage)
	}
}

func appearanceCommand(args []string, out io.Writer) error {
	if isHelp(args) {
		fmt.Fprintln(out, appearanceUsage)
		return nil
	}
	verb, args := args[0], args[1:]
	f := flag.NewFlagSet("workspace appearance "+verb, flag.ContinueOnError)
	slug := f.String("workspace", "", "Workspace slug")
	a := &v1.Appearance{}
	var charts [5]string
	switch verb {
	case "show":
	case "set":
		f.StringVar(&a.Preset, "preset", "", "ink or red")
		f.StringVar(&a.Accent, "accent", "", "custom light-theme accent")
		for i := range charts {
			f.StringVar(&charts[i], fmt.Sprintf("chart%d", i+1), "", "custom chart color")
		}
	default:
		return errors.New(appearanceUsage)
	}
	if ok, err := parse(f, args); !ok {
		return err
	}
	if f.NArg() != 0 {
		return errors.New(appearanceUsage)
	}
	if verb == "set" {
		if a.Accent != "" {
			if a.Preset != "" {
				return errors.New(appearanceUsage)
			}
			a.Preset = "custom"
		}
		for i := len(charts) - 1; i >= 0; i-- {
			if charts[i] != "" {
				a.ChartColors = charts[:i+1]
				break
			}
		}
	}
	k, ctx, close, err := openKernel(*slug)
	if err != nil {
		return err
	}
	defer close()
	if verb == "show" {
		result, err := k.Workspace.GetAppearance(ctx, &v1.GetAppearanceRequest{})
		if err != nil {
			return err
		}
		return writeJSON(out, result)
	}
	result, err := k.Workspace.SetAppearance(ctx, &v1.SetAppearanceRequest{Appearance: a})
	if err != nil {
		return err
	}
	return writeJSON(out, result)
}
