// Package contracts validates the P00 offline plugin contract. It does not
// install plugins, authorize calls, load credentials, or register executors.
package contracts

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

//go:embed schema.json profile.json
var definitions embed.FS

const schemaURL = "https://qimu.invalid/contracts/plugin-v3"
const packageURL = "https://qimu.invalid/package/"

type ContractError struct {
	Reason   string
	Location string
}

func (e *ContractError) Error() string      { return e.Reason + ": " + e.Location }
func invalid(reason, location string) error { return &ContractError{reason, location} }

type Profile struct {
	APIVersion     string `json:"apiVersion"`
	RuntimeEnabled bool   `json:"runtimeEnabled"`
	Limits         struct {
		PackageBytes  int `json:"packageBytes"`
		ManifestBytes int `json:"manifestBytes"`
		MaxFiles      int `json:"maxFiles"`
		EntryBytes    int `json:"entryBytes"`
		ExpandedBytes int `json:"expandedBytes"`
		JSONBytes     int `json:"jsonBytes"`
		JSONDepth     int `json:"jsonDepth"`
	} `json:"limits"`
}

var initOnce sync.Once
var compiled map[string]*jsonschema.Schema
var profile Profile
var initError error

func initialize() {
	raw, _ := definitions.ReadFile("profile.json")
	if initError = json.Unmarshal(raw, &profile); initError != nil {
		return
	}
	raw, _ = definitions.ReadFile("schema.json")
	var schema map[string]any
	if initError = json.Unmarshal(raw, &schema); initError != nil {
		return
	}
	compiler := jsonschema.NewCompiler() // No network loader is registered.
	compiler.DefaultDraft(jsonschema.Draft2020)
	if initError = compiler.AddResource(schemaURL, schema); initError != nil {
		return
	}
	compiled = map[string]*jsonschema.Schema{}
	for name := range schema["$defs"].(map[string]any) {
		compiled[name], initError = compiler.Compile(schemaURL + "#/$defs/" + name)
		if initError != nil {
			return
		}
	}
}

// Decode rejects duplicate keys, excessive depth and non-interoperable numbers
// before validation. JSON objects are decoded identically to browser JSON data.
func Decode(raw []byte) (any, error) {
	initOnce.Do(initialize)
	if initError != nil {
		return nil, initError
	}
	if len(raw) > profile.Limits.JSONBytes || !utf8.Valid(raw) {
		return nil, invalid("contract_invalid", "JSON size/encoding")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var parse func(int) (any, error)
	parse = func(depth int) (any, error) {
		if depth > profile.Limits.JSONDepth {
			return nil, invalid("contract_invalid", "JSON depth")
		}
		tok, err := d.Token()
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case json.Delim:
			if t == '{' {
				result := map[string]any{}
				for d.More() {
					key, err := d.Token()
					if err != nil {
						return nil, err
					}
					k, ok := key.(string)
					if !ok {
						return nil, fmt.Errorf("invalid object key")
					}
					if _, ok := result[k]; ok {
						return nil, fmt.Errorf("duplicate key")
					}
					v, err := parse(depth + 1)
					if err != nil {
						return nil, err
					}
					result[k] = v
				}
				_, err = d.Token()
				return result, err
			}
			if t == '[' {
				result := []any{}
				for d.More() {
					v, err := parse(depth + 1)
					if err != nil {
						return nil, err
					}
					result = append(result, v)
				}
				_, err = d.Token()
				return result, err
			}
			return nil, fmt.Errorf("unexpected delimiter")
		case json.Number:
			n, err := strconv.ParseFloat(string(t), 64)
			if err != nil || n > 9007199254740991 || n < -9007199254740991 {
				return nil, fmt.Errorf("number outside interoperable range")
			}
			return n, nil
		default:
			return tok, nil
		}
	}
	value, err := parse(0)
	if err != nil {
		return nil, invalid("contract_invalid", "JSON syntax/depth/number")
	}
	if _, err = d.Token(); err != io.EOF {
		return nil, invalid("contract_invalid", "trailing JSON")
	}
	return value, nil
}

func Validate(kind string, raw []byte) error {
	value, err := Decode(raw)
	if err != nil {
		return err
	}
	schema, ok := compiled[kind]
	if !ok {
		return invalid("contract_invalid", "unknown contract kind")
	}
	if err := schema.Validate(value); err != nil {
		return invalid("contract_invalid", kind)
	}
	if kind == "pipeline" {
		return validatePipeline(value.(map[string]any))
	}
	return nil
}

// PackageFiles is an already extracted UTF-8 text package; ZIP safety remains
// the installer's responsibility. P00 only supplies offline contract admission.
type PackageFiles map[string][]byte
type Policy struct{ ReservedIDs []string }

