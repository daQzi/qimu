// Package authoring provides offline author tools using the installer's contract.
// It does not install, authorize or execute plugins.
package authoring

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"sort"

	"infinite-canvas/backend/internal/plugins/contracts"
	"infinite-canvas/backend/internal/protocol"
)

func ReadDirectory(dir string) (contracts.PackageFiles, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	files := contracts.PackageFiles{}
	total := 0
	err = fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlinks are not allowed: %s", name)
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("only regular files are allowed: %s", name)
		}
		if info.Size() > 16<<20 || len(files) >= 256 {
			return fmt.Errorf("package limits exceeded")
		}
		file, err := root.Open(name)
		if err != nil {
			return err
		}
		raw, readErr := io.ReadAll(io.LimitReader(file, (16<<20)+1))
		closeErr := file.Close()
		if readErr != nil {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		total += len(raw)
		if len(raw) > 16<<20 || total > 64<<20 {
			return fmt.Errorf("package limits exceeded")
		}
		files[name] = raw
		return nil
	})
	return files, err
}

func ReadPackage(path string, policy contracts.Policy) (contracts.PackageFiles, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, protocol.PluginPackageMaxBytes+1))
	if err != nil {
		return nil, err
	}
	return ValidateArchive(raw, policy)
}

func ValidateArchive(raw []byte, policy contracts.Policy) (contracts.PackageFiles, error) {
	envelope, err := protocol.ReadPluginPackageEnvelope(raw)
	if err != nil {
		return nil, err
	}
	files := contracts.PackageFiles(envelope.Files)
	if err = contracts.ValidatePackage(files, policy); err != nil {
		return nil, err
	}
	return files, nil
}

// WithVersion copies the map so release packaging cannot mutate author sources.
func WithVersion(files contracts.PackageFiles, version string) (contracts.PackageFiles, error) {
	value, err := contracts.Decode(files["manifest.json"])
	if err != nil {
		return nil, err
	}
	manifest, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("manifest must be an object")
	}
	manifest["version"] = version
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	updated := contracts.PackageFiles{}
	for name, content := range files {
		updated[name] = content
	}
	updated["manifest.json"] = raw
	return updated, nil
}

// Pack round-trips the transport envelope before returning a deliverable archive.
func Pack(files contracts.PackageFiles, policy contracts.Policy) ([]byte, error) {
	if err := contracts.ValidatePackage(files, policy); err != nil {
		return nil, err
	}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		entry, err := writer.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err = entry.Write(files[name]); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	if _, err := ValidateArchive(buffer.Bytes(), policy); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func WriteNewPackage(path string, raw []byte) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(raw)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(path) // Only the output exclusively created by this call.
		if writeErr != nil {
			return writeErr
		}
		return closeErr
	}
	return nil
}
