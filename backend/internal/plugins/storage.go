package plugins

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/plugins/contracts"
	"infinite-canvas/backend/internal/protocol"
	"infinite-canvas/backend/internal/repository"
)

func (s *Service) persistPackage(key string, files map[string][]byte) error {
	root := filepath.Join(s.dataDir, "application-plugin-packages")
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		entry, err := writer.Create(name)
		if err != nil {
			return err
		}
		if _, err = entry.Write(files[name]); err != nil {
			return err
		}
	}
	if err := writer.Close(); err != nil {
		return err
	}
	target := filepath.Join(root, key)
	if existing, err := os.ReadFile(target); err == nil {
		if !bytes.Equal(existing, buffer.Bytes()) {
			return issue(409, "plugin_version_conflict", "已保存的插件包内容损坏，拒绝覆盖")
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	file, err := os.CreateTemp(root, ".pending-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(buffer.Bytes()); err != nil {
		file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	// Same catalog lock protects this rename across processes; installation
	// never overwrites an existing content-addressed package.
	return os.Rename(file.Name(), target)
}
func (s *Service) verifyPackage(release model.PluginRelease) error {
	_, err := s.loadPackage(release)
	return err
}
func (s *Service) loadPackage(release model.PluginRelease) (contracts.PackageFiles, error) {
	if filepath.Base(release.PackageKey) != release.PackageKey {
		return nil, fmt.Errorf("invalid stored package key")
	}
	raw, err := os.ReadFile(filepath.Join(s.dataDir, "application-plugin-packages", release.PackageKey))
	if err != nil {
		return nil, err
	}
	pkg, err := protocol.ReadPluginPackageEnvelope(raw)
	if err != nil {
		return nil, err
	}
	if contracts.PackageDigest(pkg.Files) != release.Digest {
		return nil, issue(409, "plugin_version_conflict", "发布文件摘要不匹配")
	}
	return pkg.Files, nil
}

// PruneOrphans only touches old, content-addressed application archives that
// no release references. Historical/uninstalled releases still retain files.
func (s *Service) PruneOrphans(dryRun bool) ([]string, error) {
	candidates := []string{}
	err := s.repo.WithPluginCatalog(func(repo *repository.Repository) error {
		root := filepath.Join(s.dataDir, "application-plugin-packages")
		entries, err := os.ReadDir(root)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			name := entry.Name()
			if len(name) != 64+len(".yingce-plugin") || !strings.HasSuffix(name, ".yingce-plugin") {
				continue
			}
			valid := true
			for _, c := range name[:64] {
				valid = valid && (c >= '0' && c <= '9' || c >= 'a' && c <= 'f')
			}
			if !valid {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if time.Since(info.ModTime()) < 24*time.Hour {
				continue
			}
			used, err := repo.ApplicationPackageReferenced(name)
			if err != nil {
				return err
			}
			if used {
				continue
			}
			candidates = append(candidates, name)
			if !dryRun {
				if err = os.Remove(filepath.Join(root, name)); err != nil {
					return err
				}
			}
		}
		return nil
	})
	return candidates, err
}
