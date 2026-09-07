package service

import (
	"encoding/json"
	"testing"

	"infinite-canvas/backend/internal/model"
)

func TestXiaoyunquePresetBackfillAndRoundTrip(t *testing.T) {
	svc, db, _, admin := newAppearanceTestService(t)
	value := defaultAppearanceSetting()
	value.SchemaVersion = 6
	value.SkinID = "brand-violet"
	value.SkinThemes = value.SkinThemes[:4]
	value.SkinThemes[3].Tokens.Light.Primary = "#123456"
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.SystemSetting{Key: appearanceSettingKey, ValueJSON: string(encoded)}).Error; err != nil {
		t.Fatal(err)
	}
	loaded, err := svc.AdminAppearance(admin)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.SkinThemes) != 5 || loaded.SkinID != "brand-violet" || loaded.Public.ActiveSkin.Tokens.Light.Primary != "#123456" {
		t.Fatalf("backfill changed existing choice: %#v", loaded)
	}
	loaded.SkinID = "xiaoyunque"
	updated, err := svc.UpdateAppearance(admin, loaded.AppearanceSetting)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Public.ActiveSkin.Tokens.Light.Primary != "#8359ff" || updated.Public.ActiveSkin.Tokens.Components.ShadowStyle != "brand" {
		t.Fatalf("preset did not survive save: %#v", updated.Public.ActiveSkin)
	}
	// Re-reading must not duplicate the preset or replace customized parameters.
	updated.SkinThemes[4].Tokens.Light.Primary = "#654321"
	if _, err := svc.UpdateAppearance(admin, updated.AppearanceSetting); err != nil {
		t.Fatal(err)
	}
	reloaded, err := svc.AdminAppearance(admin)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.SkinThemes) != 5 || reloaded.Public.ActiveSkin.Tokens.Light.Primary != "#654321" {
		t.Fatalf("customization was lost: %#v", reloaded)
	}
	reloaded.SkinID = defaultAppearanceSkinID
	reloaded.SkinThemes = reloaded.SkinThemes[:4]
	if _, err := svc.UpdateAppearance(admin, reloaded.AppearanceSetting); err != nil {
		t.Fatal(err)
	}
	deleted, err := svc.AdminAppearance(admin)
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted.SkinThemes) != 4 {
		t.Fatal("deleted preset was restored after saving the upgraded configuration")
	}
}
