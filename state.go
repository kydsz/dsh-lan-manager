package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type State struct {
	Port             int      `json:"port"`
	PID              int      `json:"pid,omitempty"` // Linux 下 dsh 进程组 ID,用于精确停止
	LastURLs         []string `json:"last_urls,omitempty"`
	LastCheck        int64    `json:"last_check,omitempty"`         // 上次应用版本检查成功时间(unix 秒)
	LastHarnessCheck int64    `json:"last_harness_check,omitempty"` // 上次 Harness 更新检查成功时间(unix 秒)
}

func statePath(patch string) string {
	return filepath.Join(filepath.Dir(patch), "manager-state.json")
}

func loadState(patch string) *State {
	s := &State{Port: 3080}
	if b, err := os.ReadFile(statePath(patch)); err == nil {
		if json.Unmarshal(b, s) == nil && s.Port > 0 && s.Port <= 65535 {
			return s
		}
	}
	return &State{Port: 3080}
}

func saveState(patch string, port, pid int, urls []string) error {
	st := loadState(patch)
	st.Port, st.PID, st.LastURLs = port, pid, urls
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(statePath(patch), b, 0o644)
}

// markStateCheck 记录某类更新检查成功时间,保留其余状态。
// kind 为空表示应用自身,"harness" 表示 DeepSeek Harness。
func markStateCheck(patch, kind string, t int64) error {
	st := loadState(patch)
	if kind == "harness" {
		st.LastHarnessCheck = t
	} else {
		st.LastCheck = t
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(statePath(patch), b, 0o644)
}
