package main

import (
	"bytes"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
	"rsc.io/qr"
)

// 256 色纯黑/纯白,避免受主题 16 色影响。
const (
	ansiFgBlack = "\x1b[38;5;16m"
	ansiFgWhite = "\x1b[38;5;231m"
	ansiBgBlack = "\x1b[48;5;16m"
	ansiBgWhite = "\x1b[48;5;231m"
	ansiReset   = "\x1b[0m"
)

type qrStyle int

const (
	qrHalfBlock qrStyle = iota // "▀"+颜色,适合西文 1:2 格子字体
	qrFullBlock                // "█"+颜色,适合 CJK 方形格子字体
)

// printQR 绘制二维码。TTY 用颜色渲染,否则纯字符;
// 整体缓冲后锁内一次写入,且每行 \r\n 归列,不受并发日志与光标列影响。
func printQR(w io.Writer, text string) {
	code, err := qr.Encode(text, qr.M)
	if err != nil {
		return
	}
	var buf bytes.Buffer
	if term.IsTerminal(int(os.Stderr.Fd())) {
		printColorQR(&buf, code, qrStyleForConsole())
	} else {
		printGlyphQR(&buf, code)
	}
	outMu.Lock()
	defer outMu.Unlock()
	w.Write(buf.Bytes())
}

// printColorQR 单字符+颜色输出,quiet 为四周留白。
func printColorQR(w io.Writer, code *qr.Code, style qrStyle) {
	const quiet = 2
	size := code.Size
	wide := size + quiet*2

	// halfCell[top][bottom],索引 0=黑 1=白。
	halfCell := [2][2]string{
		{ // top 黑
			ansiFgBlack + ansiBgBlack + "▀", // bottom 黑
			ansiFgBlack + ansiBgWhite + "▀", // bottom 白
		},
		{ // top 白
			ansiFgWhite + ansiBgBlack + "▀",
			ansiFgWhite + ansiBgWhite + "▀",
		},
	}
	fullCell := [2]string{
		ansiFgBlack + ansiBgBlack + "█",
		ansiFgWhite + ansiBgWhite + "█",
	}
	colorIdx := func(b bool) int {
		if b {
			return 0
		}
		return 1
	}
	black := func(x, y int) bool {
		return 0 <= x && x < size && 0 <= y && y < size && code.Black(x, y)
	}
	in := func(x, y int) bool {
		return 0 <= x && x < wide && 0 <= y && y < wide
	}

	w.Write([]byte(ansiReset + "\r\n")) // 清残留颜色并归列
	switch style {
	case qrFullBlock:
		for y := 0; y < wide; y++ {
			var b strings.Builder
			b.Grow(wide * 16)
			for x := 0; x < wide; x++ {
				px := colorIdx(false)
				if in(x, y) && black(x-quiet, y-quiet) {
					px = colorIdx(true)
				}
				b.WriteString(fullCell[px])
			}
			w.Write([]byte(b.String() + ansiReset + "\r\n"))
		}
	default: // qrHalfBlock:每行 2 个像素高
		rows := (wide + 1) / 2
		for r := 0; r < rows; r++ {
			var b strings.Builder
			b.Grow(wide * 20)
			top := r * 2
			for x := 0; x < wide; x++ {
				t, bt := colorIdx(false), colorIdx(false)
				if in(x, top) && black(x-quiet, top-quiet) {
					t = colorIdx(true)
				}
				if in(x, top+1) && black(x-quiet, top+1-quiet) {
					bt = colorIdx(true)
				}
				b.WriteString(halfCell[t][bt])
			}
			w.Write([]byte(b.String() + ansiReset + "\r\n"))
		}
	}
}

// printGlyphQR 纯字符渲染(▀▄█+空格),用于非 TTY 输出。
func printGlyphQR(w io.Writer, code *qr.Code) {
	const quiet = 2
	size := code.Size
	wide := size + quiet*2

	black := func(x, y int) bool {
		return 0 <= x && x < size && 0 <= y && y < size && code.Black(x, y)
	}
	in := func(x, y int) bool {
		return 0 <= x && x < wide && 0 <= y && y < wide
	}
	rows := (wide + 1) / 2
	for r := 0; r < rows; r++ {
		var b strings.Builder
		b.Grow(wide * 2)
		top := r * 2
		for x := 0; x < wide; x++ {
			t, bt := false, false
			if in(x, top) {
				t = black(x-quiet, top-quiet)
			}
			if in(x, top+1) {
				bt = black(x-quiet, top+1-quiet)
			}
			switch {
			case t && bt:
				b.WriteString("█")
			case t:
				b.WriteString("▀")
			case bt:
				b.WriteString("▄")
			default:
				b.WriteString(" ")
			}
		}
		b.WriteString("\n")
		w.Write([]byte(b.String()))
	}
}
