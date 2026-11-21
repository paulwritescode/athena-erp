package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	v1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/server"
	"athena-erp/internal/workspace"
)

func identityCommand(args []string, out io.Writer) error {
	if isHelp(args) {
		fmt.Fprintln(out, identityUsage)
		return nil
	}
	verb, args := args[0], args[1:]
	f := flag.NewFlagSet("workspace identity "+verb, flag.ContinueOnError)
	slug := f.String("workspace", "", "Workspace slug")
	var name, light, dark string
	switch verb {
	case "show":
	case "set":
		f.StringVar(&name, "name", "", "Workspace display name, 1 to 80 characters")
		f.StringVar(&light, "logo-light", "", "logo file for the light theme (PNG or SVG, at most 512 KB)")
		f.StringVar(&dark, "logo-dark", "", "logo file for the dark theme (PNG or SVG, at most 512 KB)")
	default:
		return errors.New(identityUsage)
	}
	if ok, err := parse(f, args); !ok {
		return err
	}
	if f.NArg() != 0 {
		return errors.New(identityUsage)
	}
	k, ctx, close, err := openKernel(*slug)
	if err != nil {
		return err
	}
	defer close()
	if verb == "show" {
		result, err := k.Workspace.GetIdentity(ctx, &v1.GetIdentityRequest{})
		if err != nil {
			return err
		}
		return writeJSON(out, result)
	}
	req := &v1.SetIdentityRequest{DisplayName: name}
	if light != "" {
		upload, err := readLogo(light)
		if err != nil {
			return err
		}
		req.LogoLight = upload
	}
	if dark != "" {
		upload, err := readLogo(dark)
		if err != nil {
			return err
		}
		req.LogoDark = upload
	}
	result, err := k.Workspace.SetIdentity(ctx, req)
	if err != nil {
		return err
	}
	return writeJSON(out, result)
}

// readLogo loads a logo file for upload. The server takes the file type from
// the name and checks the size, type and dimensions.
func readLogo(path string) (*v1.LogoUpload, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return &v1.LogoUpload{Content: content, Filename: filepath.Base(path)}, nil
}

// setNewLogo stores the display name and the --logo file of a workspace that
// athena new just created. Opening the kernel applies the system migrations
// first, so the settings and files tables exist.
func setNewLogo(ctx context.Context, upload *v1.LogoUpload, result workspace.CreateResult, driver, serverURL string) error {
	endpoint, err := workspace.ParseEndpoint(driver, serverURL, result.Manifest.Database.Name)
	if err != nil {
		return err
	}
	svc := &server.Workspace{ID: result.Manifest.ID, Name: result.Manifest.Name, Slug: result.Manifest.Slug, Root: result.Root}
	k, err := server.OpenKernel(ctx, endpoint, svc)
	if err != nil {
		return err
	}
	defer func() { _ = k.Close() }()
	_, err = k.Workspace.SetIdentity(ctx, &v1.SetIdentityRequest{DisplayName: result.Manifest.Name, LogoLight: upload})
	return err
}
