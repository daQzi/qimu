// plugin-contract validates extracted P00 fixtures offline. It never installs
// or executes a plugin and does not access databases or remote services.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"infinite-canvas/backend/internal/plugins/contracts"
)

func main() {
	dir := flag.String("dir", "", "extracted UTF-8 plugin directory (not a ZIP)")
	reserved := flag.String("reserved-ids", "", "comma-separated IDs reserved by the trusted catalog")
	flag.Parse()
	if *dir == "" {
		fmt.Fprintln(os.Stderr, "usage: plugin-contract -dir <directory>")
		os.Exit(2)
	}
	files := contracts.PackageFiles{}
	total := int64(0)
	err := filepath.WalkDir(*dir, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlinks are not allowed")
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("only regular files are allowed")
		}
		total += info.Size()
		if info.Size() > 16<<20 || total > 64<<20 || len(files) >= 256 {
			return fmt.Errorf("package limits exceeded")
		}
		relative, err := filepath.Rel(*dir, name)
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(relative)] = raw
		return nil
	})
	if err == nil {
		err = contracts.ValidatePackage(files, contracts.Policy{ReservedIDs: strings.Split(*reserved, ",")})
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"valid": true, "profile": "p00-contract/1", "runtimeEnabled": false, "fileCount": len(files), "packageDigest": contracts.PackageDigest(files)})
}
