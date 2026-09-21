# adbctl

把原 bash 脚本 `adbctl` 重写为 Go 程序：自动在「有 USB 走 USB，没有走无线调试」之间选路，
可一键配对、投屏、只流转单个应用。提供三种发行形态：

| 形态 | 文件 | 体积 | 说明 |
|---|---|---|---|
| Linux 内嵌单文件版 | `adbctl-linux-x86_64` | ~21 MB | adb/scrcpy 全部内嵌，开箱即用 |
| Linux 省空间部署版 | `adbctl-linux-x86_64-lite` | ~6.6 MB | 不内嵌，启动时扫描并按需下载依赖到用户缓存 |
| Windows 内嵌单文件版 | `adbctl-windows-x86_64.exe` | ~14.6 MB | adb/scrcpy 全部内嵌，开箱即用 |

> Windows 不提供「省空间部署版」：Windows 用户通常对安装位置和磁盘占用更敏感，也更偏好简单即用的东西，
> 一个开箱即用的单文件版就足够了。

---

## 1. 下载与使用

到本仓库的 **Releases** 页面下载对应文件：

| 平台 / 版本 | 文件 | 用法 |
|---|---|---|
| Linux 内嵌版 | `adbctl-linux-x86_64` | `chmod +x adbctl-linux-x86_64 && ./adbctl-linux-x86_64 -h` |
| Linux 省空间版 | `adbctl-linux-x86_64-lite` | `chmod +x adbctl-linux-x86_64-lite && ./adbctl-linux-x86_64-lite -h` |
| Windows | `adbctl-windows-x86_64.exe` | 放进任意目录，双击或命令行运行 `adbctl-windows-x86_64.exe -h` |

三个文件都是**便携版**：拷到 U 盘或任意路径即可用，不需要安装，也不需要预装 bash。

```bash
# Linux
./adbctl-linux-x86_64                 # 自动选路连接，打印选中的序列号或 IP:端口
./adbctl-linux-x86_64 -S              # 连上后投屏
./adbctl-linux-x86_64 -a 计算器 -S    # 只流转单个应用（虚拟显示，手机主屏不受影响）
./adbctl-linux-x86_64 -p 123456 -S    # 无线首次配对

# 省空间版用法完全相同，只是文件名不同
./adbctl-linux-x86_64-lite -S
```

```powershell
:: Windows
.\adbctl-windows-x86_64.exe -S
.\adbctl-windows-x86_64.exe -a 微信 -S
```

完整选项与行为见 `adbctl-usage.md`（与原脚本一致），本程序额外提供：

```
adbctl --version       # 版本
adbctl --print-paths   # 打印最终使用的 adb / scrcpy / scrcpy-server 路径
adbctl --check-deps    # 只扫描依赖（别名 --scan），不下载
```

### 输出约定

- **stdout 只有一样东西**：选中的目标（USB 序列号 或 无线 `IP:端口`）。
- **stderr 全部日志与提示**（包括交互选择界面）。

```bash
ADDR=$(./adbctl-linux-x86_64 -W)   # 变量拿到的是干净地址
```

---

## 2. 省空间部署版（Linux，`-lite`）

可执行文件本身只有约 6.6 MB，不内嵌任何依赖。每次启动按下面的顺序决定 adb / scrcpy 从哪来：

1. **缓存**：之前已经部署过就直接用，可离线（`\$XDG_CACHE_HOME/adbctl/1.0.0/bin`，通常是 `~/.cache/adbctl/1.0.0/bin`）；
2. **系统**：`PATH` 里**同时**有 `adb` 和 `scrcpy` 时直接用系统的，
   **不下载、不占额外空间**；
3. **按需下载**：缺哪个就下载官方 **scrcpy v4.1** 预编译包（自带 adb + scrcpy + scrcpy-server，
   约 18 MB），**校验官方 SHA256 通过后才解压**到用户缓存目录，之后一直复用。

先用它看看当前环境会怎么处理（**只扫描、不下载**）：

