package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"unicode/utf16"

	"charm.land/huh/v2"
	"golang.org/x/term"
)

func main() {
	setUtf8Console()
	exeDir := baseDir()
	cfg := loadConfig(exeDir)
	dshDir := envOr("DSH_DIR", first(cfg.DshDir, detectDshDir(exeDir), exeDir))
	patch := filepath.Join(exeDir, "lan-bind.yml")

	if runtime.GOOS == "windows" && len(listWslDistros()) == 0 {
		fmt.Fprintln(os.Stderr, "未检测到 WSL 发行版,请先安装 WSL;或改用 Linux 二进制运行")
		os.Exit(1)
	}

	// 打开时异步检测更新(应用自身 + DeepSeek Harness),不阻塞后续流程。
	if cfg.autoCheckUpdate() {
		checkUpdateOnStart(patch, dshDir)
	}

	if term.IsTerminal(int(os.Stdin.Fd())) {
		dshDir, err := resolveDshDir(exeDir)
		if err != nil {
			fmt.Fprintln(os.Stderr, "已退出")
			return
		}
		runLoop(dshDir, patch, "web", wslDistro(dshDir))
		return
	}
	// 非交互:直接启动
	st := loadState(patch)
	p, err := pickPort(st.Port, wslDistro(dshDir))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	runStart(dshDir, patch, "web", p, wslDistro(dshDir), false)
}

// resolveDshDir 依次取环境变量、配置、自动检测;都没有则交互选择。
func resolveDshDir(exeDir string) (string, error) {
	if env := os.Getenv("DSH_DIR"); env != "" {
		return env, nil
	}
	cfg := loadConfig(exeDir)
	if cfg.DshDir != "" {
		return cfg.DshDir, nil
	}
	cp := filepath.Join(exeDir, "config.yml")
	use := detectDshDir(exeDir)
	if use == "" {
		dir, err := dirForm("未检测到 dsh 仓库,输入仓库路径(留空退出)")
		if err != nil {
			return "", err
		}
		if dir == "" {
			return "", errors.New("已退出")
		}
		saveDshDir(cp, dir)
		return dir, nil
	}
	choice := ""
	err := huh.Run(huh.NewSelect[string]().
		Title("dsh 仓库 :"+use).
		Description("确认后写入 config.yml,下次启动不再询问").
		Options(
			huh.NewOption("使用此路径", "use"),
			huh.NewOption("手动输入其他路径", "input"),
			huh.NewOption("退出", "quit"),
		).
		Value(&choice))
	if err != nil {
		return "", err
	}
	switch choice {
	case "use":
		saveDshDir(cp, use)
		return use, nil
	case "input":
		dir, err := dirForm("dsh 仓库路径")
		if err != nil {
			return "", err
		}
		if dir == "" {
			return "", errors.New("已退出")
		}
		saveDshDir(cp, dir)
		return dir, nil
	}
	return "", errors.New("已退出")
}

// pickPort 非交互模式下的端口选择。
func pickPort(port int, distro string) (int, error) {
	if portFree(port, distro) {
		return port, nil
	}
	fmt.Printf("端口 %d 已被占用(可能已有 dsh 在跑)\n输入新端口后回车,或直接回车退出: ", port)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	if n := strings.TrimSpace(line); n != "" {
		p, err := strconv.Atoi(n)
		if err != nil || p <= 0 || p > 65535 {
			return 0, fmt.Errorf("无效端口: %s", n)
		}
		return p, nil
	}
	return 0, fmt.Errorf("已退出")
}

func baseDir() string {
	if exe, err := os.Executable(); err == nil {
		return filepath.Dir(exe)
	}
	return "."
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func first(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// wslDistro 优先按 UNC 路径解析,否则看已安装列表。
func wslDistro(d string) string {
	for _, pre := range []string{`\\wsl.localhost\`, `\\wsl$\`} {
		if rest, ok := strings.CutPrefix(d, pre); ok {
			if i := strings.Index(rest, `\`); i > 0 {
				return rest[:i]
			}
		}
	}
	if list := listWslDistros(); len(list) == 1 {
		return list[0]
	}
	return "Ubuntu-24.04"
}

// listWslDistros wsl -l -q 已安装发行版列表;兼容 UTF-16LE 输出。
func listWslDistros() []string {
	out, err := exec.Command("wsl", "-l", "-q").Output()
	if err != nil {
		return nil
	}
	if len(out) >= 2 && out[0] == 0xff && out[1] == 0xfe {
		out = utf16ToUTF8(out[2:])
	} else if bytesContainsZero(out) {
		out = utf16ToUTF8(out)
	}
	var list []string
	for _, ln := range strings.Split(strings.ReplaceAll(string(out), "\x00", ""), "\n") {
		if d := strings.TrimSpace(ln); d != "" {
			list = append(list, d)
		}
	}
	return list
}

func bytesContainsZero(b []byte) bool {
	for _, c := range b {
		if c == 0 {
			return true
		}
	}
	return false
}

func utf16ToUTF8(b []byte) []byte {
	u := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		u = append(u, uint16(b[i])|uint16(b[i+1])<<8)
	}
	return []byte(string(utf16.Decode(u)))
}

// detectDshDir 在 exe 目录或其下 deepseek-harness 找 dsh 仓库。
func detectDshDir(exeDir string) string {
	for _, c := range []string{filepath.Join(exeDir, "deepseek-harness"), exeDir} {
		if _, err := os.Stat(filepath.Join(c, "pnpm-workspace.yaml")); err == nil {
			return c
		}
	}
	return ""
}
