package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// A bounded undo record for the few files owned by one configuration change.
// It is private, never returned by an API, and cleared only after commit.
type configFileBefore struct {
	Name   string `json:"name"`
	Exists bool   `json:"exists"`
	Data   []byte `json:"data,omitempty"`
}
type configTransaction struct {
	Version int                `json:"version"`
	Files   []configFileBefore `json:"files"`
	dir     string
	journal string
}

func syncConfigDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

func atomicConfigWrite(path string, blob []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".config-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(blob); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return err
	}
	return syncConfigDir(filepath.Dir(path))
}

func beginConfigTransaction(dir, journal string, names []string) (*configTransaction, error) {
	if _, err := os.Lstat(filepath.Join(dir, journal)); err == nil {
		return nil, fmt.Errorf("存在未完成的配置事务，请先恢复")
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	tx := &configTransaction{Version: 1, dir: dir, journal: journal}
	for _, name := range names {
		if filepath.Base(name) != name {
			return nil, fmt.Errorf("配置文件名无效")
		}
		blob, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		tx.Files = append(tx.Files, configFileBefore{Name: name, Exists: err == nil, Data: blob})
	}
	if err := tx.writeJournal(); err != nil {
		return nil, err
	}
	return tx, nil
}

func (tx *configTransaction) writeJournal() error {
	blob, err := json.Marshal(tx)
	if err != nil {
		return err
	}
	return atomicConfigWrite(filepath.Join(tx.dir, tx.journal), blob)
}

func (tx *configTransaction) commit() error {
	if err := os.Remove(filepath.Join(tx.dir, tx.journal)); err != nil {
		return err
	}
	return syncConfigDir(tx.dir)
}

func (tx *configTransaction) rollback() error {
	// Recreate the record too if removing it failed after an uncertain fsync.
	if err := tx.writeJournal(); err != nil {
		return err
	}
	for _, before := range tx.Files {
		path := filepath.Join(tx.dir, before.Name)
		if before.Exists {
			if err := atomicConfigWrite(path, before.Data); err != nil {
				return err
			}
		} else if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if err := syncConfigDir(tx.dir); err != nil {
		return err
	}
	return tx.commit()
}

func recoverConfigTransaction(dir, journal string, names []string) error {
	blob, err := os.ReadFile(filepath.Join(dir, journal))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var tx configTransaction
	if err := json.Unmarshal(blob, &tx); err != nil {
		return fmt.Errorf("无法解析配置恢复记录")
	}
	allowed := map[string]bool{}
	for _, name := range names {
		allowed[name] = true
	}
	if tx.Version != 1 || len(tx.Files) != len(names) {
		return fmt.Errorf("配置恢复记录版本或文件集合无效")
	}
	for _, before := range tx.Files {
		if !allowed[before.Name] || filepath.Base(before.Name) != before.Name {
			return fmt.Errorf("配置恢复记录包含非受管文件")
		}
		delete(allowed, before.Name)
		if !before.Exists && len(before.Data) > 0 {
			return fmt.Errorf("配置恢复记录不一致")
		}
	}
	tx.dir, tx.journal = dir, journal
	if err := tx.rollback(); err != nil {
		return fmt.Errorf("恢复未完成配置失败: %w", err)
	}
	return nil
}