```bash
./adbctl-linux-x86_64-lite --check-deps
```

```text
模式：省空间部署版（按需下载，仅 Linux）
缓存目录：     /home/you/.cache/adbctl/1.0.0
系统 adb：     /home/you/Android/Sdk/platform-tools/adb
系统 scrcpy：  /usr/bin/scrcpy
已部署 adb：   （未找到）
已部署 scrcpy：（未找到）
结论：系统已有 adb 与 scrcpy，直接使用，无需下载。
```

- 部署位置用 `ADBCTL_CACHE` 可以改（默认 `~/.cache/adbctl/1.0.0`）。
- 删掉该目录，下次运行会重新下载部署。
- 下载默认走官方 GitHub；可用下面的变量换成镜像或本地文件，也遵循标准代理变量
  `HTTPS_PROXY` / `HTTP_PROXY`：
  | 变量 | 作用 |
  |---|---|
  | `ADBCTL_SCRCPY_URL` | scrcpy 预编译包下载地址 |
  | `ADBCTL_SCRCPY_SUMS_URL` | 官方 SHA256SUMS.txt 地址 |
- **注意**：官方 scrcpy 预编译包要求 glibc ≥ 2.35（Ubuntu 22.04+）。如果系统较老但自带 scrcpy/adb，
  会走「系统」这条路，不受影响。

---

## 3. 内嵌版依赖释放位置

内嵌版（`adbctl-linux-x86_64`、`adbctl-windows-x86_64.exe`）首次运行时把内嵌的
adb / scrcpy / scrcpy-server 释放到系统缓存目录：

| 平台 | 默认目录 |
|---|---|
| Linux | `\$XDG_CACHE_HOME/adbctl/1.0.0`（通常是 `~/.cache/adbctl/1.0.0`） |
| Windows | `%LOCALAPPDATA%\adbctl\1.0.0` |

可用 `ADBCTL_CACHE` 覆盖。删除该目录后下次运行会重新释放。

---

## 4. 环境变量

| 变量 | 默认 | 作用 |
|---|---|---|
| `ADB` | 内嵌 / 系统 adb | 指定外部 adb 可执行文件 |
| `SCRCPY` | 内嵌 / 系统 scrcpy | 指定外部 scrcpy 可执行文件 |
| `PAIR_IP` | 自己解析 mDNS | 指定取配对地址的命令 |
| `SCRCPY_ARGS` | 见下 | 整体替换 `-S` 时的 scrcpy 参数 |
| `ADBCTL_CACHE` | 系统缓存目录 | 依赖释放 / 部署目录 |
| `SCRCPY_SERVER_PATH` | 内嵌 / 下载的 server | 指定 scrcpy-server 路径 |
| `ADBCTL_SCRCPY_URL` | 官方地址 | 省空间版：scrcpy 包下载地址 |
| `ADBCTL_SCRCPY_SUMS_URL` | 官方地址 | 省空间版：SHA256 清单地址 |
| `HTTPS_PROXY` / `HTTP_PROXY` | 无 | 下载依赖时使用的代理 |

默认 scrcpy 档（实测于 Redmi K70 / 540x1200 / H.265）：

```
--video-codec=h265 --video-bit-rate=12M --max-size=1200 --max-fps=52 --video-buffer=0
```

---

## 5. 从源码构建

### 依赖

- Go 1.22+（本仓库无需任何第三方 Go 模块）
- `curl`、`tar`、`unzip`、`sha256sum`

### 一键构建

```bash
./build.sh
```

脚本会：

1. 下载官方产物并校验 scrcpy 的 SHA256；
2. 组装每个平台的运行目录（`bin/` + `lib/`）；
3. 用仓库内的小工具 `tools/mkzip` 打成 `payload/*.zip` 供 `go:embed` 内嵌；
4. 交叉编译出三个文件：
   - `dist/adbctl-linux-x86_64`：Linux 内嵌单文件版（默认构建，内嵌 payload）
   - `dist/adbctl-windows-x86_64.exe`：Windows 内嵌单文件版（默认构建，内嵌 payload）
   - `dist/adbctl-linux-x86_64-lite`：Linux 省空间部署版（`-tags lite`，不内嵌）

