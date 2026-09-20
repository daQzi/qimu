package contracts

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var composerFieldName = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]{0,79}$`)

// Workbench configures existing entry points; it never grants execution authority.
type Workbench struct {
	ObjectInputs     map[string]string `json:"objectInputs,omitempty"`
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Description      string            `json:"description"`
	HostSurfaces     []string          `json:"hostSurfaces"`
	ContextSchemaRef string            `json:"contextSchemaRef"`
	Operation        string            `json:"operation,omitempty"`
	Skill            string            `json:"skill,omitempty"`
	Recipes          []string          `json:"recipes"`
	Defaults         map[string]any    `json:"defaults"`
	Output           string            `json:"output"`
	Prompt           string            `json:"prompt,omitempty"`
}
type Recipe struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	Defaults     map[string]any `json:"defaults"`
	Requirements map[string]any `json:"requirements"`
	Prompt       string         `json:"prompt,omitempty"`
}
type WorkbenchSelection struct {
	ID        string         `json:"id"`
	ReleaseID string         `json:"releaseId"`
	RecipeIDs []string       `json:"recipeIds"`
	Input     map[string]any `json:"input"`
	Digest    string         `json:"digest"`
}
type RecipeConflict struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}
type WorkbenchComposition struct {
	Input     map[string]any   `json:"input"`
	Prompts   []string         `json:"prompts"`
	Conflicts []RecipeConflict `json:"conflicts"`
}

func WorkbenchDefinitions(files PackageFiles) (Manifest, map[string]Workbench, map[string]Recipe, error) {
	var manifest Manifest
	boards, recipes := map[string]Workbench{}, map[string]Recipe{}
	if err := json.Unmarshal(files["manifest.json"], &manifest); err != nil {
		return manifest, nil, nil, err
	}
	for _, ref := range manifest.Contributes.Workbenches {
		var board Workbench
		if err := json.Unmarshal(files[ref.Ref], &board); err != nil {
			return manifest, nil, nil, err
		}
		boards[ref.ID] = board
	}
	for _, ref := range manifest.Contributes.Recipes {
		var recipe Recipe
		if err := json.Unmarshal(files[ref.Ref], &recipe); err != nil {
			return manifest, nil, nil, err
		}
		recipes[ref.ID] = recipe
	}
	return manifest, boards, recipes, nil
}

func ValidateWorkbenches(files PackageFiles) error {
	manifest, boards, recipes, err := WorkbenchDefinitions(files)
	if err != nil {
		return err
	}
	ops := map[string]Operation{}
	skills := map[string]Skill{}
	extended := len(boards)+len(recipes) > 0
	for _, ref := range manifest.Contributes.Operations {
		var op Operation
		if err := json.Unmarshal(files[ref.Ref], &op); err != nil {
			return err
		}
		ops[manifest.ID+"."+ref.ID] = op
	}
	for _, skill := range manifest.Contributes.Skills {
		skills[skill.ID] = skill
		if skill.LaunchOperation != "" {
			extended = true
			if _, ok := ops[skill.LaunchOperation]; !ok {
				return invalid("package_reference_invalid", "skill launch operation")
			}
			found := false
			for _, address := range skill.Operations {
				found = found || address == skill.LaunchOperation
			}
			if !found {
				return invalid("scope_forbidden", "skill launch must be declared")
			}
		}
	}
	if extended && manifest.Requires.HostAPI != "^3.1.0" && manifest.Requires.HostAPI != "^3.2.0" && manifest.Requires.HostAPI != "^3.3.0" {
		return invalid("contract_invalid", "workbench requires hostApi ^3.1.0")
	}
	for _, board := range boards {
		if !strings.HasPrefix(board.ContextSchemaRef, "schemas/") || files[board.ContextSchemaRef] == nil {
			return invalid("package_reference_invalid", "workbench context schema")
		}
		if board.Operation != "" {
			op, ok := ops[board.Operation]
			if !ok || op.InputSchemaRef != board.ContextSchemaRef {
				return invalid("package_reference_invalid", "workbench must use local operation input schema")
			}
		}
		if board.Skill != "" {
			skill, ok := skills[board.Skill]
			if !ok {
				return invalid("package_reference_invalid", "workbench skill")
			}
			if skill.LaunchOperation != "" && skill.LaunchOperation != board.Operation {
				return invalid("contract_invalid", "workbench skill launch mismatch")
			}
		}
		suggestions := []map[string]any{board.Defaults}
		for _, id := range board.Recipes {
			recipe, ok := recipes[id]
			if !ok {
				return invalid("package_reference_invalid", "workbench recipe")
			}
			suggestions = append(suggestions, recipe.Defaults, recipe.Requirements)
		}
		var schema map[string]any
		if err := json.Unmarshal(files[board.ContextSchemaRef], &schema); err != nil {
			return err
		}
		// The first Composer contract is an explicit object, not another form schema.
		if schema["type"] != "object" || schema["$ref"] != nil {
			return invalid("contract_invalid", "workbench requires object schema")
		}
		properties, _ := schema["properties"].(map[string]any)
		if len(board.ObjectInputs) > 0 {
			if manifest.Requires.HostAPI != "^3.2.0" && manifest.Requires.HostAPI != "^3.3.0" {
				return invalid("contract_invalid", "object inputs require hostApi ^3.2.0")
			}
			allowed := false
			for _, p := range manifest.Permissions {
				allowed = allowed || p == "asset.read"
			}
			if !allowed {
				return invalid("scope_forbidden", "object inputs require asset.read")
			}
			for key := range board.ObjectInputs {
				if properties[key] == nil {
					return invalid("package_reference_invalid", "object input field")
				}
				required := false
				for _, field := range asArray(schema["required"]) {
					required = required || field == key
				}
				if !required {
					return invalid("contract_invalid", "object input must be required")
				}
			}
		}
		if len(properties) > 32 {
			return invalid("contract_invalid", "workbench exceeds 32 fields")
		}
		for key := range properties {
			if !composerFieldName.MatchString(key) {
				return invalid("contract_invalid", "workbench field name")
			}
		}
		for _, values := range suggestions {
			for key, value := range values {
				if properties[key] == nil {
					return invalid("contract_invalid", "unknown recipe field: "+key)
				}
				if err := ValidateData(files, board.ContextSchemaRef+"#/properties/"+key, mustJSON(value)); err != nil {
					return invalid("contract_invalid", "recipe field: "+key)
				}
			}
		}
	}
	return nil
}
func mustJSON(v any) []byte   { raw, _ := json.Marshal(v); return raw }
func sameValue(a, b any) bool { return string(mustJSON(a)) == string(mustJSON(b)) }

// User values override suggestions, but never resolve contradictory requirements.
// Multiple suggestions are intentionally order independent: disagreement is visible.
func ComposeWorkbench(board Workbench, recipes map[string]Recipe, selected []string, edits map[string]any) (WorkbenchComposition, error) {
	result := WorkbenchComposition{Input: map[string]any{}, Prompts: []string{}, Conflicts: []RecipeConflict{}}
	for k, v := range board.Defaults {
		result.Input[k] = v
	}
	if board.Prompt != "" {
		result.Prompts = append(result.Prompts, board.Prompt)
	}
	allowed, seen := map[string]bool{}, map[string]bool{}
	for _, id := range board.Recipes {
		allowed[id] = true
	}
	ids := append([]string{}, selected...)
	sort.Strings(ids)
	defaults, requirements := map[string]any{}, map[string]any{}
	addConflict := func(k, reason string) { result.Conflicts = append(result.Conflicts, RecipeConflict{k, reason}) }
	for _, id := range ids {
		recipe, ok := recipes[id]
		if !allowed[id] || !ok || seen[id] {
			return result, fmt.Errorf("invalid recipe selection: %s", id)
		}
		seen[id] = true
		if recipe.Prompt != "" {
			result.Prompts = append(result.Prompts, recipe.Prompt)
		}
		for k, v := range recipe.Defaults {
			if previous, exists := defaults[k]; exists && !sameValue(previous, v) {
				if _, userSet := edits[k]; !userSet {
					addConflict(k, "配方建议不同，请显式选择该字段")
				}
			}
			defaults[k] = v
		}
		for k, v := range recipe.Requirements {
			if previous, exists := requirements[k]; exists && !sameValue(previous, v) {
				addConflict(k, "配方要求互斥，请取消其中一个配方")
			}
			requirements[k] = v
		}
	}
	for k, v := range defaults {
		result.Input[k] = v
	}
	for k, v := range requirements {
		result.Input[k] = v
	}
	for k, v := range edits {
		if required, exists := requirements[k]; exists && !sameValue(required, v) {
			addConflict(k, "用户输入不符合已选配方要求")
		}
		result.Input[k] = v
	}
	sort.Slice(result.Conflicts, func(i, j int) bool {
		return result.Conflicts[i].Field+result.Conflicts[i].Message < result.Conflicts[j].Field+result.Conflicts[j].Message
	})
	return result, nil
}
