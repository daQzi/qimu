package main

import (
	"bytes"
	"encoding/json"
	"io"
	"path/filepath"
	"testing"
)

func TestCLIWorkflow(t *testing.T) {
	root := t.TempDir()
	dir, archive := filepath.Join(root, "source"), filepath.Join(root, "delivery.yingce-plugin")
	steps := [][]string{
		{"-init", dir, "-template", "resource", "-id", "delivery-check", "-publisher", "studio"},
		{"-dir", dir, "-out", archive, "-version", "1.1.0"},
		{"-package", archive},
	}
	var digest string
	for i, args := range steps {
		var output bytes.Buffer
		if err := run(args, &output, io.Discard); err != nil {
			t.Fatal(err)
		}
		var result map[string]any
		if err := json.Unmarshal(output.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result["valid"] != true || result["runtimeVerified"] != false || result["runtimeEnabled"] != nil {
			t.Fatal(result)
		}
		if i == 1 {
			digest = result["packageDigest"].(string)
		}
		if i == 2 && digest != result["packageDigest"] {
			t.Fatal("archive digest mismatch")
		}
	}
	for _, args := range [][]string{
		{}, {"-dir", dir, "-package", archive}, {"-package", archive, "-version", "2.0.0"},
		{"-dir", dir, "-version", "2.0.0"}, {"-dir", dir, "-publisher", "ignored"},
		{"-dir", dir, "-out", archive}, {"-init", dir, "-id", "delivery-check", "-publisher", "studio"},
	} {
		if err := run(args, io.Discard, io.Discard); err == nil {
			t.Fatal("invalid arguments accepted", args)
		}
	}
}

func TestCapabilityAndInspectionCLI(t *testing.T) {
	var output bytes.Buffer
	if err := run([]string{"-capabilities"}, &output, io.Discard); err != nil {
		t.Fatal(err)
	}
	var capabilities map[string]any
	if err := json.Unmarshal(output.Bytes(), &capabilities); err != nil || capabilities["runtimeVerified"] != false {
		t.Fatal(output.String(), err)
	}
	root := t.TempDir()
	dir := filepath.Join(root, "review")
	if err := run([]string{"-init", dir, "-template", "model-review", "-id", "custom-review", "-publisher", "studio"}, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := run([]string{"-dir", dir, "-inspect"}, &output, io.Discard); err != nil {
		t.Fatal(err)
	}
	var report struct {
		ID                    string
		RuntimeVerified       bool
		Operations            []any
		RequiredRuntimeChecks []string
	}
	if err := json.Unmarshal(output.Bytes(), &report); err != nil || report.ID != "custom-review" || report.RuntimeVerified || len(report.Operations) != 2 || len(report.RequiredRuntimeChecks) < 4 {
		t.Fatal(output.String(), err)
	}
	for _, args := range [][]string{{"-capabilities", "-dir", dir}, {"-capabilities", "extra"}, {"-dir", dir, "-inspect", "-out", filepath.Join(root, "unused.zip")}, {"-init", filepath.Join(root, "unused"), "-inspect", "-id", "custom", "-publisher", "studio"}} {
		if err := run(args, io.Discard, io.Discard); err == nil {
			t.Fatal("invalid combination accepted", args)
		}
	}
}
