package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// patchContent 生成 lan-bind.yml;local=true 时仅绑定本机,不共享局域网。
func patchContent(local bool) string {
	comment, host := "直绑所有网卡,由 dsh 自带 token 门授权局域网设备", "0.0.0.0"
	if local {
		comment, host = "仅绑定本机回环 127.0.0.1,不共享局域网", "127.0.0.1"
	}
	return fmt.Sprintf(`# dsh web %s。
- id: webserver
  config:
    host: '%s'
    port: !!js ctx.webStartup.port ?? 3080
    compression: gzip
    compressionLevel: 1
    compressionThresholdBytes: 1024
`, comment, host)
}

// 端口监听进程允许被按端口清理的运行时名。
var dshRuntimes = map[string]bool{
	"node": true, "nodejs": true, "bun": true, "tsx": true, "ts-node": true, "deno": true,
}

var procRe = regexp.MustCompile(`"([^"]+)",pid=(\d+)`)

type portProc struct {
	name string
	pid  int
}

// ensurePatch 按当前模式生成 lan-bind.yml;内容与现有文件不一致时重写(切换本机/局域网模式)。
func ensurePatch(path string, local bool) error {
	content := patchContent(local)
	if b, err := os.ReadFile(path); err == nil && string(b) == content {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// portFree 同时查本机与 WSL 侧(Windows 下 dsh 跑在 WSL 里)。
func portFree(port int, distro string) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	ln.Close()
	if runtime.GOOS == "windows" {
		return len(ssListeners(distro, port)) == 0
	}
	return true
}

// ssListeners 用 ss 查端口监听进程(需 -p 才有 pid)。
func ssListeners(distro string, port int) []portProc {
	var out []byte
	if runtime.GOOS == "windows" {
		out, _ = exec.Command("wsl", "-d", distro, "bash", "-lc", fmt.Sprintf(`ss -tlnp "sport = :%d"`, port)).Output()
	} else {
		out, _ = exec.Command("ss", "-tlnp", fmt.Sprintf("sport = :%d", port)).Output()
	}
	var ps []portProc
	for _, ln := range strings.Split(string(out), "\n") {
		if !strings.Contains(ln, "LISTEN") {
			continue
		}
		for _, m := range procRe.FindAllStringSubmatch(ln, -1) {
			pid, _ := strconv.Atoi(m[2])
			if pid > 0 {
				ps = append(ps, portProc{m[1], pid})
			}
		}
	}
	return ps
}

// buildCmd 组装启动命令;Windows 经 WSL 运行。
func buildCmd(dir, patch, profile string, port int, distro string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		d, p := wslPath(dir), wslPath(patch)
		prel := relPatch(d, p)
		args := []string{"dsh", "--profile", shq(profile), "--patch", shq(prel), "--port", strconv.Itoa(port)}
		line := fmt.Sprintf("export npm_config_confirm_modules_purge=false && cd %s && pnpm %s", shq(d), strings.Join(args, " "))
		return exec.Command("wsl", "-d", distro, "bash", "-ic", line)
	}
	prel := relPatch(dir, patch)
	args := []string{"dsh", "--profile", profile, "--patch", prel, "--port", strconv.Itoa(port)}
	c := exec.Command("pnpm", args...)
	c.Env = append(os.Environ(), "npm_config_confirm_modules_purge=false")
	c.Dir = dir
	prepCmd(c)
	return c
}

// relPatch 取相对于仓库目录的 patch 路径。
func relPatch(dir, patch string) string {
	if strings.HasPrefix(patch, dir) {
		if rel := strings.TrimPrefix(patch, dir); strings.TrimPrefix(rel, "/") != "" {
			return strings.TrimPrefix(rel, "/")
		}
	}
	return patch
}

// wslPath Windows 路径转 WSL 路径。
func wslPath(p string) string {
	if strings.HasPrefix(p, `\\`) {
		segs := strings.SplitN(p, `\`, 4)
		if len(segs) == 4 {
			rest := segs[3]
			if i := strings.Index(rest, `\`); i >= 0 {
				rest = rest[i:]
			} else {
				rest = `\` + rest
			}
			p = strings.ReplaceAll(rest, `\`, "/")
			if !strings.HasPrefix(p, "/") {
				p = "/" + p
			}
			return p
		}
	} else if len(p) >= 2 && p[1] == ':' {
		p = "/mnt/" + strings.ToLower(p[:1]) + p[2:]
	}
	return strings.ReplaceAll(p, "\\", "/")
}

func shq(s string) string {
	return "'" + s + "'"
}

// resetAuth 清掉 dsh 授权记录,使所有设备重新扫码。
func resetAuth(distro string) error {
	script := `f="${DSH_HOME:-$HOME/.dsh}/.credentials.yaml" && perl -0pi -e 's/^(\s*)client-connection\/browser-session:\n(\1\s+.*\n?)+//gm; s/^records:[ \t]*\n(?=\S|$)//gm' "$f"`
	var err error
	if runtime.GOOS == "windows" {
		err = exec.Command("wsl", "-d", distro, "bash", "-lc", script).Run()
	} else {
		err = exec.Command("bash", "-lc", script).Run()
	}
	return err
}

// wslBash 在 WSL 里执行命令(Windows 专用);非 Windows 直接本地执行。
func wslBash(distro, script string) {
	if runtime.GOOS == "windows" {
		exec.Command("wsl", "-d", distro, "bash", "-lc", script).Run()
	} else {
		exec.Command("bash", "-lc", script).Run()
	}
}

// stopDsh 按 模式=>进程组=>端口 多路清理 dsh,并确认端口释放。
func stopDsh(patch, distro string) bool {
	port := loadState(patch).Port
	wslBash(distro, "pkill -f apps/cli/src/bin.ts 2>/dev/null; pkill -f 'pnpm dsh' 2>/dev/null")
	pid := loadState(patch).PID
	if runtime.GOOS != "windows" && pid > 0 {
		killGroup(pid, "TERM")
	}
	killPortProcs(distro, port, "TERM")
	time.Sleep(time.Second)
	if runtime.GOOS != "windows" && pid > 0 {
		killGroup(pid, "KILL")
	}
	killPortProcs(distro, port, "KILL") // 兜底强杀
	time.Sleep(time.Second)
	return portFree(port, distro)
}

// killPortProcs 按端口清理 dsh 运行时进程。
func killPortProcs(distro string, port int, sig string) {
	for _, p := range ssListeners(distro, port) {
		if !dshRuntimes[p.name] {
			continue
		}
		if runtime.GOOS == "windows" {
			wslBash(distro, fmt.Sprintf("kill -%s %d 2>/dev/null", sig, p.pid))
		} else {
			killPid(p.pid, sig)
		}
	}
}
