package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"charm.land/huh/v2"
)

const (
	actStart  = "start"
	actLocal  = "local"
	actQR     = "qr"
	actStop   = "stop"
	actReset  = "reset"
	actUpdate = "update"
	actQuit   = "quit"
)

// showMenu 主菜单。
func showMenu() (string, error) {
	choice := ""
	err := huh.Run(huh.NewSelect[string]().
		Title(fmt.Sprintf("dsh 局域网管理 v%s", appVersion)).
		Options(
			huh.NewOption("启动 dsh(含二维码)", actStart),
			huh.NewOption("启动 dsh(仅本机,不共享局域网)", actLocal),
			huh.NewOption("重看上次二维码", actQR),
			huh.NewOption("停止 dsh", actStop),
			huh.NewOption("重置授权(设备全部下线)", actReset),
			huh.NewOption("检测更新", actUpdate),
			huh.NewOption("退出", actQuit),
		).
		Value(&choice))
	if err != nil && !errors.Is(err, huh.ErrUserAborted) {
		fmt.Fprintf(os.Stderr, "菜单出错: %v\n", err)
	}
	return choice, err
}

// portForm 选择端口。
func portForm(defaultPort int) (int, error) {
	s := strconv.Itoa(defaultPort)
	err := huh.Run(huh.NewInput().
		Title("端口").
		Validate(func(v string) error {
			n, err := strconv.Atoi(v)
			if err != nil || n <= 0 || n > 65535 {
				return fmt.Errorf("端口需为 1-65535")
			}
			return nil
		}).
		Value(&s))
	if err != nil {
		return 0, err
	}
	p, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, err
	}
	return p, nil
}

// occupiedInput 端口被占用时输入新端口。
func occupiedInput(port int) (int, error) {
	s := ""
	err := huh.Run(huh.NewInput().
		Title(fmt.Sprintf("端口 %d 已被占用", port)).
		Description("输入新端口后回车,留空取消").
		Validate(func(v string) error {
			if v == "" {
				return nil
			}
			n, err := strconv.Atoi(v)
			if err != nil || n <= 0 || n > 65535 {
				return fmt.Errorf("端口需为 1-65535")
			}
			return nil
		}).
		Value(&s))
	if err != nil {
		return 0, err
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("已取消")
	}
	return strconv.Atoi(s)
}

// dirForm 输入目录。
func dirForm(title string) (string, error) {
	s := ""
	err := huh.Run(huh.NewInput().Title(title).Value(&s))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(s), nil
}

// showLastQR 重打上次保存的地址与二维码。
func showLastQR(st *State) {
	if len(st.LastURLs) == 0 {
		syncPrintln(os.Stdout, "还没有保存过访问地址,先启动一次 dsh")
		return
	}
	syncPrintln(os.Stdout, "上次访问地址(token 随 dsh 重启失效,无法访问请重新启动):")
	for _, u := range st.LastURLs {
		printURL(u)
	}
}

// confirmStop 退出前确认是否停止仍运行的 dsh。
func confirmStop() bool {
	yes := false
	err := huh.Run(huh.NewConfirm().
		Title("dsh 仍在运行,退出前是否停止?").
		Value(&yes))
	return err == nil && yes
}
