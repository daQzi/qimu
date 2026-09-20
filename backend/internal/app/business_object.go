package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/objects"
	"infinite-canvas/backend/internal/plugins"
	"infinite-canvas/backend/internal/repository"
)

type BusinessObjectWrite = objects.WriteRequest
type BusinessObjectReference = objects.Reference
type BusinessObjectView = objects.View

func (s *Service) ListBusinessObjects(user, q string, archived bool, offset int) ([]model.BusinessObject, error) {
	if err := s.pluginOperationAccess(user); err != nil {
		return nil, err
	}
	return objects.List(s.repo, user, q, archived, offset)
}
func (s *Service) ReadBusinessObject(user, id string, version int) (objects.View, error) {
	if err := s.pluginOperationAccess(user); err != nil {
		return objects.View{}, err
	}
	return objects.Read(s.repo, user, objects.Reference{ObjectID: id, Version: version, Type: "brand", SchemaVersion: 1}, true)
}
func (s *Service) SaveBusinessObject(user string, req objects.WriteRequest) (objects.View, error) {
	if err := s.pluginOperationAccess(user); err != nil {
		return objects.View{}, err
	}
	return objects.Save(s.repo, user, req)
}
func (s *Service) ArchiveBusinessObject(user, id string, version int, archived bool) error {
	if err := s.pluginOperationAccess(user); err != nil {
		return err
	}
	return objects.Archive(s.repo, user, id, version, archived)
}
func businessObjectAdapters() []plugins.ShortHostAdapter {
	prepare := func(repo *repository.Repository, user string, input map[string]json.RawMessage, _ plugins.HostOperationContext) (plugins.PreparedOperation, error) {
		ref, err := objects.DecodeReference(input["reference"])
		if err != nil {
			return plugins.PreparedOperation{}, err
		}
		view, err := objects.Read(repo, user, ref, false)
		if err != nil {
			return plugins.PreparedOperation{}, err
		}
		raw, err := json.Marshal(view)
		return plugins.PreparedOperation{Result: raw, SourceDigest: view.Digest}, err
	}
	search := func(repo *repository.Repository, user string, input map[string]json.RawMessage, _ plugins.HostOperationContext) (plugins.PreparedOperation, error) {
		var query string
		var offset int
		if bytes.Equal(input["query"], []byte("null")) || bytes.Equal(input["offset"], []byte("null")) || json.Unmarshal(input["query"], &query) != nil || json.Unmarshal(input["offset"], &offset) != nil {
			return plugins.PreparedOperation{}, BadAuthRequest("检索输入无效")
		}
		rows, err := objects.List(repo, user, query, false, offset)
		if err != nil {
			return plugins.PreparedOperation{}, err
		}
		raw, err := json.Marshal(map[string]any{"objects": rows})
		return plugins.PreparedOperation{Result: raw}, err
	}
	save := func(repo *repository.Repository, user string, input map[string]json.RawMessage, _ plugins.HostOperationContext) (plugins.PreparedOperation, error) {
		for _, key := range []string{"expectedVersion", "clientKey", "brand"} {
			if len(input[key]) == 0 || bytes.Equal(input[key], []byte("null")) {
				return plugins.PreparedOperation{}, BadAuthRequest("品牌写入缺少必填字段")
			}
		}
		raw, err := json.Marshal(input)
		if err != nil {
			return plugins.PreparedOperation{}, err
		}
		var req objects.WriteRequest
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err = decoder.Decode(&req); err != nil {
			return plugins.PreparedOperation{}, BadAuthRequest("品牌写入输入无效")
		}
		view, err := objects.Preview(repo, user, req)
		if err != nil {
			return plugins.PreparedOperation{}, err
		}
		raw, err = json.Marshal(view)
		if err != nil {
			return plugins.PreparedOperation{}, err
		}
		digest := sha256.Sum256(raw)
		return plugins.PreparedOperation{Result: raw, SourceDigest: hex.EncodeToString(digest[:]), Commit: func(tx *repository.Repository) error { _, e := objects.Save(tx, user, req); return e }}, nil
	}
	return []plugins.ShortHostAdapter{
		{ID: "object.read", Permissions: []string{"asset.read"}, Effects: []string{"read"}, Prepare: prepare},
		{ID: "object.search", Permissions: []string{"asset.search"}, Effects: []string{"read"}, Prepare: search},
		{ID: "object.save", Permissions: []string{"asset.import"}, Effects: []string{"draft_write"}, Prepare: save},
	}
}
