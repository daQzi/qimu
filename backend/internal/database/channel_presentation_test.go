package database

import (
	"reflect"
	"strings"
	"testing"
)

func TestChannelPresentationRejectsUpstreamHistoryWithoutRewritingIt(t *testing.T) {
	db, err := Open(Config{Driver: "sqlite", DSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	if err := MigrateSchema(db); err != nil {
		t.Fatal(err)
	}
	// 模拟来自上游的另一套迁移历史，不能把同版本号视为本 fork 已升级。
	for _, record := range []schemaMigration{
		{Version: 9, Name: "channel_presentation", Checksum: "sha256:channel-presentation-v9-20260908"},
		{Version: 10, Name: "creation_runtime", Checksum: "sha256:creation-runtime-v10-20260909"},
	} {
		if err := db.Model(&schemaMigration{}).Where("version = ?", record.Version).Updates(map[string]any{"name": record.Name, "checksum": record.Checksum}).Error; err != nil {
			t.Fatal(err)
		}
	}
	var before, after []schemaMigration
	if err := db.Order("version").Find(&before).Error; err != nil {
		t.Fatal(err)
	}
	for _, check := range []func() error{func() error { return MigrateSchema(db) }, func() error { return RequireSchemaVersion(db) }} {
		if err := check(); err == nil || !strings.Contains(err.Error(), "迁移 9 名称不一致") {
			t.Fatalf("unexpected history validation: %v", err)
		}
	}
	if err := db.Order("version").Find(&after).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("rejected history was rewritten")
	}
}

func TestChannelPresentationUpgradePreservesCreationV9(t *testing.T) {
	db, err := Open(Config{Driver: "sqlite", DSN: "file:creation-v9-upgrade?mode=memory&cache=shared"})
	if err != nil {
		t.Fatal(err)
	}
	if err := MigrateSchema(db); err != nil {
		t.Fatal(err)
	}
	// Recreate the local v9 boundary before channel presentation existed.
	for _, sql := range []string{
		`DELETE FROM schema_migrations WHERE version = 10`,
		`ALTER TABLE model_channels DROP COLUMN public_alias`,
		`ALTER TABLE model_channels DROP COLUMN sort_order`,
		`ALTER TABLE channel_models DROP COLUMN sort_order`,
	} {
		if err := db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	var before schemaMigration
	if err := db.First(&before, "version = ?", 9).Error; err != nil {
		t.Fatal(err)
	}
	if before.Name != "creation_run_admission" || before.Checksum != creationRunAdmissionChecksum {
		t.Fatalf("local v9 identity changed: %#v", before)
	}
	for i := 0; i < 2; i++ {
		if err := MigrateSchema(db); err != nil {
			t.Fatal(err)
		}
	}
	var after schemaMigration
	if err := db.First(&after, "version = ?", 9).Error; err != nil {
		t.Fatal(err)
	}
	if before.Name != after.Name || before.Checksum != after.Checksum || !before.AppliedAt.Equal(after.AppliedAt) {
		t.Fatal("upgrade rewrote the existing creation migration")
	}
	var added schemaMigration
	if err := db.First(&added, "version = ?", 10).Error; err != nil {
		t.Fatal(err)
	}
	if added.Name != "channel_presentation" {
		t.Fatalf("unexpected v10: %#v", added)
	}
	for _, item := range []struct{ table, column string }{
		{"model_channels", "public_alias"}, {"model_channels", "sort_order"}, {"channel_models", "sort_order"},
	} {
		if !db.Migrator().HasColumn(item.table, item.column) {
			t.Fatalf("missing %s.%s", item.table, item.column)
		}
	}
	if err := RequireSchemaVersion(db); err != nil {
		t.Fatal(err)
	}
}

func TestChannelPresentationMigrationPreservesExistingRows(t *testing.T) {
	db, err := Open(Config{Driver: "sqlite", DSN: "file:channel-presentation-migration?mode=memory&cache=shared"})
	if err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		`CREATE TABLE model_channels (id TEXT PRIMARY KEY, name TEXT, api_key TEXT)`,
		`CREATE TABLE channel_models (id TEXT PRIMARY KEY, price_version INTEGER, unit_price_microcredits INTEGER)`,
		`INSERT INTO model_channels VALUES ('existing', '原渠道', 'synthetic-key')`,
		`INSERT INTO channel_models VALUES ('existing', 7, 12345)`,
	} {
		if err := db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		if err := migrateChannelPresentation(db); err != nil {
			t.Fatal(err)
		}
	}
	var channel struct {
		Name, APIKey, PublicAlias string
		SortOrder                 int
	}
	if err := db.Table("model_channels").First(&channel).Error; err != nil {
		t.Fatal(err)
	}
	if channel.Name != "原渠道" || channel.APIKey != "synthetic-key" || channel.PublicAlias != "" || channel.SortOrder != 0 {
		t.Fatalf("channel migration: %#v", channel)
	}
	var cm struct{ PriceVersion, UnitPriceMicrocredits, SortOrder int }
	if err := db.Table("channel_models").First(&cm).Error; err != nil {
		t.Fatal(err)
	}
	if cm.PriceVersion != 7 || cm.UnitPriceMicrocredits != 12345 || cm.SortOrder != 0 {
		t.Fatalf("model migration: %#v", cm)
	}
}
