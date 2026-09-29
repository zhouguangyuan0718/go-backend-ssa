package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"

	"go-backend-ssa/internal/port"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "syncgo:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: syncgo inspect|generate|check -repo PATH [-root PATH] [-out PATH] [-summary PATH]")
	}
	command := args[0]
	if command != "inspect" && command != "generate" && command != "check" {
		return fmt.Errorf("unknown command %q", command)
	}
	f := flag.NewFlagSet(command, flag.ContinueOnError)
	repo := f.String("repo", "", "local Go Git repository containing the pin")
	root := f.String("root", ".", "project root")
	out := f.String("out", "", "inspect output; empty writes JSON to stdout")
	summary := f.String("summary", "", "compact inspect summary path")
	if err := f.Parse(args[1:]); err != nil {
		return err
	}
	if *repo == "" || f.NArg() != 0 {
		return fmt.Errorf("-repo is required and positional arguments are not accepted")
	}
	if command != "inspect" && (*out != "" || *summary != "") {
		return fmt.Errorf("-out and -summary only apply to inspect")
	}
	var err error
	*root, err = filepath.Abs(*root)
	if err != nil {
		return err
	}
	resolve := func(p string) string {
		if filepath.IsAbs(p) {
			return p
		}
		return filepath.Join(*root, p)
	}
	pin, plan := port.Baseline(), port.Migration()
	if err := plan.Validate(); err != nil {
		return err
	}
	if runtime.Version() != pin.BuildToolchain {
		return fmt.Errorf("use GOTOOLCHAIN=%s (running %s)", pin.BuildToolchain, runtime.Version())
	}
	s, err := port.OpenSource(ctx, *repo, pin)
	if err != nil {
		return err
	}
	if command == "inspect" {
		r, err := port.Inspect(s, plan.InspectRoots)
		if err != nil {
			return err
		}
		b, err := port.JSON(r)
		if err != nil {
			return err
		}
		if *summary != "" {
			if *out != "" && resolve(*summary) == resolve(*out) {
				return fmt.Errorf("summary and full report must use distinct paths")
			}
			data, e := port.JSON(port.Summarize(r, b))
			if e != nil {
				return e
			}
			name := resolve(*summary)
			if e = os.MkdirAll(filepath.Dir(name), 0755); e != nil {
				return e
			}
			if e = os.WriteFile(name, data, 0644); e != nil {
				return e
			}
		}
		if *out == "" {
			_, err = os.Stdout.Write(b)
			return err
		}
		name := resolve(*out)
		if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(name, b, 0644); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "inspected %d packages at %s → %s\n", len(r.Packages), pin.Tag, name)
		return nil
	}
	r, err := port.Render(s, plan, *root)
	if err != nil {
		return err
	}
	if command == "check" {
		err = port.Check(*root, r)
	} else {
		err = port.Install(*root, r)
	}
	if err == nil {
		fmt.Fprintf(os.Stderr, "%s: %d files verified at %s\n", command, len(r.Ledger.Files), pin.Tag)
	}
	return err
}
