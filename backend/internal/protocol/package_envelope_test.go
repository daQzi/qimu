package protocol

import (
	"archive/zip"
	"bytes"
	"os"
	"testing"
)

func TestApplicationEnvelopeRejectsUnsafeEntries(t *testing.T) {
	for _, kind := range []string{"traversal", "duplicate", "symlink", "symlink-directory"} {
		t.Run(kind, func(t *testing.T) {
			var b bytes.Buffer
			w := zip.NewWriter(&b)
			manifest, _ := w.Create("manifest.json")
			manifest.Write([]byte(`{"apiVersion":"yingce.plugin/v3"}`))
			switch kind {
			case "traversal":
				f, _ := w.Create("skills/../escape.md")
				f.Write([]byte("x"))
			case "duplicate":
				f, _ := w.Create("manifest.json")
				f.Write([]byte("{}"))
			case "symlink", "symlink-directory":
				name := "skills/link.md"
				mode := os.ModeSymlink | 0600
				if kind == "symlink-directory" {
					name = "skills/link/"
				}
				h := &zip.FileHeader{Name: name}
				h.SetMode(mode)
				f, _ := w.CreateHeader(h)
				f.Write([]byte("../outside"))
			}
			w.Close()
			if _, err := ReadPluginPackageEnvelope(b.Bytes()); err == nil {
				t.Fatal("unsafe ZIP accepted")
			}
		})
	}
}

func TestApplicationEnvelopeDoesNotEnableLegacyParser(t *testing.T) {
	raw := zipPluginPackage(t, map[string][]byte{"manifest.json": []byte(`{"apiVersion":"yingce.plugin/v3"}`), "skills/read/SKILL.md": []byte("method")})
	if _, err := ReadPluginPackageEnvelope(raw); err != nil {
		t.Fatal(err)
	}
	if _, err := ParsePluginPackage(raw); err == nil {
		t.Fatal("legacy runtime accepted an application")
	}
}
