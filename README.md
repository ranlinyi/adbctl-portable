# adbctl（自包含单文件版）

把原 bash 脚本 `adbctl` 重写为**一个可执行文件**：内嵌 **adb（Android platform-tools）**、**scrcpy** 与
**scrcpy-server**，首次运行时自动释放到用户缓存目录。目标机器**不需要**预装 adb、scrcpy、bash 或任何脚本运行时。

支持平台：**Windows x86_64**、**Linux x86_64**。

---

## 1. 下载与使用（Release 便携版）

到本仓库的 **Releases** 页面下载对应文件：

| 平台 | 文件 | 用法 |
|---|---|---|
| Windows | `adbctl-windows-x86_64.exe` | 放进任意目录，双击或命令行运行 `adbctl-windows-x86_64.exe -h` |
| Linux | `adbctl-linux-x86_64` | `chmod +x adbctl-linux-x86_64 && ./adbctl-linux-x86_64 -h` |

两个文件都是自包含的**便携版**：拷到 U 盘或任意路径即可用，安装零依赖。

```bash
# Linux
./adbctl-linux-x86_64                 # 自动选路连接，打印选中的序列号或 IP:端口
./adbctl-linux-x86_64 -S              # 连上后投屏
./adbctl-linux-x86_64 -a 计算器 -S    # 只流转单个应用（虚拟显示，手机主屏不受影响）
./adbctl-linux-x86_64 -p 123456 -S    # 无线首次配对
```

```powershell
:: Windows
.\adbctl-windows-x86_64.exe -S
.\adbctl-windows-x86_64.exe -a 微信 -S
```

完整选项与行为见 `adbctl-usage.md`（同原脚本），本程序额外提供：

```
adbctl --version       # 版本
adbctl --print-paths   # 打印内嵌 adb / scrcpy 释放后的实际路径
```

### 输出约定

- **stdout 只有一样东西**：选中的目标（USB 序列号 或 无线 `IP:端口`）。
- **stderr 全部日志与提示**（包括交互选择界面）。

```bash
ADDR=$(./adbctl-linux-x86_64 -W)   # 变量拿到的是干净地址
```

---

## 2. 内嵌依赖释放位置

首次运行时把内嵌的 adb / scrcpy / scrcpy-server 释放到系统缓存目录：

| 平台 | 默认目录 |
|---|---|
| Linux | `\$XDG_CACHE_HOME/adbctl/1.0.0`（通常是 `~/.cache/adbctl/1.0.0`） |
| Windows | `%LOCALAPPDATA%\adbctl\1.0.0` |

可用环境变量 `ADBCTL_CACHE` 覆盖释放目录。删除该目录后下次运行会重新释放。

---

## 3. 环境变量

| 变量 | 默认 | 作用 |
|---|---|---|
| `ADB` | 内嵌 adb | 指定外部 adb 可执行文件 |
| `SCRCPY` | 内嵌 scrcpy | 指定外部 scrcpy 可执行文件 |
| `PAIR_IP` | 自己解析 mDNS | 指定取配对地址的命令 |
| `SCRCPY_ARGS` | 见下 | 整体替换 `-S` 时的 scrcpy 参数 |
| `ADBCTL_CACHE` | 系统缓存目录 | 内嵌依赖释放目录 |
| `SCRCPY_SERVER_PATH` | 内嵌 server | 指定 scrcpy-server 路径 |

默认 scrcpy 档（实测于 Redmi K70 / 540x1200 / H.265）：

```
--video-codec=h265 --video-bit-rate=12M --max-size=1200 --max-fps=52 --video-buffer=0
```

---

## 4. 从源码构建

### 依赖

- Go 1.22+（本仓库无需任何第三方 Go 模块）
- `curl`、`tar`、`unzip`、`sha256sum`

### 一键构建

```bash
./build.sh
```

脚本会：

1. 下载官方产物并校验 scrcpy 的 SHA256（见下表）；
2. 组装每个平台的运行目录（`bin/` + `lib/`）；
3. 用仓库内的小工具 `tools/mkzip` 打成 `payload/*.zip` 供 `go:embed` 内嵌；
4. 交叉编译出：
   - `dist/adbctl-linux-x86_64`
   - `dist/adbctl-windows-x86_64.exe`

### 手动构建

```bash
# 先按 build.sh 组装好 .build/linux 与 .build/windows，再：
go run ./tools/mkzip .build/linux   payload/adbctl-payload-linux-amd64.zip
go run ./tools/mkzip .build/windows payload/adbctl-payload-windows-amd64.zip

CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o dist/adbctl-linux-x86_64 .
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o dist/adbctl-windows-x86_64.exe .
```

---

## 5. 内嵌依赖版本与来源

| 组件 | 版本 | 来源 |
|---|---|---|
| adb | 37.0.0（随 scrcpy 官方预编译包附带） | https://github.com/Genymobile/scrcpy/releases/tag/v4.1 |
| scrcpy（Linux x86_64 预编译包） | v4.1 | 同上 |
| scrcpy（Windows win64 预编译包） | v4.1 | 同上 |

构建时会用 scrcpy 官方 `SHA256SUMS.txt` 校验下载的预编译包（SHA256 不匹配直接中止）。

各内嵌组件的许可证归其各自作者所有（adb/platform-tools 为 Apache-2.0，scrcpy 为 Apache-2.0），
本仓库仅分发打包后的运行时。

---

## 6. 源码结构

```
.
├── main.go                 # adbctl 全部逻辑（选路、mDNS、配对、应用模糊匹配、启动 scrcpy）
├── isatty_linux.go         # Linux：真正的 isatty（ioctl TCGETS）
├── isatty_windows.go       # Windows：GetConsoleMode
├── payload_linux.go        # //go:build linux  —— 内嵌 Linux payload
├── payload_windows.go      # //go:build windows —— 内嵌 Windows payload
├── tools/mkzip/main.go     # 把目录打成保留可执行位的 zip
├── build.sh                # 一键下载 + 组装 + 交叉编译
├── publish.sh              # 一条命令推源码到 GitHub 并发 Release（需 PAT）
├── RELEASING.md            # 推送到 GitHub / 发 Release 的步骤（含 PAT 获取）
├── adbctl-usage.md         # 完整用法说明
├── legacy/                 # 原 bash adbctl 与 pair_ip（仅供对照）
├── dist/                   # 构建输出（gitignore，Release 资源从这里上传）
└── payload/                # 下载的官方产物与生成的 payload zip（gitignore）
```

---

## 7. 与原脚本的差异

- 运行时不依赖 bash / awk / sed / grep 等外部命令，逻辑全部在 Go 内实现。
- `-a` 模糊匹配的分数规则、退出码、stdout/stderr 约定与原脚本保持一致。
- 多个候选地址现在会**去重**后逐个尝试（原脚本可能重复试同一地址）。
- 交互判定用真正的 `isatty`（Linux `ioctl(TCGETS)` / Windows `GetConsoleMode`），管道、`/dev/null`、CI 中不会卡住，与原脚本 `[ -t 0 ]` 一致。
- 新增 `--version`、`--print-paths`、`SCRCPY` 环境变量。

---

## 8. 许可证

包装程序源码：MIT（见 `LICENSE`）。内嵌二进制遵循各自许可证。
