package database

import (
	"fmt"
	"gorm.io/gorm"
	"infinite-canvas/backend/internal/model"
	"os"
	"strings"
	"testing"
	"time"
)

func TestP01PluginMigration(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			config := Config{Driver: driver, DSN: fmt.Sprintf("file:p01_%d?mode=memory&cache=shared", time.Now().UnixNano())}
			if driver == "postgres" {
				dsn := os.Getenv("CANVAS_TEST_POSTGRES_DSN")
				if strings.TrimSpace(dsn) == "" {
					t.Skip("isolated PostgreSQL DSN not configured")
				}
				base, e := Open(Config{Driver: "postgres", DSN: dsn})
				if e != nil {
					t.Fatal(e)
				}
				name := fmt.Sprintf("p01_migrate_%d", time.Now().UnixNano())
				if e = base.Exec(`CREATE SCHEMA "` + name + `"`).Error; e != nil {
					t.Fatal(e)
				}
				t.Cleanup(func() { base.Exec(`DROP SCHEMA "` + name + `" CASCADE`); sql, _ := base.DB(); sql.Close() })
				config.DSN, e = postgresDSNWithSearchPath(dsn, name)
				if e != nil {
					t.Fatal(e)
				}
			}
			db, e := Open(config)
			if e != nil {
				t.Fatal(e)
			}
			sql, _ := db.DB()
			t.Cleanup(func() { sql.Close() })
			type oldState struct {
				ID        string `gorm:"primaryKey;size:36"`
				UserID    string `gorm:"size:36;uniqueIndex:idx_user_plugin_state_user_plugin,priority:1"`
				PluginID  string `gorm:"size:120;uniqueIndex:idx_user_plugin_state_user_plugin,priority:2"`
				Enabled   bool
				CreatedAt time.Time
				UpdatedAt time.Time
			}
			if e = db.Table("user_plugin_states").AutoMigrate(&oldState{}); e != nil {
				t.Fatal(e)
			}
			if e = db.Table("user_plugin_states").Create(&oldState{ID: "old", UserID: "alice", PluginID: "legacy", Enabled: true}).Error; e != nil {
				t.Fatal(e)
			}
			for i := 0; i < 2; i++ {
				if e = db.Transaction(func(tx *gorm.DB) error { return migrateApplicationPlugins(tx) }); e != nil {
					t.Fatal(e)
				}
			}
			var row model.UserPluginState
			if e = db.First(&row, "id=?", "old").Error; e != nil {
				t.Fatal(e)
			}
			if !row.Enabled || row.InstalledReleaseID != "" || row.Revision != 0 {
				t.Fatalf("legacy state changed: %+v", row)
			}
			for _, v := range []any{&model.PluginApplication{}, &model.PluginRelease{}, &model.PluginSkillBinding{}, &model.PluginReleaseDependency{}, &model.PluginCatalogLock{}, &model.PluginNamespace{}} {
				if !db.Migrator().HasTable(v) {
					t.Fatalf("missing table %T", v)
				}
			}
			if !db.Migrator().HasIndex(&model.PluginRelease{}, "idx_plugin_release_version") {
				t.Fatal("missing immutable release index")
			}
		})
	}
}