func validPath(name string) bool {
	if name == "manifest.json" {
		return true
	}
	if path.Clean(name) != name || strings.ContainsAny(name, "\\%:#\x00") || strings.HasPrefix(name, "/") {
		return false
	}
	roots := []string{"skills/", "operations/", "pipelines/", "schemas/", "views/", "connectors/", "blueprints/"}
	for _, root := range roots {
		if strings.HasPrefix(name, root) {
			for _, c := range name {
				if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("_./-", c)) {
					return false
				}
			}
			return strings.HasSuffix(name, ".json") || root == "skills/" && strings.HasSuffix(name, ".md")
		}
	}
	return false
}

func ValidatePackage(files PackageFiles, policy Policy) error {
	initOnce.Do(initialize)
	if initError != nil {
		return initError
	}
	if len(files) == 0 || len(files) > profile.Limits.MaxFiles {
		return invalid("contract_invalid", "file count")
	}
	total := 0
	docs := map[string]map[string]any{}
	for name, raw := range files {
		if !validPath(name) {
			return invalid("package_reference_invalid", name)
		}
		total += len(raw)
		if len(raw) > profile.Limits.EntryBytes || total > profile.Limits.ExpandedBytes || !utf8.Valid(raw) {
			return invalid("contract_invalid", "file size/encoding")
		}
		if strings.HasSuffix(name, ".json") {
			v, err := Decode(raw)
			if err != nil {
				return err
			}
			o, ok := v.(map[string]any)
			if !ok {
				return invalid("contract_invalid", name)
			}
			docs[name] = o
		}
	}
	if err := Validate("manifest", files["manifest.json"]); err != nil {
		return err
	}
	manifest := docs["manifest.json"]
	pluginID := manifest["id"].(string)
	if pluginID == "qimu" || strings.HasPrefix(pluginID, "qimu-") {
		return invalid("scope_forbidden", pluginID)
	}
	for _, id := range policy.ReservedIDs {
		if pluginID == id {
			return invalid("scope_forbidden", pluginID)
		}
	}
	dependencies := map[string]bool{}
	for _, v := range manifest["dependencies"].([]any) {
		d := v.(map[string]any)
		id := d["id"].(string)
		if id == pluginID {
			return invalid("dependency_cycle", id)
		}
		if dependencies[id] {
			return invalid("contract_invalid", "duplicate dependency")
		}
		dependencies[id] = true
	}
	contributes := manifest["contributes"].(map[string]any)
	registry := map[string]map[string]bool{}
	for _, kind := range []string{"skills", "operations", "views", "canvasBlueprints", "connectors", "pipelines"} {
		registry[kind] = map[string]bool{}
		entries, _ := contributes[kind].([]any)
		for _, v := range entries {
			entry := v.(map[string]any)
			id := entry["id"].(string)
			if registry[kind][id] {
				return invalid("contract_invalid", "duplicate contribution")
			}
			registry[kind][id] = true
			key := "ref"
			if kind == "skills" {
				key = "entry"
			}
			p := entry[key].(string)
			raw, ok := files[p]
			if !ok || !validPath(p) {
				return invalid("package_reference_invalid", p)
			}
			root := kind
			if kind == "canvasBlueprints" {
				root = "blueprints"
			}
			if !strings.HasPrefix(p, root+"/") {
				return invalid("package_reference_invalid", p)
			}
			if kind == "skills" {
				if !strings.HasSuffix(p, "/SKILL.md") || len(raw) == 0 {
					return invalid("contract_invalid", "skill entry")
				}
				continue
			}
			typ := map[string]string{"operations": "operation", "views": "view", "canvasBlueprints": "blueprint", "connectors": "httpConnector", "pipelines": "pipeline"}[kind]
			if err := Validate(typ, raw); err != nil {
				return err
			}
			if docs[p]["id"] != id {
				return invalid("contract_invalid", "contribution id mismatch")
			}
			if kind == "connectors" {
				if err := ValidateHTTPConnector(raw); err != nil {
					return err
				}
			}
		}
	}
	grants := map[string]bool{}
	for _, p := range manifest["permissions"].([]any) {
		grants[p.(string)] = true
	}
	for _, v := range asArray(contributes["operations"]) {
		op := docs[v.(map[string]any)["ref"].(string)]
		for _, k := range []string{"inputSchemaRef", "outputSchemaRef"} {
			p := op[k].(string)
			if _, ok := docs[p]; !ok || !strings.HasPrefix(p, "schemas/") {
				return invalid("package_reference_invalid", p)
			}
		}
		for _, p := range op["requiredPermissions"].([]any) {
			if !grants[p.(string)] {
				return invalid("scope_forbidden", "permission exceeds manifest")
			}
		}
		if view, ok := op["resultView"].(string); ok && !registry["views"][view] {
			return invalid("package_reference_invalid", view)
		}
		execution := op["execution"].(map[string]any)
		if execution["kind"] == "pipeline" {
			if err := validateSequentialPackage(files, op); err != nil {
				return err
			}
			continue
		}
		if execution["kind"] == "http" {
			connectorID := execution["connector"].(string)
			if !registry["connectors"][connectorID] {
				return invalid("operation_unavailable", "HTTP connector not registered")
			}
			var connector HTTPConnector
			for _, ref := range asArray(contributes["connectors"]) {
				entry := ref.(map[string]any)
				if entry["id"] == connectorID {
					if err := json.Unmarshal(files[entry["ref"].(string)], &connector); err != nil {
						return err
					}
				}
			}
			action, ok := connector.Actions[execution["action"].(string)]
			if !ok {
				return invalid("package_reference_invalid", "connector action")
			}
			permissions := map[string]bool{}
			for _, p := range asArray(op["requiredPermissions"]) {
				permissions[p.(string)] = true
			}
			effects := asArray(op["effects"])
			if !permissions["connection.use"] || (len(action.Resources) > 0 && !permissions["media.read"]) || (action.Artifact != nil && !permissions["resource.create"]) || len(effects) != 1 || (effects[0] != "external_write" && effects[0] != "generation") {
				return invalid("scope_forbidden", "HTTP operation effects and permissions")
			}
			continue
		}
		// Installation validates the admitted host contract, never permissions
		// invented by a package. Execution repeats the host minimum checks.
		if execution["kind"] != "host" || (execution["adapter"] != "resource.inspect" && execution["adapter"] != "resource.snapshot" && execution["adapter"] != "canvas.blueprint.instantiate") || execution["mode"] != "inline" {
			return invalid("operation_unavailable", "host adapter profile")
		}
		permissions := asArray(op["requiredPermissions"])
		hasRead := false
		hasCreate := false
		hasCanvasRead, hasCanvasWrite := false, false
		for _, p := range permissions {
			hasRead = hasRead || p == "media.read"
			hasCreate = hasCreate || p == "resource.create"
			hasCanvasRead = hasCanvasRead || p == "canvas.read"
			hasCanvasWrite = hasCanvasWrite || p == "canvas.write"
		}
		effects := asArray(op["effects"])
		expectedEffect := "read"
		if execution["adapter"] == "resource.snapshot" {
			expectedEffect = "draft_write"
			hasRead = hasRead && hasCreate
		}
		if execution["adapter"] == "canvas.blueprint.instantiate" {
			expectedEffect = "draft_write"
			hasRead = hasCanvasRead && hasCanvasWrite && op["context"].(map[string]any)["requiresCanvas"] == true
		}
		if !hasRead || len(effects) != 1 || effects[0] != expectedEffect {
			return invalid("scope_forbidden", "adapter minimum contract")
		}
	}
	for _, v := range asArray(contributes["skills"]) {
		for _, a := range v.(map[string]any)["operations"].([]any) {
			address := a.(string)
			parts := strings.Split(address, ".")
			if parts[0] == pluginID {
				if !registry["operations"][parts[1]] {
					return invalid("package_reference_invalid", address)
				}
			} else if !dependencies[parts[0]] {
				return invalid("plugin_dependency_missing", address)
			}
		}
	}
	for _, v := range asArray(contributes["views"]) {
		p := docs[v.(map[string]any)["ref"].(string)]["schemaRef"].(string)
		if _, ok := docs[p]; !ok || !strings.HasPrefix(p, "schemas/") {
			return invalid("package_reference_invalid", p)
		}
	}
	for _, v := range asArray(contributes["canvasBlueprints"]) {
		bp := docs[v.(map[string]any)["ref"].(string)]
		raw, _ := json.Marshal(bp)
		var definition CanvasBlueprint
		if err := json.Unmarshal(raw, &definition); err != nil {
			return err
		}
		if err := ValidateBlueprint(definition, files); err != nil {
			return err
		}
		keys := map[string]bool{}
		for _, n := range bp["nodes"].([]any) {
			node := n.(map[string]any)
			key := node["key"].(string)
			if keys[key] {
				return invalid("contract_invalid", "duplicate blueprint key")
			}
			keys[key] = true
			if !registry["views"][node["view"].(string)] {
				return invalid("package_reference_invalid", "blueprint view")
			}
		}
	}
	return validateUserSchemas(files, docs)
}

func asArray(v any) []any { result, _ := v.([]any); return result }

// PackageDigest hashes exact UTF-8 file bytes, not ZIP metadata or reserialized
// JSON. Names are restricted to ASCII and sorted, making Go/JS order identical.
func PackageDigest(files PackageFiles) string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	h := sha256.New()
	h.Write([]byte("qimu-package-v1\n"))
	for _, name := range names {
		sum := sha256.Sum256(files[name])
		fmt.Fprintf(h, "%s\x00%x\n", name, sum)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func OperationDigest(packageDigest, operationID string) string {
	sum := sha256.Sum256([]byte("qimu-operation-v1\n" + packageDigest + "\n" + operationID + "\n"))
	return hex.EncodeToString(sum[:])
}
