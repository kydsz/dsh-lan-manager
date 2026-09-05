package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	// updateAPI GitHub Releases 最新版本接口(本应用自身)。
	updateAPI = "https://api.github.com/repos/kydsz/dsh-lan-manager/releases/latest"
	// harnessRepoURL DeepSeek Harness 仓库地址(仅作展示)。
	harnessRepoURL = "https://github.com/deepseek-ai/deepseek-harness"
	// updateTimeout 网络请求超时,保证检查从不拖慢主流程。
	updateTimeout = 8 * time.Second
	// updateCheckInterval 启动时自动检查的最小间隔,避免频繁请求网络。
	updateCheckInterval = 24 * time.Hour
	// gitFetchTimeout git fetch 超时(秒)。
	gitFetchTimeout = "12"
)

// appVersion 当前版本;构建时可 -ldflags "-X main.appVersion=vX.Y.Z" 覆盖。
var appVersion = "1.0.0"

// releaseInfo GitHub release JSON 中需要的字段。
type releaseInfo struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Body    string `json:"body"`
}

// fetchLatestRelease 查询 GitHub 上本应用最新发布版本。
func fetchLatestRelease() (*releaseInfo, error) {
	client := &http.Client{Timeout: updateTimeout}
	req, err := http.NewRequest(http.MethodGet, updateAPI, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "dsh-lan-manager/"+appVersion)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, errors.New("仓库暂无发布的版本")
	default:
		return nil, fmt.Errorf("GitHub 返回 %s", resp.Status)
	}
	var r releaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	if r.TagName == "" {
		return nil, errors.New("返回数据缺少版本号")
	}
	return &r, nil
}

// harnessStatus dsh 仓库相对 git 远端的更新状态。
type harnessStatus struct {
	Head   string // 本地 HEAD 短哈希
	Behind int    // 落后上游提交数
	Msg    string // 上游最新提交标题
	Ver    string // 本地版本(describe tags 或 HEAD)
	UpHead string // 上游 HEAD 短哈希
	UpVer  string // 上游版本(describe tags 或 HEAD)
}

// checkHarness 对比本地 dsh 仓库与 git 远端:先 fetch,再比对 HEAD 与上游。
// Windows 下经 WSL 执行;distro 仅在 Windows 使用。
func checkHarness(dshDir, distro string) (*harnessStatus, error) {
	if _, err := os.Stat(dshDir); err != nil {
		return nil, fmt.Errorf("找不到 dsh 仓库 %s", dshDir)
	}
	script := fmt.Sprintf(`d=%s
timeout %s git -C "$d" fetch -q origin 2>/dev/null
head=$(git -C "$d" rev-parse --short HEAD 2>/dev/null)
up=$(git -C "$d" rev-parse --short @{u} 2>/dev/null)
[ -z "$up" ] && up=$(git -C "$d" rev-parse --short refs/remotes/origin/HEAD 2>/dev/null)
[ -z "$up" ] && up=$(git -C "$d" rev-parse --short origin/master 2>/dev/null)
if [ -z "$head" ] || [ -z "$up" ]; then echo ERR; exit 1; fi
behind=$(git -C "$d" rev-list --count HEAD..$up 2>/dev/null)
[ -z "$behind" ] && behind=0
msg=$(git -C "$d" log -1 --format=%%s $up 2>/dev/null)
ver=$(git -C "$d" describe --tags --abbrev=0 2>/dev/null)
[ -z "$ver" ] && ver=$head
upver=$(git -C "$d" describe --tags --abbrev=0 $up 2>/dev/null)
[ -z "$upver" ] && upver=$up
echo "$head"
echo "$behind"
echo "$msg"
echo "$ver"
echo "$up"
echo "$upver"`, shq(wslPath(dshDir)), gitFetchTimeout)
	out, err := gitRun(distro, script)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 6 {
		return nil, errors.New("git 输出解析失败")
	}
	if lines[0] == "ERR" {
		return nil, errors.New("仓库不是 git 仓库,或远端不可用")
	}
	behind, _ := strconv.Atoi(lines[1])
	return &harnessStatus{
		Head: lines[0], Behind: behind, Msg: lines[2],
		Ver: lines[3], UpHead: lines[4], UpVer: lines[5],
	}, nil
}

// gitRun 执行脚本:Windows 经 WSL,其余本地 bash。
func gitRun(distro, script string) ([]byte, error) {
	if runtime.GOOS == "windows" {
		return exec.Command("wsl", "-d", distro, "bash", "-lc", script).Output()
	}
	return exec.Command("bash", "-lc", script).Output()
}

