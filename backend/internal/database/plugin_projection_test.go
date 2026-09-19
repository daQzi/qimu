package database

import (
	"infinite-canvas/backend/internal/model"
	"testing"
)

func TestP03ProjectionMigrationPreservesResults(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			db := p02LineageDatabase(t, driver, "p03")
			if err := MigrateSchema(db); err != nil {
				t.Fatal(err)
			}
			run := model.PluginRun{ID: "saved", UserID: "user", ReleaseID: "release", ReleaseVersion: "1.2.0", Operation: "resource-helper.snapshot-video", IdempotencyKey: "existing-key", RequestDigest: "old-digest", ContractHash: "old-contract", Status: "succeeded", Revision: 3, ResultJSON: `{"resourceId":"video"}`, SourceResourceID: "video"}
			if err := db.Create(&run).Error; err != nil {
				t.Fatal(err)
			}
			// Recreate the P02 shape in an isolated schema, then run the real upgrade.
			for _, column := range []string{"projection_status", "failure_message"} {
				if err := db.Migrator().DropColumn(&model.PluginRun{}, column); err != nil {
					t.Fatal(err)
				}
			}
			if err := db.Migrator().DropTable(&model.PluginCanvasProjection{}); err != nil {
				t.Fatal(err)
			}
			if err := db.Delete(&schemaMigration{}, "version>=?", 30).Error; err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				if err := MigrateSchema(db); err != nil {
					t.Fatal(err)
				}
			}
			var got model.PluginRun
			if err := db.First(&got, "id=?", run.ID).Error; err != nil {
				t.Fatal(err)
			}
			if got.ResultJSON != run.ResultJSON || got.SourceResourceID != "video" || got.Revision != 3 {
				t.Fatal("P02 result changed during upgrade")
			}
			if !db.Migrator().HasTable(&model.PluginCanvasProjection{}) || !db.Migrator().HasColumn(&model.PluginRun{}, "projection_status") {
				t.Fatal("projection schema missing")
			}
		})
	}
}
