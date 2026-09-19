package contracts_test

import (
	"encoding/json"
	"os"
	"testing"

	"infinite-canvas/backend/internal/protocol"
)

func TestP00DoesNotEnableLiveManifest(t *testing.T) {
	raw, err := os.ReadFile("testdata/resource-helper/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var old protocol.Manifest
	if err = json.Unmarshal(raw, &old); err != nil {
		t.Fatal(err)
	}
	if err = protocol.ValidateManifest(old); err == nil {
		t.Fatal("offline v3 leaked into live installer")
	}
	raw, err = os.ReadFile("profile.json")
	if err != nil {
		t.Fatal(err)
	}
	var profile struct {
		RuntimeEnabled bool `json:"runtimeEnabled"`
	}
	if err = json.Unmarshal(raw, &profile); err != nil {
		t.Fatal(err)
	}
	if profile.RuntimeEnabled {
		t.Fatal("P00 cannot enable runtime")
	}
}