// compareVersions 版本比较:a>b 返回 1,a<b 返回 -1,相等返回 0。
// 支持可选 v 前缀与预发布标记(1.2.0-beta),忽略构建元数据(1.2.0+build)。
func compareVersions(a, b string) int {
	a, _, _ = strings.Cut(strings.TrimPrefix(strings.TrimSpace(a), "v"), "+")
	b, _, _ = strings.Cut(strings.TrimPrefix(strings.TrimSpace(b), "v"), "+")
	am, apre := cutPre(a)
	bm, bpre := cutPre(b)
	if n := compareNumeric(am, bm); n != 0 {
		return n
	}
	switch {
	case apre == "" && bpre == "":
		return 0
	case apre == "":
		return 1 // 同主版本,无预发布视为更新
	case bpre == "":
		return -1
	default:
		return strings.Compare(apre, bpre)
	}
}

// cutPre 拆分主版本与预发布标识。
func cutPre(v string) (main, pre string) {
	if m, p, ok := strings.Cut(v, "-"); ok {
		return m, p
	}
	return v, ""
}

// compareNumeric 逐段比较点分数字版本;缺段按 0 补,非数字段按字典序。
func compareNumeric(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < max(len(as), len(bs)); i++ {
		av, bv := "0", "0"
		if i < len(as) {
			av = strings.TrimSpace(as[i])
		}
		if i < len(bs) {
			bv = strings.TrimSpace(bs[i])
		}
		if av == bv {
			continue
		}
		an, aerr := strconv.Atoi(av)
		bn, berr := strconv.Atoi(bv)
		if aerr == nil && berr == nil {
			if an > bn {
				return 1
			}
			return -1
		}
		if av > bv {
			return 1
		}
		return -1
	}
	return 0
}

// checkUpdateOnStart 打开时异步检测更新(应用自身 + DeepSeek Harness):
// 仅有更新才提示,失败/最新静默;各自 24 小时内成功检查过则跳过。
func checkUpdateOnStart(patch, dshDir string) {
	go func() {
		st := loadState(patch)
		var notes []string
		if st.LastCheck == 0 || time.Since(time.Unix(st.LastCheck, 0)) >= updateCheckInterval {
			if rel, err := fetchLatestRelease(); err == nil {
				markStateCheck(patch, "", time.Now().Unix())
				latest := strings.TrimPrefix(rel.TagName, "v")
				cur := strings.TrimPrefix(appVersion, "v")
				if compareVersions(latest, cur) > 0 {
					notes = append(notes, fmt.Sprintf("⚠ dsh-lan-manager 发现新版本 v%s(当前 v%s):%s", latest, cur, rel.HTMLURL))
				}
			}
		}
		if st.LastHarnessCheck == 0 || time.Since(time.Unix(st.LastHarnessCheck, 0)) >= updateCheckInterval {
			if hs, err := checkHarness(dshDir, wslDistro(dshDir)); err == nil {
				markStateCheck(patch, "harness", time.Now().Unix())
				if hs.Behind > 0 {
					notes = append(notes, fmt.Sprintf("⚠ DeepSeek Harness 有更新:%s → %s,落后 %d 个提交(最新:%s %s)",
						hs.Ver, hs.UpVer, hs.Behind, hs.UpHead, hs.Msg))
				}
			}
		}
		if len(notes) > 0 {
			syncPrintln(os.Stderr, strings.Join(notes, "\n"))
		}
	}()
}

// checkUpdate 菜单“检测更新”:同步查询应用与 Harness 并打印结果。
func checkUpdate(dshDir, distro string) {
	if rel, err := fetchLatestRelease(); err != nil {
		syncPrintln(os.Stderr, "dsh-lan-manager 检测失败:"+err.Error())
	} else {
		latest := strings.TrimPrefix(rel.TagName, "v")
		cur := strings.TrimPrefix(appVersion, "v")
		if compareVersions(latest, cur) > 0 {
			syncPrintf(os.Stdout, "dsh-lan-manager 有更新:当前 v%s,最新 v%s\n%s\n", cur, latest, rel.HTMLURL)
			if b := strings.TrimSpace(rel.Body); b != "" {
				if i := strings.IndexByte(b, '\n'); i > 0 {
					b = b[:i]
				}
				syncPrintf(os.Stdout, "更新说明: %s\n", b)
			}
		} else {
			syncPrintf(os.Stdout, "dsh-lan-manager 已是最新版本:v%s\n", cur)
		}
	}
	if hs, err := checkHarness(dshDir, distro); err != nil {
		syncPrintln(os.Stderr, "DeepSeek Harness 检测失败:"+err.Error())
	} else if hs.Behind > 0 {
		syncPrintf(os.Stdout, "DeepSeek Harness 有更新:当前 %s,上游 %s,落后 %d 个提交\n最新提交: %s %s\n%s\n",
			hs.Ver, hs.UpVer, hs.Behind, hs.UpHead, hs.Msg, harnessRepoURL)
	} else {
		syncPrintf(os.Stdout, "DeepSeek Harness 已是最新:%s (%s)\n", hs.Ver, hs.Head)
	}
}
