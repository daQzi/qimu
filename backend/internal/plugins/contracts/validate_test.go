package contracts

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type fixtureChange struct {
	File       string  `json:"file"`
	Pointer    string  `json:"pointer"`
	Value      any     `json:"value"`
	Remove     bool    `json:"remove"`
	RemoveFile bool    `json:"removeFile"`
	Text       *string `json:"text"`
}
type fixtureCase struct {
	Name     string              `json:"name"`
	Kind     string              `json:"kind"`
	Contract string              `json:"contract"`
	Value    json.RawMessage     `json:"value"`
	Raw      *string             `json:"raw"`
	Reason   string              `json:"reason"`
	Changes  []fixtureChange     `json:"changes"`
	Graph    map[string][]string `json:"graph"`
}

func fixture(t *testing.T) PackageFiles {
	t.Helper()
	files := PackageFiles{}
	err := filepath.WalkDir("testdata/resource-helper", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		raw, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		files[strings.TrimPrefix(filepath.ToSlash(name), "testdata/resource-helper/")] = raw
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}
func mutate(t *testing.T, files PackageFiles, c fixtureChange) {
	t.Helper()
	if c.RemoveFile {
		delete(files, c.File)
		return
	}
	if c.Text != nil {
		files[c.File] = []byte(*c.Text)
		return
	}
	var doc any
	if err := json.Unmarshal(files[c.File], &doc); err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(strings.TrimPrefix(c.Pointer, "/"), "/")
	value := doc
	for _, p := range parts[:len(parts)-1] {
		switch v := value.(type) {
		case map[string]any:
			value = v[p]
		case []any:
			i, _ := strconv.Atoi(p)
			value = v[i]
		default:
			t.Fatal("invalid mutation")
		}
	}
	key := parts[len(parts)-1]
	switch v := value.(type) {
	case map[string]any:
		if c.Remove {
			delete(v, key)
		} else {
			v[key] = c.Value
		}
	case []any:
		i, _ := strconv.Atoi(key)
		v[i] = c.Value
	default:
		t.Fatal("invalid target")
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	files[c.File] = raw
}
func TestSharedContractCorpus(t *testing.T) {
	raw, err := os.ReadFile("testdata/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []fixtureCase
	if err = json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			var err error
			switch c.Kind {
			case "package":
				files := fixture(t)
				for _, m := range c.Changes {
					mutate(t, files, m)
				}
				err = ValidatePackage(files, Policy{ReservedIDs: []string{"official-tools"}})
			case "contract":
				raw := []byte(c.Value)
				if c.Raw != nil {
					raw = []byte(*c.Raw)
				}
				err = Validate(c.Contract, raw)
			case "graph":
				err = ValidateDependencyGraph(c.Graph)
			default:
				t.Fatalf("unknown case %q", c.Kind)
			}
			if c.Reason == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			var issue *ContractError
			if !errors.As(err, &issue) || issue.Reason != c.Reason {
				t.Fatalf("wanted %s, got %v", c.Reason, err)
			}
		})
	}
}
func TestWireTypesRoundTrip(t *testing.T) {
	files := fixture(t)
	for _, v := range []struct {
		kind, file string
		target     any
	}{{"manifest", "manifest.json", &Manifest{}}, {"operation", "operations/inspect-video.json", &Operation{}}} {
		if err := json.Unmarshal(files[v.file], v.target); err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(v.target)
		if err != nil {
			t.Fatal(err)
		}
		if err = Validate(v.kind, raw); err != nil {
			t.Fatal(err)
		}
	}
}
func TestPackageDigestGolden(t *testing.T) {
	raw, err := os.ReadFile("testdata/digests.json")
	if err != nil {
		t.Fatal(err)
	}
	var expected map[string]string
	if err = json.Unmarshal(raw, &expected); err != nil {
		t.Fatal(err)
	}
	files := fixture(t)
	digest := PackageDigest(files)
	if digest != expected["packageDigest"] {
		t.Fatalf("package digest %s", digest)
	}
	if got := OperationDigest(digest, "resource-helper.inspect-video"); got != expected["operationDigest"] {
		t.Fatalf("operation digest %s", got)
	}
	files["skills/check-source/SKILL.md"] = append(files["skills/check-source/SKILL.md"], byte('\n'))
	if PackageDigest(files) == digest {
		t.Fatal("file content change must invalidate package and operation identity")
	}
}
func TestPackageBounds(t *testing.T) {
	files := fixture(t)
	files["skills/check-source/large.md"] = []byte(strings.Repeat("x", (16<<20)+1))
	if err := ValidatePackage(files, Policy{}); err == nil {
		t.Fatal("oversized entry accepted")
	}
	if _, err := Decode([]byte{0xff}); err == nil {
		t.Fatal("invalid utf8 accepted")
	}
}
