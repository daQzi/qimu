package database

import (
	"infinite-canvas/backend/internal/model"
	"testing"
)

func TestP10MigrationPreservesWorkbenchState(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			db := p02LineageDatabase(t, driver, "p10")
			if err := MigrateSchema(db); err != nil {
				t.Fatal(err)
			}
			previous := schemaMigration{}
			if err := db.First(&previous, "name = ?", "agent_threads").Error; err != nil {
				t.Fatal(err)
			}
			run := model.PluginRun{ID: "p10-kept-run", UserID: "user", IdempotencyKey: "keep", Status: "waiting_input", Revision: 8}
			if err := db.Create(&run).Error; err != nil {
				t.Fatal(err)
			}
			for _, table := range []any{&model.BusinessObjectVersion{}, &model.BusinessObject{}} {
				if err := db.Migrator().DropTable(table); err != nil {
					t.Fatal(err)
				}
			}
			if err := db.Delete(&schemaMigration{}, "name = ?", "business_objects").Error; err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				if err := MigrateSchema(db); err != nil {
					t.Fatal(err)
				}
			}
			var after schemaMigration
			if err := db.First(&after, "name = ?", "agent_threads").Error; err != nil {
				t.Fatal(err)
			}
			if previous.Checksum != after.Checksum || !previous.AppliedAt.Equal(after.AppliedAt) {
				t.Fatal("changed historical migration")
			}
			var saved model.PluginRun
			if err := db.First(&saved, "id = ?", run.ID).Error; err != nil {
				t.Fatal(err)
			}
			if saved.Status != run.Status || saved.Revision != 8 {
				t.Fatal("changed plugin execution")
			}
			if !db.Migrator().HasIndex(&model.BusinessObjectVersion{}, "idx_object_version_key") {
				t.Fatal("missing idempotency index")
			}
			if err := RequireSchemaVersion(db); err != nil {
				t.Fatal(err)
			}
		})
	}
}
