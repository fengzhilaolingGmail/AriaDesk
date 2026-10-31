// Package history 持久化已完成/已停止任务的记录（aria2 进程退出后其结果即丢失，
// 应用侧自行保留历史，存放于 %APPDATA%\AriaDesk\history.json）。
package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"ariadesk/internal/config"
)

type Record struct {
	Gid             string    `json:"gid"`
	Name            string    `json:"name"`
	Status          string    `json:"status"` // complete / error / removed
	TotalLength     string    `json:"totalLength"`
	CompletedLength string    `json:"completedLength"`
	Dir             string    `json:"dir"`
	// FilePath 任务完成时解析出的目标文件/目录路径（多文件 BT 为以任务名命名的目录），
	// 用于“打开文件位置”定位；旧记录可能为空，前端回退到 Dir
	FilePath    string    `json:"filePath,omitempty"`
	CompletedAt time.Time `json:"completedAt"`
}

type Store struct {
	mu      sync.Mutex
	max     int
	records []Record
}

func path() string {
	return filepath.Join(config.DataDir(), "history.json")
}

func Load() *Store {
	s := &Store{max: config.DefaultHistoryMax}
	data, err := os.ReadFile(path())
	if err != nil {
		return s
	}
	_ = json.Unmarshal(data, &s.records)
	return s
}

// SetMax 设置记录条数上限（1~5000），超出立即裁剪最旧的记录。
func (s *Store) SetMax(n int) {
	if n <= 0 {
		n = config.DefaultHistoryMax
	}
	if n > config.MaxHistoryMax {
		n = config.MaxHistoryMax
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.max == n {
		return
	}
	s.max = n
	s.saveLocked()
}

func (s *Store) saveLocked() {
	if len(s.records) > s.max {
		s.records = s.records[:s.max]
	}
	data, err := json.MarshalIndent(s.records, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(path(), data, 0o644)
}

// Add 追加记录（按 gid 去重），返回记录列表是否发生变化。
func (s *Store) Add(r Record) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.records {
		if e.Gid == r.Gid {
			return false
		}
	}
	s.records = append(s.records, r)
	s.sortLocked()
	s.saveLocked()
	return true
}

func (s *Store) Remove(gid string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, e := range s.records {
		if e.Gid == gid {
			s.records = append(s.records[:i], s.records[i+1:]...)
			s.saveLocked()
			return true
		}
	}
	return false
}

func (s *Store) Clear() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.records) == 0 {
		return false
	}
	s.records = nil
	s.saveLocked()
	return true
}

// List 返回记录副本，新的在前。
func (s *Store) List() []Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Record, len(s.records))
	copy(out, s.records)
	return out
}

func (s *Store) sortLocked() {
	sort.SliceStable(s.records, func(i, j int) bool {
		return s.records[i].CompletedAt.After(s.records[j].CompletedAt)
	})
}
