// plugin-contract creates, validates and packages application plugins offline.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"infinite-canvas/backend/internal/plugins/authoring"
	"infinite-canvas/backend/internal/plugins/contracts"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("plugin-contract", flag.ContinueOnError)
	flags.SetOutput(stderr)
	dir := flags.String("dir", "", "validate an extracted UTF-8 plugin directory")
	archive := flags.String("package", "", "validate a finished .yingce-plugin archive")
	initDir := flags.String("init", "", "create a template in a new directory (parent must exist)")
	template := flags.String("template", "skill", "template: skill or resource")
	id := flags.String("id", "", "new plugin ID, required with -init")
	publisher := flags.String("publisher", "", "publisher ID, required with -init")
	reserved := flags.String("reserved-ids", "", "comma-separated IDs reserved by the trusted catalog")
	output := flags.String("out", "", "optional archive output with -dir; never overwrites")
	version := flags.String("version", "", "package version override with -dir; source unchanged")
	if err := flags.Parse(args); err != nil {
		return err
	}
	modes := 0
	for _, value := range []string{*dir, *archive, *initDir} {
		if value != "" {
			modes++
		}
	}
	if modes != 1 || flags.NArg() != 0 {
		return fmt.Errorf("choose exactly one of -dir, -package or -init")
	}
	if *dir == "" && (*output != "" || *version != "") {
		return fmt.Errorf("-out and -version require -dir")
	}
	if *version != "" && *output == "" {
		return fmt.Errorf("-version requires -out")
	}
	if *initDir == "" {
		invalid := false
		flags.Visit(func(f *flag.Flag) {
			if f.Name == "template" || f.Name == "id" || f.Name == "publisher" {
				invalid = true
			}
		})
		if invalid {
			return fmt.Errorf("-template, -id and -publisher require -init")
		}
	}
	policy := contracts.Policy{ReservedIDs: strings.Split(*reserved, ",")}
	var files contracts.PackageFiles
	var err error
	switch {
	case *initDir != "":
		files, err = authoring.Template(*template, *id, *publisher, policy)
		if err == nil {
			err = authoring.Init(*initDir, files)
		}
	case *archive != "":
		files, err = authoring.ReadPackage(*archive, policy)
	default:
		files, err = authoring.ReadDirectory(*dir)
		if err == nil && *version != "" {
			files, err = authoring.WithVersion(files, *version)
		}
		if err == nil {
			err = contracts.ValidatePackage(files, policy)
		}
		if err == nil && *output != "" {
			var raw []byte
			raw, err = authoring.Pack(files, policy)
			if err == nil {
				err = authoring.WriteNewPackage(*output, raw)
			}
		}
	}
	if err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(map[string]any{
		"valid": true, "apiVersion": "yingce.plugin/v3", "profile": "p00-contract/1",
		"runtimeVerified": false, "fileCount": len(files), "packageDigest": contracts.PackageDigest(files),
	})
}
