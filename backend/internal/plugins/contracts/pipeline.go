package contracts

import (
	"regexp"
	"strings"
)

var bindingPattern = regexp.MustCompile(`^(input#|item#|steps/[a-z][a-z0-9-]*#)(/.*)?$`)

// validatePipeline checks a frozen DAG contract only; no step is scheduled.
func validatePipeline(doc map[string]any) error {
	steps := map[string]map[string]any{}
	graph := map[string][]string{}
	for _, v := range doc["steps"].([]any) {
		s := v.(map[string]any)
		key := s["key"].(string)
		if _, ok := steps[key]; ok {
			return invalid("contract_invalid", "duplicate step")
		}
		steps[key] = s
		graph[key] = []string{}
		for _, dep := range s["dependsOn"].([]any) {
			graph[key] = append(graph[key], dep.(string))
		}
	}
	if err := ValidateDependencyGraph(graph); err != nil {
		return err
	}
	check := func(reference string, allowed map[string]bool, allowItem bool) error {
		if !bindingPattern.MatchString(reference) {
			return invalid("contract_invalid", "binding syntax")
		}
		pointer := strings.SplitN(reference, "#", 2)[1]
		for i := 0; i < len(pointer); i++ {
			if pointer[i] == '~' && (i+1 == len(pointer) || (pointer[i+1] != '0' && pointer[i+1] != '1')) {
				return invalid("contract_invalid", "binding pointer")
			}
		}
		if strings.HasPrefix(reference, "item#") && !allowItem {
			return invalid("contract_invalid", "item outside foreach")
		}
		if strings.HasPrefix(reference, "steps/") {
			key := strings.SplitN(strings.TrimPrefix(reference, "steps/"), "#", 2)[0]
			if !allowed[key] {
				return invalid("package_reference_invalid", "undeclared step dependency")
			}
		}
		return nil
	}
	for key, s := range steps {
		allowed := map[string]bool{}
		for _, dep := range graph[key] {
			allowed[dep] = true
		}
		_, hasItems := s["foreach"]
		if loop, ok := s["foreach"].(map[string]any); ok {
			if err := check("item#"+loop["itemKey"].(string), allowed, true); err != nil {
				return err
			}
			if err := check(loop["from"].(string), allowed, false); err != nil {
				return err
			}
		}
		var bindings []map[string]any
		inputs, _ := s["inputs"].(map[string]any)
		for _, v := range inputs {
			bindings = append(bindings, v.(map[string]any))
		}
		if condition, ok := s["when"].(map[string]any); ok {
			if ref, ok := condition["exists"].(string); ok {
				if err := check(ref, allowed, hasItems); err != nil {
					return err
				}
			}
			for _, v := range asArray(condition["equals"]) {
				bindings = append(bindings, v.(map[string]any))
			}
		}
		for _, b := range bindings {
			if from, ok := b["from"].(string); ok {
				if err := check(from, allowed, hasItems); err != nil {
					return err
				}
			}
		}
	}
	all := map[string]bool{}
	for k := range steps {
		all[k] = true
	}
	for _, v := range doc["outputs"].(map[string]any) {
		if from, ok := v.(map[string]any)["from"].(string); ok {
			if err := check(from, all, false); err != nil {
				return err
			}
		}
	}
	return nil
}
