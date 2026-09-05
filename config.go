package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const configTemplate = `#  DeepSeek harness 路径配置
dsh-dir: ""        # DSH 仓库目录,留空=自动检测(程序目录或其下 deepseek-harness)
update-check: true # 打开时自动检测 GitHub 最新版本,false 关闭
`

type Config struct {
	DshDir      string `yaml:"dsh-dir"`
	UpdateCheck *bool  `yaml:"update-check"` // 打开时自动检测更新;未配置默认开启
}

// autoCheckUpdate 未配置 update-check 时默认开启。
func (c *Config) autoCheckUpdate() bool {
	return c.UpdateCheck == nil || *c.UpdateCheck
}

func loadConfig(dir string) *Config {
	cfg := &Config{}
	path := filepath.Join(dir, "config.yml")
	b, err := os.ReadFile(path)
	if err == nil {
		yaml.Unmarshal(b, cfg)
	}
	if err != nil {
		os.WriteFile(path, []byte(configTemplate), 0o644)
	}
	return cfg
}

func saveDshDir(path, dir string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var out []string
	done := false
	for _, ln := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(strings.TrimSpace(ln), "dsh-dir:") {
			out = append(out, "dsh-dir: "+strconv.Quote(dir))
			done = true
			continue
		}
		out = append(out, ln)
	}
	if !done {
		out = append(out, "dsh-dir: "+strconv.Quote(dir))
	}
	return os.WriteFile(path, []byte(strings.Join(out, "\n")), 0o644)
}
