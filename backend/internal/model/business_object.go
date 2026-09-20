package model

import "time"

// BusinessObject is an owned index. Versions are immutable and retained on archive.
type BusinessObject struct {
	ID        string    `json:"objectId" gorm:"primaryKey;size:80"`
	UserID    string    `json:"-" gorm:"index;size:36"`
	Type      string    `json:"type" gorm:"size:32"`
	Title     string    `json:"title" gorm:"size:160"`
	Version   int       `json:"version"`
	Archived  bool      `json:"archived"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
type BusinessObjectVersion struct {
	ObjectID      string    `json:"objectId" gorm:"primaryKey;size:80;uniqueIndex:idx_object_version_key,priority:1"`
	Version       int       `json:"version" gorm:"primaryKey"`
	SchemaVersion int       `json:"schemaVersion"`
	DataJSON      string    `json:"-" gorm:"type:text"`
	Digest        string    `json:"digest" gorm:"size:64"`
	SourceJSON    string    `json:"-" gorm:"type:text"`
	ClientKey     string    `json:"-" gorm:"size:128;uniqueIndex:idx_object_version_key,priority:2"`
	RequestDigest string    `json:"-" gorm:"size:64"`
	CreatedAt     time.Time `json:"createdAt"`
}
