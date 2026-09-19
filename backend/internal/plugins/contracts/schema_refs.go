package contracts

import (
	"github.com/santhosh-tekuri/jsonschema/v6"
	"strings"
)

func resolveRef(current, reference string, docs map[string]map[string]any) (string, string, map[string]any, error) {
	parts := strings.Split(reference, "#")
	if len(parts) > 2 {
		return "", "", nil, invalid("package_reference_invalid", reference)
	}
	file := parts[0]
	if file == "" {
		file = current
	}
	if !validPath(file) || !strings.HasPrefix(file, "schemas/") {
		return "", "", nil, invalid("package_reference_invalid", reference)
	}
	pointer := ""
	if len(parts) == 2 {
		pointer = parts[1]
	}
	var value any = docs[file]
	if pointer != "" {
		if !strings.HasPrefix(pointer, "/") {
			return "", "", nil, invalid("package_reference_invalid", reference)
		}
		for _, segment := range strings.Split(pointer[1:], "/") {
			for i := 0; i < len(segment); i++ {
				if segment[i] == '~' && (i+1 == len(segment) || (segment[i+1] != '0' && segment[i+1] != '1')) {
					return "", "", nil, invalid("package_reference_invalid", reference)
				}
			}
			segment = strings.ReplaceAll(strings.ReplaceAll(segment, "~1", "/"), "~0", "~")
			m, ok := value.(map[string]any)
			if !ok {
				return "", "", nil, invalid("package_reference_invalid", reference)
			}
			value = m[segment]
		}
	}
	node, ok := value.(map[string]any)
	if !ok || node == nil {
		return "", "", nil, invalid("package_reference_invalid", reference)
	}
	return file, pointer, node, nil
}

func validateUserSchemas(files PackageFiles, docs map[string]map[string]any) error {
	marks := map[string]int{}
	var visit func(string, string, map[string]any, int) error
	visit = func(file, pointer string, node map[string]any, depth int) error {
		if depth > profile.Limits.JSONDepth {
			return invalid("contract_invalid", "schema reference depth")
		}
		key := file + "#" + pointer
		if marks[key] == 1 {
			return invalid("schema_reference_cycle", key)
		}
		if marks[key] == 2 {
			return nil
		}
		marks[key] = 1
		if reference, ok := node["$ref"].(string); ok {
			f, p, n, err := resolveRef(file, reference, docs)
			if err != nil {
				return err
			}
			if err = compiled["userSchema"].Validate(n); err != nil {
				return invalid("contract_invalid", "schema target")
			}
			if err = visit(f, p, n, depth+1); err != nil {
				return err
			}
		}
		for _, field := range []string{"properties", "$defs"} {
			children, _ := node[field].(map[string]any)
			for name, child := range children {
				escaped := strings.ReplaceAll(strings.ReplaceAll(name, "~", "~0"), "/", "~1")
				if err := visit(file, pointer+"/"+field+"/"+escaped, child.(map[string]any), depth+1); err != nil {
					return err
				}
			}
		}
		if child, ok := node["items"].(map[string]any); ok {
			if err := visit(file, pointer+"/items", child, depth+1); err != nil {
				return err
			}
		}
		marks[key] = 2
		return nil
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	// References are package-root relative, so rewrite only validated $ref
	// values to registered in-memory resources. No URL loader can fetch data.
	var rewrite func(string, map[string]any) map[string]any
	rewrite = func(file string, node map[string]any) map[string]any {
		out := map[string]any{}
		for k, v := range node {
			switch k {
			case "$ref":
				f, p, _, _ := resolveRef(file, v.(string), docs)
				out[k] = packageURL + f + "#" + p
			case "properties", "$defs":
				m := map[string]any{}
				for name, child := range v.(map[string]any) {
					m[name] = rewrite(file, child.(map[string]any))
				}
				out[k] = m
			case "items":
				out[k] = rewrite(file, v.(map[string]any))
			default:
				out[k] = v
			}
		}
		return out
	}
	// Validate all nodes before following references: a forward reference can
	// otherwise reach an unvalidated document and panic on a malformed child.
	for file := range docs {
		if strings.HasPrefix(file, "schemas/") {
			if err := Validate("userSchema", files[file]); err != nil {
				return err
			}
		}
	}
	for file, node := range docs {
		if strings.HasPrefix(file, "schemas/") {
			if err := visit(file, "", node, 0); err != nil {
				return err
			}
		}
	}
	for file, node := range docs {
		if strings.HasPrefix(file, "schemas/") {
			if err := compiler.AddResource(packageURL+file, rewrite(file, node)); err != nil {
				return invalid("contract_invalid", file)
			}
		}
	}
	for file := range docs {
		if strings.HasPrefix(file, "schemas/") {
			if _, err := compiler.Compile(packageURL + file); err != nil {
				return invalid("contract_invalid", file)
			}
		}
	}
	return nil
}

// ValidateDependencyGraph is a pure catalog check. Version resolution,
// publisher ownership and enabled-state admission remain P01/P02 concerns.
func ValidateDependencyGraph(graph map[string][]string) error {
	if len(graph) > 256 {
		return invalid("contract_invalid", "dependency graph limit")
	}
	for _, deps := range graph {
		if len(deps) > 256 {
			return invalid("contract_invalid", "dependency graph limit")
		}
	}
	marks := map[string]int{}
	var visit func(string) error
	visit = func(id string) error {
		if marks[id] == 1 {
			return invalid("dependency_cycle", id)
		}
		if marks[id] == 2 {
			return nil
		}
		next, ok := graph[id]
		if !ok {
			return invalid("plugin_dependency_missing", id)
		}
		marks[id] = 1
		for _, dep := range next {
			if err := visit(dep); err != nil {
				return err
			}
		}
		marks[id] = 2
		return nil
	}
	for id := range graph {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}
