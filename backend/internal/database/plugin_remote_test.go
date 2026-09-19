package database

import (
	"infinite-canvas/backend/internal/model"
	"testing"
	"time"
)

func TestP04MigrationPreservesP03(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			db := p02LineageDatabase(t, driver, "p04")
			if err := MigrateSchema(db); err != nil {
				t.Fatal(err)
			}
			run := model.PluginRun{ID: "p03-run", UserID: "user", IdempotencyKey: "p03-existing", Status: "succeeded", Revision: 3, ResultJSON: `{"result":"kept"}`, ReleaseVersion: "1.3.0"}
			if err := db.Create(&run).Error; err != nil {
				t.Fatal(err)
			}
			for _, table := range []any{&model.PluginConnection{}, &model.PluginConnectionVersion{}, &model.PluginConnectionRate{}, &model.PluginOperationPrice{}, &model.PluginRemoteExecution{}, &model.PluginRunResource{}} {
				if err := db.Migrator().DropTable(table); err != nil {
					t.Fatal(err)
				}
			}
			if err := db.Delete(&schemaMigration{}, "version>=?", 33).Error; err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				if err := MigrateSchema(db); err != nil {
					t.Fatal(err)
				}
			}
			var saved model.PluginRun
			if err := db.First(&saved, "id=?", run.ID).Error; err != nil {
				t.Fatal(err)
			}
			if saved.ResultJSON != run.ResultJSON || saved.Revision != run.Revision || saved.TaskID != nil {
				t.Fatal("P03 result changed")
			}
			for _, table := range []any{&model.PluginConnection{}, &model.PluginConnectionVersion{}, &model.PluginRemoteExecution{}, &model.PluginRunResource{}} {
				if !db.Migrator().HasTable(table) {
					t.Fatalf("missing %T", table)
				}
			}
		})
	}
}

func TestP04MigratesAcceptedPluginLineage(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			db := p02LineageDatabase(t, driver, "accepted-p03")
			if err := db.AutoMigrate(&schemaMigration{}); err != nil {
				t.Fatal(err)
			}
			byName := map[string]migration{}
			for _, m := range schemaMigrations {
				byName[m.name] = m
				if m.version > 27 {
					continue
				}
				if err := m.apply(db); err != nil {
					t.Fatal(err)
				}
				if err := db.Create(&schemaMigration{Version: m.version, Name: m.name, Checksum: m.checksum, AppliedAt: time.Now().UTC()}).Error; err != nil {
					t.Fatal(err)
				}
			}
			at := time.Now().UTC().Truncate(time.Second)
			for index, name := range []string{"application_plugin_releases", "application_plugin_invocations", "application_plugin_projections"} {
				m := byName[name]
				if err := m.apply(db); err != nil {
					t.Fatal(err)
				}
				if err := db.Create(&schemaMigration{Version: int64(28 + index), Name: m.name, Checksum: m.checksum, AppliedAt: at}).Error; err != nil {
					t.Fatal(err)
				}
			}
			original := model.PluginRun{ID: "old-p03", UserID: "user", IdempotencyKey: "old-unique", ReleaseVersion: "1.3.0", Status: "succeeded", Revision: 3, ResultJSON: `{"preserved":true}`}
			if err := db.Create(&original).Error; err != nil {
				t.Fatal(err)
			}
			if err := MigrateSchema(db); err != nil {
				t.Fatal(err)
			}
			var old schemaMigration
			if err := db.First(&old, "version=?", 28).Error; err != nil || old.Name != "application_plugin_releases" || !old.AppliedAt.Equal(at) {
				t.Fatal("rewrote accepted migration", err)
			}
			var added schemaMigration
			if err := db.First(&added, "version=?", 31).Error; err != nil || added.Name != "agent_execution_journal" {
				t.Fatal("missing main journal upgrade", err)
			}
			var saved model.PluginRun
			if err := db.First(&saved, "id=?", original.ID).Error; err != nil || saved.ResultJSON != original.ResultJSON {
				t.Fatal("lost plugin result", err)
			}
			if err := db.Model(&schemaMigration{}).Where("version=?", 28).Update("checksum", "unknown").Error; err != nil {
				t.Fatal(err)
			}
			if err := MigrateSchema(db); err == nil {
				t.Fatal("unknown migration checksum accepted")
			}
		})
	}
}
