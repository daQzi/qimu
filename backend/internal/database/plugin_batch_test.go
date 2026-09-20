package database

import (
	"infinite-canvas/backend/internal/model"
	"testing"
)

func TestP06MigrationPreservesPipelineState(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			db := p02LineageDatabase(t, driver, "p06")
			if err := MigrateSchema(db); err != nil {
				t.Fatal(err)
			}
			run := model.PluginRun{ID: "parent", UserID: "user", IdempotencyKey: "history", PipelineID: "process", Status: "waiting_input", Revision: 9}
			if err := db.Create(&run).Error; err != nil {
				t.Fatal(err)
			}
			state := model.PluginPipelineExecution{RunID: run.ID, Cursor: 1, OutputsJSON: `{"inspect":{"name":"kept"}}`}
			if err := db.Create(&state).Error; err != nil {
				t.Fatal(err)
			}
			var previous schemaMigration
			db.First(&previous, "version=?", 34)
			for _, table := range []any{&model.PluginBatchApproval{}, &model.PluginExecutionSlot{}, &model.PluginBatchMetric{}} {
				if err := db.Migrator().DropTable(table); err != nil {
					t.Fatal(err)
				}
			}
			for _, col := range []string{"derived_from_run_id", "attempt"} {
				if err := db.Migrator().DropColumn(&model.PluginRun{}, col); err != nil {
					t.Fatal(err)
				}
			}
			if err := db.Migrator().DropColumn(&model.PluginPipelineExecution{}, "batch_json"); err != nil {
				t.Fatal(err)
			}
			if driver == "postgres" {
				if err := db.Exec("ALTER TABLE plugin_remote_executions ALTER COLUMN cancel_status TYPE varchar(32)").Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := db.Delete(&schemaMigration{}, "version=?", 35).Error; err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				if err := MigrateSchema(db); err != nil {
					t.Fatal(err)
				}
			}
			var after schemaMigration
			db.First(&after, "version=?", 34)
			if previous.Checksum != after.Checksum || !previous.AppliedAt.Equal(after.AppliedAt) {
				t.Fatal("P05 migration changed")
			}
			var saved model.PluginPipelineExecution
			db.First(&saved, "run_id=?", run.ID)
			if saved.Cursor != 1 || saved.OutputsJSON != state.OutputsJSON || saved.BatchJSON != "" {
				t.Fatal("legacy state changed")
			}
			var parent model.PluginRun
			db.First(&parent, "id=?", run.ID)
			if parent.Revision != 9 || parent.Attempt != 1 {
				t.Fatal("history or default attempt", parent)
			}
			remote := model.PluginRemoteExecution{TaskID: "remote-task", RunID: "remote-run", UserID: "user", ConnectionVersionID: "connection", SubmissionKey: "submission", Attempt: 1, State: "cancelled", CancelStatus: "unsupported_external_may_continue"}
			if err := db.Create(&remote).Error; err != nil {
				t.Fatal("cancel status widening", err)
			}
		})
	}
}
