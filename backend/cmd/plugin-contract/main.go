// plugin-contract validates extracted P00 fixtures offline. It never installs
// or executes a plugin and does not access databases or remote services.
package main

import (
	"archive/zip"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"infinite-canvas/backend/internal/plugins/contracts"
)

func main() {
	dir := flag.String("dir", "", "extracted UTF-8 plugin directory (not a ZIP)")
	reserved := flag.String("reserved-ids", "", "comma-separated IDs reserved by the trusted catalog")
	output := flag.String("out", "", "optional .yingce-plugin output; never overwrites an existing file")
	version := flag.String("version", "", "optional release version override in the generated package; source files remain unchanged")
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
		if *version != "" {
			var value any
			value, err = contracts.Decode(files["manifest.json"])
			if err == nil {
				manifest, ok := value.(map[string]any)
				if !ok {
					err = fmt.Errorf("manifest must be an object")
				} else {
					manifest["version"] = *version
					files["manifest.json"], err = json.MarshalIndent(manifest, "", "  ")
				}
			}
		}
	}
	if err == nil {
		err = contracts.ValidatePackage(files, contracts.Policy{ReservedIDs: strings.Split(*reserved, ",")})
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *output != "" {
		file, err := os.OpenFile(*output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		writer := zip.NewWriter(file)
		names := make([]string, 0, len(files))
		for name := range files {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			entry, e := writer.Create(name)
			if e != nil {
				err = e
				break
			}
			if _, e = entry.Write(files[name]); e != nil {
				err = e
				break
			}
		}
		if e := writer.Close(); err == nil {
			err = e
		}
		if e := file.Close(); err == nil {
			err = e
		}
		if err != nil {
			os.Remove(*output)
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"valid": true, "profile": "p00-contract/1", "runtimeEnabled": false, "fileCount": len(files), "packageDigest": contracts.PackageDigest(files)})
}
