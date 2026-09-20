package repository

import (
	"gorm.io/gorm"
	"infinite-canvas/backend/internal/model"
	"time"
)

func (r *Repository) WithBusinessObjectWriter(user string, fn func(*Repository) error) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// Serialize quota, version allocation and archive for both SQLite and PostgreSQL.
		lock := tx.Model(&model.User{}).Where("id = ?", user).UpdateColumn("id", gorm.Expr("id"))
		if lock.Error != nil {
			return lock.Error
		}
		if lock.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return fn(New(tx))
	})
}
func (r *Repository) BusinessObject(user, id string) (*model.BusinessObject, error) {
	var row model.BusinessObject
	err := r.db.Where("user_id = ? AND id = ?", user, id).First(&row).Error
	return &row, err
}
func (r *Repository) BusinessObjectVersion(id string, version int) (*model.BusinessObjectVersion, error) {
	var row model.BusinessObjectVersion
	err := r.db.Where("object_id = ? AND version = ?", id, version).First(&row).Error
	return &row, err
}
func (r *Repository) BusinessObjectVersionByKey(id, key string) (*model.BusinessObjectVersion, error) {
	var row model.BusinessObjectVersion
	err := r.db.Where("object_id = ? AND client_key = ?", id, key).First(&row).Error
	return &row, err
}
func (r *Repository) BusinessObjects(user, query string, archived bool, offset int) ([]model.BusinessObject, error) {
	rows := []model.BusinessObject{}
	q := r.db.Where("user_id = ? AND archived = ?", user, archived)
	if query != "" {
		q = q.Where("title LIKE ?", "%"+query+"%")
	}
	err := q.Order("updated_at DESC, id DESC").Offset(offset).Limit(30).Find(&rows).Error
	return rows, err
}
func (r *Repository) BusinessObjectCount(user string) (int64, error) {
	var n int64
	err := r.db.Model(&model.BusinessObject{}).Where("user_id = ?", user).Count(&n).Error
	return n, err
}
func (r *Repository) CreateBusinessObject(row *model.BusinessObject) error {
	return r.db.Create(row).Error
}
func (r *Repository) CreateBusinessObjectVersion(row *model.BusinessObjectVersion) error {
	return r.db.Create(row).Error
}
func (r *Repository) UpdateBusinessObject(row *model.BusinessObject) error {
	return r.db.Model(&model.BusinessObject{}).Where("id = ? AND user_id = ?", row.ID, row.UserID).Updates(map[string]any{"version": row.Version, "title": row.Title, "archived": row.Archived, "updated_at": time.Now()}).Error
}
