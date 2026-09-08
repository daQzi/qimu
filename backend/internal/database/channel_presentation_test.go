package database

import "testing"

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
