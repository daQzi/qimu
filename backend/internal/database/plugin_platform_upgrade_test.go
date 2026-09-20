package database

import (
	"infinite-canvas/backend/internal/model"
	"testing"
	"time"
)

func TestP14MainAndWorkstudioUpgrade(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		for _, lineage := range []string{"main31", "workstudio37"} {
			t.Run(driver+"/"+lineage, func(t *testing.T) {
				db := p02LineageDatabase(t, driver, lineage)
				sql, _ := db.DB()
				t.Cleanup(func() { sql.Close() })
				if err := db.AutoMigrate(&schemaMigration{}); err != nil {
					t.Fatal(err)
				}
				before := []schemaMigration{}
				for _, item := range schemaMigrations {
					if lineage == "main31" && item.version > 31 {
						continue
					}
					if lineage == "workstudio37" {
						if item.version == 30 || item.version == 31 {
							continue
						}
						if item.version >= 32 {
							item.version -= 2
						}
					}
					if err := item.apply(db); err != nil {
						t.Fatal(err)
					}
					row := schemaMigration{Version: item.version, Name: item.name, Checksum: item.checksum, AppliedAt: time.Now().UTC()}
					if err := db.Create(&row).Error; err != nil {
						t.Fatal(err)
					}
					before = append(before, row)
				}
				if lineage == "workstudio37" {
					if err := db.Create(&model.PluginRun{ID: "retained", UserID: "user", IdempotencyKey: "retained", Status: "waiting_input", Revision: 9}).Error; err != nil {
						t.Fatal(err)
					}
				}
				for i := 0; i < 2; i++ {
					if err := MigrateSchema(db); err != nil {
						t.Fatal(err)
					}
				}
				for _, row := range before {
					var got schemaMigration
					if err := db.First(&got, "version=?", row.Version).Error; err != nil {
						t.Fatal(err)
					}
					if got.Name != row.Name || got.Checksum != row.Checksum || !got.AppliedAt.Equal(row.AppliedAt) {
						t.Fatalf("historical record changed: %+v", got)
					}
				}
				for _, table := range []any{&model.Tool{}, &model.ToolFavorite{}, &model.PluginRun{}, &model.BusinessObjectVersion{}} {
					if !db.Migrator().HasTable(table) {
						t.Fatalf("missing %T", table)
					}
				}
				if lineage == "workstudio37" {
					var run model.PluginRun
					if err := db.First(&run, "id=?", "retained").Error; err != nil || run.Revision != 9 || run.Status != "waiting_input" {
						t.Fatal("plugin run not preserved", err)
					}
				}
				if err := RequireSchemaVersion(db); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
