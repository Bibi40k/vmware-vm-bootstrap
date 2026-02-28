package bootstrap

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	installStatsPath      = "install-stats.json"
	installStatsMaxSample = 30
)

type installStatsFile struct {
	Profiles map[string]installProfile `json:"profiles"`
}

type installProfile struct {
	SamplesSec []int64 `json:"samples_sec"`
	UpdatedAt  string  `json:"updated_at"`
}

func installStatsKey(cfg *VMConfig) string {
	profile := strings.TrimSpace(cfg.EffectiveProfile())
	if profile == "" {
		profile = "unknown"
	}
	version := strings.TrimSpace(cfg.EffectiveOSVersion())
	if version == "" {
		version = "unknown"
	}
	return fmt.Sprintf("%s-%s_cpu-%d_mem-%d", profile, version, cfg.CPUs, cfg.MemoryMB)
}

func loadInstallDurationEstimate(cfg *VMConfig) (time.Duration, int) {
	statsPath := resolveInstallStatsPath()
	data, err := os.ReadFile(statsPath)
	if err != nil {
		return 0, 0
	}
	var st installStatsFile
	if err := json.Unmarshal(data, &st); err != nil || st.Profiles == nil {
		return 0, 0
	}
	prof, ok := st.Profiles[installStatsKey(cfg)]
	if !ok || len(prof.SamplesSec) == 0 {
		return 0, 0
	}
	s := append([]int64(nil), prof.SamplesSec...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	median := s[len(s)/2]
	if median <= 0 {
		return 0, 0
	}
	return time.Duration(median) * time.Second, len(prof.SamplesSec)
}

func recordInstallDuration(cfg *VMConfig, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	statsPath := resolveInstallStatsPath()
	if err := os.MkdirAll(filepath.Dir(statsPath), 0o755); err != nil {
		return err
	}

	st := installStatsFile{Profiles: map[string]installProfile{}}
	if data, err := os.ReadFile(statsPath); err == nil {
		_ = json.Unmarshal(data, &st)
		if st.Profiles == nil {
			st.Profiles = map[string]installProfile{}
		}
	}

	key := installStatsKey(cfg)
	prof := st.Profiles[key]
	prof.SamplesSec = append(prof.SamplesSec, int64(d.Round(time.Second)/time.Second))
	if len(prof.SamplesSec) > installStatsMaxSample {
		prof.SamplesSec = prof.SamplesSec[len(prof.SamplesSec)-installStatsMaxSample:]
	}
	prof.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	st.Profiles[key] = prof

	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(statsPath, raw, 0o644)
}

func resolveInstallStatsPath() string {
	wd, err := os.Getwd()
	if err != nil {
		return filepath.Join("tmp", installStatsPath)
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, "tmp", installStatsPath)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return filepath.Join("tmp", installStatsPath)
}
