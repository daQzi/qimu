// Package objects owns the built-in, versioned business object contracts.
// It does not accept plugin-defined SQL, schemas, or browser PayloadJSON.
package objects

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"infinite-canvas/backend/internal/kernel"
	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/repository"
	"io"
	"strings"
	"time"
	"unicode/utf8"
)

type Reference struct {
	ObjectID      string `json:"objectId"`
	Version       int    `json:"version"`
	Type          string `json:"type"`
	SchemaVersion int    `json:"schemaVersion"`
}
type Brand struct {
	Name         string   `json:"name"`
	Audience     string   `json:"audience"`
	Positioning  string   `json:"positioning"`
	Claims       []string `json:"claims"`
	Restrictions []string `json:"restrictions"`
}
type WriteRequest struct {
	ObjectID        string     `json:"objectId,omitempty"`
	ExpectedVersion int        `json:"expectedVersion"`
	ClientKey       string     `json:"clientKey"`
	Brand           Brand      `json:"brand"`
	Source          *Reference `json:"source,omitempty"`
}
type View struct {
	Reference      Reference  `json:"reference"`
	Brand          Brand      `json:"brand"`
	Digest         string     `json:"digest"`
	Source         *Reference `json:"source,omitempty"`
	Archived       bool       `json:"archived"`
	CurrentVersion int        `json:"currentVersion"`
}

func hash(v any) string {
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func invalid(message string) error  { return kernel.NewAppError(400, message) }
func conflict(message string) error { return kernel.NewAppError(409, message) }
func validText(value string, min, max int) bool {
	return utf8.ValidString(value) && strings.TrimSpace(value) == value && utf8.RuneCountInString(value) >= min && utf8.RuneCountInString(value) <= max
}
func ValidateBrand(b Brand) error {
	if !validText(b.Name, 1, 160) || !validText(b.Audience, 1, 1000) || !validText(b.Positioning, 1, 2000) || b.Claims == nil || b.Restrictions == nil || len(b.Claims) > 20 || len(b.Restrictions) > 20 {
		return invalid("请填写品牌名称、受众、定位；卖点和限制须为最多 20 项的列表")
	}
	for _, items := range [][]string{b.Claims, b.Restrictions} {
		for _, v := range items {
			if !validText(v, 1, 500) {
				return invalid("品牌条目须为 1–500 字符且无首尾空白")
			}
		}
	}
	raw, _ := json.Marshal(b)
	if len(raw) > 16<<10 {
		return invalid("品牌资料最多 16 KiB")
	}
	return nil
}
func DecodeReference(raw []byte) (Reference, error) {
	var ref Reference
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&ref) != nil || decoder.Decode(new(any)) != io.EOF || !validText(ref.ObjectID, 1, 80) || ref.Version < 1 || ref.Version > 100 || ref.Type != "brand" || ref.SchemaVersion != 1 {
		return ref, invalid("需要明确的 brand/v1 对象版本引用，不按相同字段自动转换类型")
	}
	return ref, nil
}
func Read(repo *repository.Repository, user string, ref Reference, allowArchived bool) (View, error) {
	raw, _ := json.Marshal(ref)
	if _, err := DecodeReference(raw); err != nil {
		return View{}, err
	}
	row, err := repo.BusinessObject(user, ref.ObjectID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return View{}, kernel.NotFound("品牌不存在或不属于当前账号")
	}
	if err != nil {
		return View{}, err
	}
	if row.Type != ref.Type {
		return View{}, invalid("对象类型不兼容")
	}
	if row.Archived && !allowArchived {
		return View{}, conflict("品牌已归档，不能用于新调用")
	}
	version, err := repo.BusinessObjectVersion(row.ID, ref.Version)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return View{}, kernel.NotFound("品牌版本不存在")
	}
	if err != nil {
		return View{}, err
	}
	if version.SchemaVersion != ref.SchemaVersion {
		return View{}, invalid("对象 Schema 版本不兼容，请显式转换")
	}
	view := View{Reference: ref, Digest: version.Digest, Archived: row.Archived, CurrentVersion: row.Version}
	if err = json.Unmarshal([]byte(version.DataJSON), &view.Brand); err != nil {
		return view, err
	}
	if err = ValidateBrand(view.Brand); err != nil {
		return view, err
	}
	if hash(view.Brand) != version.Digest {
		return view, fmt.Errorf("business object digest mismatch")
	}
	if version.SourceJSON != "" {
		if err = json.Unmarshal([]byte(version.SourceJSON), &view.Source); err != nil {
			return view, err
		}
	}
	return view, nil
}
func List(repo *repository.Repository, user, query string, archived bool, offset int) ([]model.BusinessObject, error) {
	if user == "" {
		return nil, kernel.Unauthorized("请先登录")
	}
	if !validText(query, 0, 120) || offset < 0 || offset > 1000 {
		return nil, invalid("检索参数无效")
	}
	return repo.BusinessObjects(user, query, archived, offset)
}
func ValidateWrite(req WriteRequest) error {
	if !validText(req.ClientKey, 8, 128) || req.ExpectedVersion < 0 || req.ExpectedVersion > 100 || (req.ObjectID == "" && req.ExpectedVersion != 0) || (req.ObjectID != "" && !validText(req.ObjectID, 1, 80)) {
		return invalid("需要有效的请求编号和预期版本")
	}
	return ValidateBrand(req.Brand)
}

