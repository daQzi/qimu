package authoring

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"infinite-canvas/backend/internal/plugins/contracts"
)

func TestTemplatesDeliverInstallableArchives(t *testing.T) {
	for _, kind := range []string{"skill", "resource"} {
		t.Run(kind, func(t *testing.T) {
			files, err := Template(kind, "delivery-check", "studio", contracts.Policy{})
			if err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(t.TempDir(), "source")
			if err = Init(dir, files); err != nil {
				t.Fatal(err)
			}
			if err = Init(dir, files); err == nil {
				t.Fatal("overwrote existing template")
			}
			read, err := ReadDirectory(dir)
			if err != nil {
				t.Fatal(err)
			}
			updated, err := WithVersion(read, "1.1.0")
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Equal(updated["manifest.json"], read["manifest.json"]) {
				t.Fatal("version not changed")
			}
			if !bytes.Equal(read["manifest.json"], files["manifest.json"]) {
				t.Fatal("source mutated")
			}
			raw, err := Pack(updated, contracts.Policy{})
			if err != nil {
				t.Fatal(err)
			}
			other, err := Pack(updated, contracts.Policy{})
			if err != nil || !bytes.Equal(raw, other) {
				t.Fatal("unstable archive", err)
			}
			path := filepath.Join(t.TempDir(), "delivery.yingce-plugin")
			if err = WriteNewPackage(path, raw); err != nil {
				t.Fatal(err)
			}
			if err = WriteNewPackage(path, []byte("overwrite")); err == nil {
				t.Fatal("overwrote archive")
			}
			validated, err := ReadPackage(path, contracts.Policy{})
			if err != nil || contracts.PackageDigest(validated) != contracts.PackageDigest(updated) {
				t.Fatal("installer mismatch", err)
			}
			if _, err = ReadPackage(path, contracts.Policy{ReservedIDs: []string{"delivery-check"}}); err == nil {
				t.Fatal("reserved ID accepted")
			}
		})
	}
}

func TestAuthoringRejectsUnsafeSourcesAndArchives(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("external data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "manifest.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadDirectory(dir); err == nil {
		t.Fatal("followed symlink")
	}
	bigDir := t.TempDir()
	file, err := os.Create(filepath.Join(bigDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = file.Truncate((16 << 20) + 1); err != nil {
		t.Fatal(err)
	}
	file.Close()
	if _, err = ReadDirectory(bigDir); err == nil {
		t.Fatal("oversize source accepted")
	}
	for _, paths := range [][]string{{"../manifest.json"}, {"manifest.json", "manifest.json"}, {"manifest.json", "backend/run.js"}} {
		var buffer bytes.Buffer
		writer := zip.NewWriter(&buffer)
		for _, path := range paths {
			entry, err := writer.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = entry.Write([]byte("{}")); err != nil {
				t.Fatal(err)
			}
		}
		if err = writer.Close(); err != nil {
			t.Fatal(err)
		}
		if _, err = ValidateArchive(buffer.Bytes(), contracts.Policy{}); err == nil {
			t.Fatal("unsafe archive accepted", paths)
		}
	}
	for _, id := range []string{"../bad", "bad\"", ""} {
		if _, err = Template("resource", id, "studio", contracts.Policy{}); err == nil {
			t.Fatal("unsafe ID", id)
		}
	}
	if _, err = Template("unknown", "valid", "studio", contracts.Policy{}); err == nil {
		t.Fatal("unknown template")
	}
}
