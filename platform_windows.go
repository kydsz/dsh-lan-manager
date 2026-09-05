//go:build windows

package main

import (
	"os/exec"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32       = windows.NewLazySystemDLL("kernel32.dll")
	procGetCFontEx = kernel32.NewProc("GetCurrentConsoleFontEx")
)

const lfFacesize = 32

// consoleFontEx 对应 Win32 CONSOLE_FONT_INFOEX,仅取需要的字段。
type consoleFontEx struct {
	cbSize     uint32
	nFont      uint32
	dwFontX    uint16
	dwFontY    uint16
	fontFamily uint32
	fontWeight uint32
	face       [lfFacesize]uint16
}

// setUtf8Console UTF-8 输出,并给 stdout/stderr 开 VT(二维码走 stderr)。
func setUtf8Console() {
	windows.SetConsoleOutputCP(65001)
	for _, std := range []uint32{windows.STD_OUTPUT_HANDLE, windows.STD_ERROR_HANDLE} {
		h, err := windows.GetStdHandle(std)
		if err != nil {
			continue
		}
		var m uint32
		if windows.GetConsoleMode(h, &m) == nil {
			windows.SetConsoleMode(h, m|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
		}
	}
}

// qrStyleForConsole 中文字体用整块模式,其余用半块。
func qrStyleForConsole() qrStyle {
	name := strings.ToLower(consoleFaceName())
	for _, hint := range wideFontHints {
		if strings.Contains(name, hint) {
			return qrFullBlock
		}
	}
	return qrHalfBlock
}

func consoleFaceName() string {
	h, err := windows.GetStdHandle(windows.STD_ERROR_HANDLE)
	if err != nil {
		return ""
	}
	fe := consoleFontEx{cbSize: uint32(unsafe.Sizeof(consoleFontEx{}))}
	r, _, _ := procGetCFontEx.Call(uintptr(h), 0, uintptr(unsafe.Pointer(&fe)))
	if r == 0 {
		return ""
	}
	var b strings.Builder
	for _, c := range fe.face {
		if c == 0 {
			break
		}
		b.WriteRune(rune(c))
	}
	return b.String()
}

// wideFontHints:命中即认为方块字符按全角渲染。
var wideFontHints = []string{
	"宋体", "黑体", "楷体", "仿宋", "等线", "微软雅黑", "雅黑", "明体", "苹方",
	"思源", "更纱", "文楷", "霞鹜", "中易",
	"simsun", "nsimsun", "simhei", "kaiti", "fangsong", "dengxian", "yahei", "pingfang",
	"source han", "noto sans cjk", "noto serif cjk", "sarasa", "lxgw", "wenkai",
	"cjk", "hei", "song", "ming", "gothic", "meiryo", "malgun",
}

// Windows 下子进程是 wsl.exe,进程组与本地 kill 均无意义。
func prepCmd(c *exec.Cmd) {}

func killGroup(pid int, sig string) {}

func killPid(pid int, sig string) {}