### 手动构建

```bash
# 先按 build.sh 组装好 .build/linux 与 .build/windows，再：
go run ./tools/mkzip .build/linux   payload/adbctl-payload-linux-amd64.zip
go run ./tools/mkzip .build/windows payload/adbctl-payload-windows-amd64.zip

# 内嵌单文件版
CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o dist/adbctl-linux-x86_64 .
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o dist/adbctl-windows-x86_64.exe .

# 省空间部署版（Linux，按需下载）
CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build -tags lite -trimpath -ldflags "-s -w" -o dist/adbctl-linux-x86_64-lite .
```

---

## 6. 依赖版本与来源

| 组件 | 版本 | 来源 |
|---|---|---|
| adb | 37.0.0（随 scrcpy 官方预编译包附带） | https://github.com/Genymobile/scrcpy/releases/tag/v4.1 |
| scrcpy（Linux x86_64 预编译包） | v4.1 | 同上 |
| scrcpy（Windows win64 预编译包） | v4.1 | 同上 |

内嵌版在**构建时**、省空间版在**运行时下载时**都会用 scrcpy 官方 `SHA256SUMS.txt` 校验
（SHA256 不匹配直接中止）。

各依赖的许可证归其各自作者所有（adb/platform-tools 为 Apache-2.0，scrcpy 为 Apache-2.0），
本仓库仅分发打包后的运行时。

---

## 7. 源码结构

```
.
├── main.go                 # adbctl 全部逻辑（选路、mDNS、配对、应用模糊匹配、启动 scrcpy）
├── lite.go                 # //go:build lite —— 省空间版：扫描依赖 + 下载 + SHA256 校验 + 解压
├── lite_stub.go            # //go:build !lite —— 内嵌版的空实现
├── isatty_linux.go         # Linux：真正的 isatty（ioctl TCGETS）
├── isatty_windows.go       # Windows：GetConsoleMode
├── payload_linux.go        # //go:build linux && !lite —— 内嵌 Linux payload
├── payload_lite_linux.go   # //go:build linux && lite —— 省空间版（payload 为空）
├── payload_windows.go      # //go:build windows —— 内嵌 Windows payload
├── tools/mkzip/main.go     # 把目录打成保留可执行位的 zip
├── build.sh                # 一键下载 + 组装 + 交叉编译（生成三个文件）
├── publish.sh              # 一条命令推源码到 GitHub 并发 Release（需 PAT）
├── RELEASING.md            # 推送到 GitHub / 发 Release 的步骤（含 PAT 获取）
├── adbctl-usage.md         # 完整用法说明
├── legacy/                 # 原 bash adbctl 与 pair_ip（仅供对照）
├── dist/                   # 构建输出（gitignore，Release 资源从这里上传）
└── payload/                # 下载的官方产物与生成的 payload zip（gitignore）
```

---

## 8. 与原脚本的差异

- 运行时不依赖 bash / awk / sed / grep 等外部命令，逻辑全部在 Go 内实现。
- `-a` 模糊匹配的分数规则、退出码、stdout/stderr 约定与原脚本保持一致。
- 多个候选地址现在会**去重**后逐个尝试（原脚本可能重复试同一地址）。
- 交互判定用真正的 `isatty`（Linux `ioctl(TCGETS)` / Windows `GetConsoleMode`），
  管道、`/dev/null`、CI 中不会卡住，与原脚本 `[ -t 0 ]` 一致。
- 新增 `--version`、`--print-paths`、`--check-deps`、`SCRCPY` 环境变量。
- 新增 Linux 省空间部署版：启动时扫描系统/缓存，缺依赖自动下载并校验后部署。

---

## 9. 许可证

包装程序源码：MIT（见 `LICENSE`）。内嵌/下载的二进制遵循各自许可证。
