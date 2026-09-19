package database

import (
	"infinite-canvas/backend/internal/model"
	"testing"
)

func TestP05MigrationPreservesRemoteHistory(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			db := p02LineageDatabase(t, driver, "p05")
			if err := MigrateSchema(db); err != nil {
				t.Fatal(err)
			}
			run := model.PluginRun{ID: "remote-history", UserID: "user", IdempotencyKey: "remote-existing", Status: "succeeded", Revision: 8, ResultJSON: `{"message":"kept"}`}
			if err := db.Create(&run).Error; err != nil {
				t.Fatal(err)
			}
			var before schemaMigration
			if err := db.First(&before, "version=?", 33).Error; err != nil {
				t.Fatal(err)
			}
			for _, table := range []any{&model.PluginPipelineExecution{}, &model.PluginInputRequest{}} {
				if err := db.Migrator().DropTable(table); err != nil {
					t.Fatal(err)
				}
			}
			for _, column := range []string{"pipeline_id", "parent_run_id", "parent_step_key"} {
				if err := db.Migrator().DropColumn(&model.PluginRun{}, column); err != nil {
					t.Fatal(err)
				}
			}
			if err := db.Delete(&schemaMigration{}, "version=?", 34).Error; err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				if err := MigrateSchema(db); err != nil {
					t.Fatal(err)
				}
			}
			var after schemaMigration
			db.First(&after, "version=?", 33)
			if before.Checksum != after.Checksum || !before.AppliedAt.Equal(after.AppliedAt) {
				t.Fatal("P04 migration rewritten")
			}
			var saved model.PluginRun
			db.First(&saved, "id=?", run.ID)
			if saved.ResultJSON != run.ResultJSON || saved.Revision != 8 {
				t.Fatal("P04 run changed")
			}
			for _, table := range []any{&model.PluginPipelineExecution{}, &model.PluginInputRequest{}} {
				if !db.Migrator().HasTable(table) {
					t.Fatal("pipeline table missing")
				}
			}
		})
	}
}