// Preview has no write side effects; Save repeats these checks under the owner lock.
func Preview(repo *repository.Repository, user string, req WriteRequest) (View, error) {
	if err := ValidateWrite(req); err != nil {
		return View{}, err
	}
	id := req.ObjectID
	if id == "" {
		id = "brand-" + hash([]string{user, req.ClientKey})[:32]
	}
	row, err := repo.BusinessObject(user, id)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return View{}, err
	}
	if err == nil {
		old, e := repo.BusinessObjectVersionByKey(id, req.ClientKey)
		if e == nil {
			if old.RequestDigest != hash(req) {
				return View{}, conflict("请求编号已用于不同品牌内容")
			}
			return Read(repo, user, Reference{id, old.Version, "brand", 1}, true)
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return View{}, e
		}
		if row.Archived || row.Version != req.ExpectedVersion {
			return View{}, conflict("品牌版本或归档状态已变化")
		}
	} else if req.ObjectID != "" {
		return View{}, kernel.NotFound("品牌不存在")
	} else {
		n, e := repo.BusinessObjectCount(user)
		if e != nil {
			return View{}, e
		}
		if n >= 100 {
			return View{}, conflict("每个账号最多 100 个品牌对象（含归档）")
		}
	}
	if req.ExpectedVersion >= 100 {
		return View{}, conflict("每个品牌最多 100 个版本")
	}
	if req.Source != nil {
		if req.ObjectID != "" {
			return View{}, invalid("派生只能创建新对象")
		}
		if _, err = Read(repo, user, *req.Source, true); err != nil {
			return View{}, err
		}
	}
	return View{Reference: Reference{id, req.ExpectedVersion + 1, "brand", 1}, Brand: req.Brand, Digest: hash(req.Brand), Source: req.Source, CurrentVersion: req.ExpectedVersion + 1}, nil
}
func Save(repo *repository.Repository, user string, req WriteRequest) (View, error) {
	if err := ValidateWrite(req); err != nil {
		return View{}, err
	}
	id := req.ObjectID
	if id == "" {
		id = "brand-" + hash([]string{user, req.ClientKey})[:32]
	}
	digest := hash(req)
	var view View
	err := repo.WithBusinessObjectWriter(user, func(tx *repository.Repository) error {
		row, err := tx.BusinessObject(user, id)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		creating := errors.Is(err, gorm.ErrRecordNotFound)
		if creating && req.ObjectID != "" {
			return kernel.NotFound("品牌不存在或不属于当前账号")
		}
		if !creating {
			previous, e := tx.BusinessObjectVersionByKey(id, req.ClientKey)
			if e == nil {
				if previous.RequestDigest != digest {
					return conflict("请求编号已用于不同品牌内容")
				}
				view, e = Read(tx, user, Reference{id, previous.Version, "brand", 1}, true)
				return e
			}
			if !errors.Is(e, gorm.ErrRecordNotFound) {
				return e
			}
			if row.Archived {
				return conflict("品牌已归档，请先恢复")
			}
			if row.Version != req.ExpectedVersion {
				return conflict("品牌版本已变化，请重新读取后保存")
			}
		} else {
			n, e := tx.BusinessObjectCount(user)
			if e != nil {
				return e
			}
			if n >= 100 {
				return conflict("每个账号最多 100 个品牌对象（含归档）")
			}
			row = &model.BusinessObject{ID: id, UserID: user, Type: "brand", CreatedAt: time.Now()}
		}
		if row.Version >= 100 {
			return conflict("每个品牌最多 100 个版本")
		}
		source := ""
		if req.Source != nil {
			if !creating {
				return invalid("派生只能创建新对象")
			}
			if _, e := Read(tx, user, *req.Source, true); e != nil {
				return e
			}
			raw, _ := json.Marshal(req.Source)
			source = string(raw)
		}
		row.Version++
		row.Title = req.Brand.Name
		row.UpdatedAt = time.Now()
		if creating {
			if err = tx.CreateBusinessObject(row); err != nil {
				return err
			}
		} else {
			if err = tx.UpdateBusinessObject(row); err != nil {
				return err
			}
		}
		raw, _ := json.Marshal(req.Brand)
		version := &model.BusinessObjectVersion{ObjectID: id, Version: row.Version, SchemaVersion: 1, DataJSON: string(raw), Digest: hash(req.Brand), SourceJSON: source, ClientKey: req.ClientKey, RequestDigest: digest, CreatedAt: time.Now()}
		if err = tx.CreateBusinessObjectVersion(version); err != nil {
			return err
		}
		view, err = Read(tx, user, Reference{id, row.Version, "brand", 1}, true)
		return err
	})
	return view, err
}
func Archive(repo *repository.Repository, user, id string, expected int, archived bool) error {
	if !validText(id, 1, 80) || expected < 1 || expected > 100 {
		return invalid("需要有效的对象编号和当前版本")
	}
	return repo.WithBusinessObjectWriter(user, func(tx *repository.Repository) error {
		row, err := tx.BusinessObject(user, id)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return kernel.NotFound("品牌不存在")
		}
		if err != nil {
			return err
		}
		if row.Version != expected {
			return conflict("品牌版本已变化")
		}
		row.Archived = archived
		return tx.UpdateBusinessObject(row)
	})
}
