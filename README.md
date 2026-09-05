# dsh-lan-manager

dsh(DeepSeek Harness)局域网管理工具。交互式菜单一键启动/停止 dsh、扫码访问、授权管理,终端直接打印二维码。

## 特性

- 交互式菜单:启动 dsh(含二维码)、仅本机启动、重看二维码、停止、重置授权、检测更新

- 终端二维码:ANSI 颜色 + ASCII 渲染,兼容各种终端/字体,Windows 自动启用 UTF-8 与 VT

- 局域网共享:自动生成 `lan-bind.yml`,打印带 token 的地址并附二维码;也支持仅本机模式

- 重置授权:一键清空浏览器会话,所有设备重新扫码

- 更新检测:自动/手动检测本应用与 DeepSeek Harness 更新

- 跨平台:Windows(经 WSL)与 Linux;WSL 发行版自动识别,配置全部用相对路径

## 运行环境

| 平台 | 必需环境 |
| --- | --- |
| Windows | Windows 10/11 64 位 + WSL(已安装发行版,如 Ubuntu-24.04);dsh 在 WSL 内运行 |
| Linux | 64 位发行版,本机直接运行 |

**dsh 所在系统(即 WSL 发行版或 Linux 本机)需安装以下工具:**

| 工具 | 用途 |
| --- | --- |
| Node.js ≥ 18 + pnpm | 启动 dsh(DeepSeek Harness 自身依赖) |
| git | 检测 dsh 仓库更新 |
| perl | 重置授权时清理浏览器会话凭证 |
| ss(iproute2 包) | 检测端口占用 |
| bash + coreutils(timeout) | 执行内部脚本 |

**其他要求:**

- 自行构建需要 Go ≥ 1.26;直接使用可执行文件则无需安装 Go
- 更新检测需能访问 GitHub API
- 手机扫码访问需手机与电脑在同一局域网

## 安装

把可执行文件放在 **deepseek-harness 仓库目录** 下即可运行:

```
dsh-lan-manager 目录
├── dsh-lan-manager.exe   # Windows
├── dsh-lan-manager-linux # Linux(需 chmod +x)
└── deepseek-harness/     # dsh 仓库
```

```bash
chmod +x dsh-lan-manager-linux
```

也可以放任意目录,首次启动交互指定 dsh 路径(写入 `config.yml`),或用环境变量 `DSH_DIR`。

## 使用

运行后进入菜单,按需选择即可:

```
dsh 局域网管理 v1.0.0
  ▸ 启动 dsh(含二维码)
    启动 dsh(仅本机,不共享局域网)
    重看上次二维码
    停止 dsh
    重置授权(设备全部下线)
    检测更新
    退出
```

启动后手机扫码即可访问;token 每次启动随机生成,重启即失效。

## 构建

```bash
# Windows
go build -ldflags "-s -w -X main.appVersion=v1.0.0" -o dsh-lan-manager.exe .

# Linux
GOOS=linux GOARCH=amd64 go build -ldflags "-s -w -X main.appVersion=v1.0.0" -o dsh-lan-manager-linux .
```

```bash
go test ./...
```

## 许可证

[MIT](LICENSE)
