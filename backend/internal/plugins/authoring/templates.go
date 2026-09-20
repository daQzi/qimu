package authoring

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"infinite-canvas/backend/internal/plugins/contracts"
)

//go:embed templates
var templates embed.FS

var authorID = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

func Template(kind, id, publisher string, policy contracts.Policy) (contracts.PackageFiles, error) {
	if kind != "skill" && kind != "resource" {
		return nil, fmt.Errorf("template must be skill or resource")
	}
	if !authorID.MatchString(id) || !authorID.MatchString(publisher) {
		return nil, fmt.Errorf("id and publisher must be lowercase IDs (1-64 characters, starting with a letter)")
	}
	files := contracts.PackageFiles{}
	root, err := fs.Sub(templates, "templates/"+kind)
	if err != nil {
		return nil, err
	}
	err = fs.WalkDir(root, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		raw, err := fs.ReadFile(root, name)
		if err != nil {
			return err
		}
		files[name] = []byte(strings.NewReplacer("__PLUGIN_ID__", id, "__PUBLISHER_ID__", publisher).Replace(string(raw)))
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err = contracts.ValidatePackage(files, policy); err != nil {
		return nil, err
	}
	return files, nil
}

// Init refuses an existing destination; templates are validated before any write.
func Init(dir string, files contracts.PackageFiles) error {
	if err := contracts.ValidatePackage(files, contracts.Policy{}); err != nil {
		return err
	}
	if err := os.Mkdir(dir, 0700); err != nil {
		return err
	}
	for name, raw := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return fmt.Errorf("partial template at %s: %w", dir, err)
		}
		if err := WriteNewPackage(path, raw); err != nil {
			return fmt.Errorf("partial template at %s: %w", dir, err)
		}
	}
	return nil
}
