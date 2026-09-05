package main

import (
	"fmt"
	"io"
	"sync"
)

// outMu 串行化终端输出,防止日志行打进二维码行间。
var outMu sync.Mutex

// syncPrintf / syncPrintln 锁内整行原子输出。
func syncPrintf(w io.Writer, format string, a ...any) {
	outMu.Lock()
	defer outMu.Unlock()
	fmt.Fprintf(w, format, a...)
}

func syncPrintln(w io.Writer, a ...any) {
	outMu.Lock()
	defer outMu.Unlock()
	fmt.Fprintln(w, a...)
}

// relayStderr 逐块转发子进程 stderr(锁内写)。
func relayStderr(w io.Writer, r io.Reader) {
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			outMu.Lock()
			w.Write(buf[:n])
			outMu.Unlock()
		}
		if err != nil {
			return
		}
	}
}
