package main

import (
	"bufio"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"runtime"
	"strings"
	"time"
)

var (
	ansiRe = regexp.MustCompile("\x1b\\[[0-9;]*[A-Za-z]")
	urlRe  = regexp.MustCompile("https?://[^\\s,)\\x60>]+")
)

// runLoop 主菜单循环。
func runLoop(dshDir, patch, profile, distro string) {
	for {
		choice, err := showMenu()
		if err != nil {
			return
		}
		st := loadState(patch)
		switch choice {
		case actStart:
			if p, ok := choosePort(st, distro); ok {
				runStart(dshDir, patch, profile, p, distro, false)
			}
		case actLocal:
			if p, ok := choosePort(st, distro); ok {
				runStart(dshDir, patch, profile, p, distro, true)
			}
		case actQR:
			showLastQR(st)
		case actStop:
			if stopDsh(patch, distro) {
				syncPrintln(os.Stdout, "dsh 已停止")
			} else {
				syncPrintln(os.Stdout, "端口仍在监听,可能还有残留进程")
			}
		case actReset:
			if resetAuth(distro) == nil {
				syncPrintln(os.Stdout, "授权已重置:所有设备需重新扫新二维码访问")
			} else {
				syncPrintln(os.Stdout, "重置失败,凭据文件可能不存在")
			}
		case actUpdate:
			checkUpdate(dshDir, distro)
		case actQuit:
			if !portFree(st.Port, distro) && confirmStop() {
				stopDsh(patch, distro)
			}
			return
		}
	}
}

// choosePort 交互选择端口,返回是否确定。
func choosePort(st *State, distro string) (int, bool) {
	p, err := portForm(st.Port)
	if err != nil {
		return 0, false
	}
	if !portFree(p, distro) {
		p, err = occupiedInput(p)
		if err != nil {
			return 0, false
		}
	}
	return p, true
}

// runStart 启动 dsh;local=true 时仅绑定本机,不共享局域网。
func runStart(dshDir, patch, profile string, port int, distro string, local bool) {
	if _, err := os.Stat(dshDir); err != nil {
		syncPrintf(os.Stderr, "找不到 dsh 仓库 %s:请把它放在程序目录下,或在 config.yml 配 dsh-dir\n", dshDir)
		return
	}
	if err := ensurePatch(patch, local); err != nil {
		syncPrintf(os.Stderr, "写 patch 失败: %v\n", err)
		return
	}
	_ = resetAuth(distro)
	cmd := buildCmd(dshDir, patch, profile, port, distro)
	syncPrintf(os.Stdout, "启动: %s\n\n", cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		syncPrintf(os.Stderr, "管道失败: %v\n", err)
		return
	}
	// stderr 也走管道转发,与二维码绘制串行化,避免被打断。
	stderrPipe, perr := cmd.StderrPipe()
	if perr != nil {
		syncPrintf(os.Stderr, "stderr 管道失败: %v\n", perr)
		return
	}
	go relayStderr(os.Stderr, stderrPipe)
	if err := cmd.Start(); err != nil {
		syncPrintf(os.Stderr, "启动失败: %v\n", err)
		return
	}
	setupSignal(patch, distro)

	lan := make(chan string, 8)
	go scan(stdout, lan)
	found := make(chan struct{})
	go func() {
		select {
		case <-time.After(10 * time.Second):
			syncPrintln(os.Stderr, "10 秒内未检测到访问地址,请检查 pnpm build 是否完成、端口是否被 Windows 侧占用")
		case <-found:
		}
	}()
	first := true
	checked := false
	seen := map[string]bool{}
	var urls []string
	for u := range lan {
		if seen[u] {
			continue
		}
		seen[u] = true
		urls = append(urls, u)
		isLan := printURL(u)
		if local {
			if isLan {
				syncPrintln(os.Stderr, "注意:检测到局域网地址,但当前为本机模式,其他设备无法访问")
			}
			if !checked {
				checked = true
				syncPrintln(os.Stderr, "仅本机模式:服务只绑定 127.0.0.1,不会共享到局域网")
			}
		} else if isLan && !checked {
			checked = true
			go checkAccess(u)
		}
		saveState(patch, port, pidOf(cmd), urls)
		if first {
			close(found)
			first = false
		}
	}
	_ = cmd.Wait()
}

// pidOf Windows 下子进程是 wsl.exe,不记录;Linux 记录进程组 ID 便于停止。
func pidOf(cmd *exec.Cmd) int {
	if runtime.GOOS == "windows" {
		return 0
	}
	return cmd.Process.Pid
}

// checkAccess 从本机请求一次,提示打印机/防火墙问题并给出排查线索。
func checkAccess(u string) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(u)
	if err != nil {
		syncPrintln(os.Stderr, "⚠ 自检:本机访问 "+u+" 失败,手机很可能也连不上。请检查 Windows 防火墙/网络(如 WSL 虚拟交换机隔离),或换一个网段再试")
		return
	}
	resp.Body.Close()
	syncPrintln(os.Stderr, "✓ 自检通过:本机可访问,局域网设备可扫码")
}

// printURL 打印地址;局域网地址附带二维码,返回是否为局域网地址。
func printURL(u string) bool {
	host := ""
	if parsed, err := url.Parse(u); err == nil {
		host = parsed.Hostname()
	}
	if host == "" || host == "localhost" || host == "127.0.0.1" || host == "::1" || host == "0.0.0.0" {
		syncPrintf(os.Stdout, "本机地址: %s\n", u)
		return false
	}
	syncPrintf(os.Stderr, "\r\n局域网地址: %s\r\n", u)
	printQR(os.Stderr, u)
	syncPrintln(os.Stderr)
	return true
}

// scan 透传 dsh 输出,并解析带 token 的访问地址。
func scan(r io.Reader, lan chan<- string) {
	defer close(lan)
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		syncPrintln(os.Stdout, line)
		text := ansiRe.ReplaceAllString(line, "")
		for _, u := range urlRe.FindAllString(text, -1) {
			if strings.Contains(u, "token=") {
				lan <- u
			}
		}
	}
}

// setupSignal Ctrl+C 时停掉 dsh。
func setupSignal(patch, distro string) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	go func() {
		<-sig
		syncPrintln(os.Stderr, "\n正在停止 dsh...")
		stopDsh(patch, distro)
	}()
}
