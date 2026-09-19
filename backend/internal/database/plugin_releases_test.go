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

func TestP02MigrationAcceptsMainAndP01Version27Lineages(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		for _, lineage := range []string{"main-cost", "p01-plugin"} {
			t.Run(driver+"/"+lineage, func(t *testing.T) {
				db := p02LineageDatabase(t, driver, lineage)
				var err error
				if err != nil {
					t.Fatal(err)
				}
				sqlDB, _ := db.DB()
				t.Cleanup(func() { _ = sqlDB.Close() })
				if err = db.AutoMigrate(&schemaMigration{}); err != nil {
					t.Fatal(err)
				}
				for _, item := range schemaMigrations[:26] {
					if err = item.apply(db); err != nil {
						t.Fatalf("apply prefix %d: %v", item.version, err)
					}
					if err = db.Create(&schemaMigration{Version: item.version, Name: item.name, Checksum: item.checksum, AppliedAt: time.Now().UTC()}).Error; err != nil {
						t.Fatal(err)
					}
				}
				if lineage == "main-cost" {
					item := schemaMigrations[26]
					if err = item.apply(db); err != nil {
						t.Fatal(err)
					}
					if err = db.Create(&schemaMigration{Version: 27, Name: item.name, Checksum: item.checksum, AppliedAt: time.Now().UTC()}).Error; err != nil {
						t.Fatal(err)
					}
				} else {
					if err = migrateApplicationPlugins(db); err != nil {
						t.Fatal(err)
					}
					if err = db.Create(&model.PluginApplication{ID: "preserved", PublisherID: "publisher", Installed: true, Available: true, Revision: 1}).Error; err != nil {
						t.Fatal(err)
					}
					if err = db.Create(&schemaMigration{Version: 27, Name: "application_plugin_releases", Checksum: "sha256:application-plugin-releases-v27", AppliedAt: time.Now().UTC()}).Error; err != nil {
						t.Fatal(err)
					}
				}
				if err = MigrateSchema(db); err != nil {
					t.Fatal(err)
				}
				status, err := ReadSchemaStatus(db)
				if err != nil || !status.Ready || status.Current != CurrentSchemaVersion {
					t.Fatalf("status=%+v err=%v", status, err)
				}
				for _, table := range []any{&model.PluginApplication{}, &model.PluginRun{}, &model.PluginRunStep{}, &model.PluginRunEvent{}} {
					if !db.Migrator().HasTable(table) {
						t.Fatalf("missing %T", table)
					}
				}
				if lineage == "p01-plugin" {
					var app model.PluginApplication
					if err = db.First(&app, "id = ?", "preserved").Error; err != nil || !app.Installed {
						t.Fatalf("P01 data lost: %+v %v", app, err)
					}
					var record schemaMigration
					if err = db.First(&record, "version = ?", 28).Error; err != nil || record.Name != "channel_credit_cost" {
						t.Fatalf("cost migration not remapped: %+v %v", record, err)
					}
				}
			})
		}
	}
}

func p02LineageDatabase(t *testing.T, driver, lineage string) *gorm.DB {
	t.Helper()
	config := Config{Driver: "sqlite", DSN: fmt.Sprintf("file:p02_lineage_%s_%d?mode=memory&cache=shared", lineage, time.Now().UnixNano())}
	if driver == "postgres" {
		dsn := strings.TrimSpace(os.Getenv("CANVAS_TEST_POSTGRES_DSN"))
		if dsn == "" {
			t.Skip("isolated PostgreSQL DSN not configured")
		}
		base, err := Open(Config{Driver: "postgres", DSN: dsn})
		if err != nil {
			t.Fatal(err)
		}
		schema := fmt.Sprintf("p02_lineage_%d", time.Now().UnixNano())
		if err = base.Exec(`CREATE SCHEMA "` + schema + `"`).Error; err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = base.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`).Error
			sqlDB, _ := base.DB()
			_ = sqlDB.Close()
		})
		config = Config{Driver: "postgres"}
		config.DSN, err = postgresDSNWithSearchPath(dsn, schema)
		if err != nil {
			t.Fatal(err)
		}
	}
	db, err := Open(config)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}
