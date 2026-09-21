// adbctl — Android 设备一键连接 / 投屏：有 USB 走 USB，没有才走无线调试。
//
// 有两种构建：
//  1. 内嵌单文件版（默认）：把 adb、scrcpy、scrcpy-server 全部内嵌进可执行文件，
//     首次运行释放到用户缓存目录，开箱即用，不需要预装任何东西；
//  2. 省空间部署版（-tags lite，仅 Linux）：可执行文件本身很小，启动时扫描系统与
//     缓存里有没有 adb / scrcpy，缺哪个就从官方源下载（校验 SHA256）部署到用户缓存。
package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const appVersion = "1.0.0"

const scrcpyArgsDefault = "--video-codec=h265 --video-bit-rate=12M --max-size=1200 --max-fps=52 --video-buffer=0"

// 省空间部署版按需下载的 scrcpy 版本（与 build.sh 保持一致）
const scrcpyVersion = "4.1"

const (
	connSvc = "_adb-tls-connect._tcp"
	pairSvc = "_adb-tls-pairing._tcp"
)

var ipPortRe = regexp.MustCompile("([0-9]{1,3}\\.){3}[0-9]{1,3}:[0-9]{1,5}")

var usageText = strings.Join([]string{
	"adbctl — Android 设备一键连接 / 投屏（内嵌单文件版 / 省空间部署版）",
	"",
	"  adbctl               # 自动选路：有 USB 就用 USB，否则连无线。stdout 只输出选中的序列号/地址",
	"  adbctl -S            # 连上后启动 scrcpy（参数见下方 SCRCPY_ARGS）",
	"  adbctl -a com.miui.calculator -S   # 只流转指定应用（虚拟显示，手机主屏不受影响）；-a 隐含 -S",
	"  adbctl -a 计算器 -S                # 模糊匹配：唯一命中直接用；多个则按匹配度列出序号让你选",
	"  adbctl -a +微信 -S                 # 前缀 + = 启动前先强杀该应用；? 前缀=模糊（本程序默认就是模糊）",
	"  adbctl --list-apps                 # 列出设备上全部应用（应用名 + 包名）",
	"  adbctl -U            # 强制走 USB；当前没有在位 USB 设备则报错退出",
	"  adbctl -W            # 强制走无线（忽略在位的 USB）",
	"  adbctl -p 748966     # 无线配对（748966 = 手机上显示的 6 位配对码）；隐含 -W",
	"  adbctl -l            # 只列出无线候选地址，不做任何连接（无线专用）",
	"  adbctl -w 30         # 最多等 30 秒等 mDNS 广播出现（默认 0 = 只查一次）",
	"  adbctl -h            # 帮助",
	"  adbctl --version     # 版本",
	"  adbctl --print-paths # 打印最终使用的 adb / scrcpy / server 路径",
	"  adbctl --check-deps  # 扫描依赖是否存在（省空间版；只扫描不下载，别名 --scan）",
	"",
	"这个程序会执行什么（副作用全部明列，只有这几条）:",
	"  * adb devices -l                    查询式，只读（判断有没有 USB）",
	"  * adb mdns services                 查询式，只读（仅无线路径）",
	"  * adb pair <配对地址> <配对码>      仅当给了 -p",
	"  * adb connect <候选地址>            仅无线路径，逐个候选试，成功即停",
	"  * adb shell -n pm list packages     仅 -a 的精确包名快速通道，只读",
	"  * scrcpy -s <目标> --list-apps      仅当 --list-apps，或 -a 需要解析应用名时",
	"  * scrcpy -s <目标> [--new-display --start-app=<包名>] $SCRCPY_ARGS",
	"                                      仅当给了 -S / -a",
	"  不写任何文件到当前目录；不用 sudo；不扫端口；除 adb 自身的连接外不联网。",
	"  （唯一写入：内嵌版首次运行释放依赖；省空间版缺依赖时下载到用户缓存）",
	"",
	"选路优先级:",
	"  -p / -l / -W  -> 无线",
	"  -U            -> USB（没有则退出码 1）",
	"  默认（无参数）-> 有 USB 设备就用 USB；没有才回落到无线",
	"  USB 判定依据：adb devices -l 中 state=device 且带 \" usb:\" 标记的那一行",
	"",
	"环境变量:",
	"  ADB                    指定 adb（默认用内嵌/系统的 adb）",
	"  SCRCPY                 指定 scrcpy（默认用内嵌/系统的 scrcpy）",
	"  PAIR_IP                指定取配对地址的命令（默认自己解析 mDNS）",
	"  SCRCPY_ARGS            覆盖 -S 时的 scrcpy 启动参数（默认见下）",
	"  ADBCTL_CACHE           依赖释放 / 部署目录（默认用户缓存目录）",
	"  ADBCTL_SCRCPY_URL      省空间版：scrcpy 包下载地址（可换镜像/本地文件）",
	"  ADBCTL_SCRCPY_SUMS_URL 省空间版：SHA256 清单地址",
	"",
	"scrcpy 默认档（2026-09-21 实测于 Redmi K70 / 540x1200 / H.265）:",
	"  " + scrcpyArgsDefault,
	"  VBR 内容自适应；音频默认转发到电脑（不想要声音加 --no-audio）",
	"",
	"应用名模糊匹配打分:",
	"  精确包名100 / 精确应用名95 / 应用名前缀85 / 包名末段82 / 包名含点号80 / 包名子串65 / 应用名子串60",
	"  唯一最高分直接用；多个并列则按匹配度列序号让你选；非交互终端会拒绝并提示用完整包名",
	"",
}, "\n")

var (
	payloadRoot   string
	payloadBinDir string
	adbPath       string
	scrcpyPath    string
	serverPath    string
	childEnv      []string
)

// depsReport 记录 --check-deps 扫描到的依赖来源（仅 lite 版会填充）。
type depsReport struct {
	sysAdb      string
	sysScrcpy   string
	cacheAdb    string
	cacheScrcpy string
	cacheRoot   string
}

func orNone(s string) string {
	if s == "" {
		return "（未找到）"
	}
	return s
}

func main() {
	os.Exit(run())
}

func run() int {
	route := "auto"
	doScrcpy := false
	doList := false
	doListApps := false
	pairCode := ""
	waitSec := 0
	app := ""

	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-S", "--scrcpy":
			doScrcpy = true
		case "-a", "--app":
			app = takeValue(args, &i)
			doScrcpy = true
		case "--list-apps":
			doListApps = true
			doScrcpy = true
		case "-U", "--usb":
			route = "usb"
		case "-W", "--wifi":
			route = "wifi"
		case "-l", "--list":
			doList = true
			route = "wifi"
		case "-p", "--pair":
			pairCode = takeValue(args, &i)
			route = "wifi"
		case "-w", "--wait":
			w := takeValue(args, &i)
			n, err := strconv.Atoi(w)
			if err != nil || n < 0 {
				fmt.Fprintln(os.Stderr, "adbctl: -w 需要一个秒数")
				return 2
			}
			waitSec = n
		case "-h", "--help":
			fmt.Print(usageText)
			return 0
		case "--version":
			fmt.Printf("adbctl %s\n", appVersion)
			return 0
		case "--print-paths":
			if err := setup(); err != nil {
				fmt.Fprintf(os.Stderr, "adbctl: %v\n", err)
				return 1
			}
			fmt.Printf("adb=%s\nscrcpy=%s\nserver=%s\nroot=%s\n", adbPath, scrcpyPath, serverPath, payloadRoot)
			return 0
		case "--check-deps", "--scan":
			if len(embeddedPayload) > 0 {
				if err := setup(); err != nil {
					fmt.Fprintf(os.Stderr, "adbctl: %v\n", err)
					return 1
				}
				fmt.Println("模式：内嵌单文件版（依赖已内嵌，无需扫描/下载）")
				fmt.Printf("adb=%s\nscrcpy=%s\nserver=%s\n", adbPath, scrcpyPath, serverPath)
				return 0
			}
			rep := scanDeps()
			fmt.Println("模式：省空间部署版（按需下载，仅 Linux）")
			fmt.Printf("缓存目录：     %s\n", rep.cacheRoot)
			fmt.Printf("系统 adb：     %s\n", orNone(rep.sysAdb))
			fmt.Printf("系统 scrcpy：  %s\n", orNone(rep.sysScrcpy))
			fmt.Printf("已部署 adb：   %s\n", orNone(rep.cacheAdb))
			fmt.Printf("已部署 scrcpy：%s\n", orNone(rep.cacheScrcpy))
			switch {
			case rep.cacheAdb != "" && rep.cacheScrcpy != "":
				fmt.Println("结论：使用缓存里已部署的依赖，无需下载。")
			case rep.sysAdb != "" && rep.sysScrcpy != "":
				fmt.Println("结论：系统已有 adb 与 scrcpy，直接使用，无需下载。")
			default:
				fmt.Printf("结论：缺少依赖，运行时会自动下载官方 scrcpy v%s（约 18MB）到缓存。\n", scrcpyVersion)
			}
			return 0
		default:
			fmt.Fprintf(os.Stderr, "adbctl: 未知参数 '%s'（-h 看用法）\n", args[i])
			return 2
		}
	}

	if err := setup(); err != nil {
		fmt.Fprintf(os.Stderr, "adbctl: %v\n", err)
		return 127
	}
	if err := checkExecutable(adbPath); err != nil {
		fmt.Fprintln(os.Stderr, "adbctl: 找不到 adb（可用 ADB=/path/to/adb 指定）")
		return 127
	}

	// ---------- USB 路径 ----------
	if route != "wifi" {
		usb := usbSerial()
		if usb != "" {
			fmt.Fprintf(os.Stderr, "adbctl: 检测到 USB 设备，走有线：%s\n", usb)
			fmt.Println(usb)
			return launchOrExit(usb, doScrcpy, doListApps, app)
		}
		if route == "usb" {
			fmt.Fprintln(os.Stderr, "adbctl: -U 指定走 USB，但当前没有在位的 USB 设备。")
			fmt.Fprintln(os.Stderr, "         检查：1) 数据线已插好  2) 手机的『USB 调试』已授权本机")
			return 1
		}
		fmt.Fprintln(os.Stderr, "adbctl: 没有 USB 设备，回落到无线调试。")
	}

	// ---------- 无线路径 ----------
	deadline := time.Now().Add(time.Duration(waitSec) * time.Second)
	var cands []string
	for {
		cands = connectCandidates()
		if len(cands) > 0 {
			break
		}
		if time.Now().Before(deadline) {
			time.Sleep(time.Second)
			continue
		}
		fmt.Fprintf(os.Stderr, "adbctl: 没有发现 %s 广播。检查：\n", connSvc)
		fmt.Fprintln(os.Stderr, "         1. 手机【无线调试】已开启（Android 11+）")
		fmt.Fprintln(os.Stderr, "         2. 与电脑同一网段，路由器没开 AP 隔离 / 访客网络隔离")
		fmt.Fprintln(os.Stderr, "         3. 电脑上别跑 avahi-daemon（会和 adb 抢 5353）")
		return 1
	}

	if doList {
		for _, c := range cands {
			fmt.Println(c)
		}
		return 0
	}

	if pairCode != "" {
		paddr := pairingAddress()
		if paddr == "" {
			fmt.Fprintln(os.Stderr, "adbctl: 没找到配对服务。请在手机上打开『使用配对码配对设备』对话框后重试（-w N 可等待）。")
			return 1
		}
		fmt.Fprintf(os.Stderr, "adbctl: 配对 %s …\n", paddr)
		out, err := adbCaptureAll("pair", paddr, pairCode)
		fmt.Fprint(os.Stderr, out)
		if err != nil {
			fmt.Fprintln(os.Stderr, "adbctl: 配对失败：核对 6 位配对码，并确认配对对话框仍开着。")
			return 1
		}
		time.Sleep(time.Second)
		cands = connectCandidates()
		if len(cands) == 0 {
			fmt.Fprintln(os.Stderr, "adbctl: 配对成功，但没看到连接服务，请稍后重试连接。")
			return 1
		}
	}

	ok := ""
	for _, a := range cands {
		out, _ := adbCaptureAll("connect", a)
		if strings.Contains(out, "connected to") {
			fmt.Println(a)
			ok = a
			break
		}
		first := out
		if idx := strings.IndexByte(out, '\n'); idx >= 0 {
			first = out[:idx]
		}
		fmt.Fprintf(os.Stderr, "adbctl: %s 不可用（%s）\n", a, strings.TrimSpace(first))
	}
	if ok == "" {
		fmt.Fprintln(os.Stderr, "adbctl: 所有候选地址都连不上。")
		fmt.Fprintln(os.Stderr, "         如果刚开关过无线调试，等几秒再试；必要时在手机上重新配对。")
		return 1
	}
	return launchOrExit(ok, doScrcpy, doListApps, app)
}

// takeValue 取出当前选项的值；没有下一个参数时返回空串（与原脚本行为一致）。
func takeValue(args []string, i *int) string {
	if *i+1 < len(args) {
		*i++
		return args[*i]
	}
	return ""
}

// ---------------- 内嵌依赖释放 ----------------

func setup() error {
	if len(embeddedPayload) > 0 {
		root, err := ensurePayload()
		if err != nil {
			return err
		}
		payloadRoot = root
		payloadBinDir = filepath.Join(root, "bin")
		adbPath = filepath.Join(payloadBinDir, exeName("adb"))
		scrcpyPath = filepath.Join(payloadBinDir, exeName("scrcpy"))
		serverPath = filepath.Join(payloadBinDir, "scrcpy-server")
	} else {
		if err := resolveExternal(); err != nil {
			return err
		}
	}
	if v := os.Getenv("ADB"); v != "" {
		adbPath = v
	}
	if v := os.Getenv("SCRCPY"); v != "" {
		scrcpyPath = v
	}
	childEnv = buildEnv(payloadBinDir)
	return nil
}

func exeName(base string) string {
	if isWindows() {
		return base + ".exe"
	}
	return base
}

func cacheHome() string {
	if v := os.Getenv("ADBCTL_CACHE"); v != "" {
		return v
	}
	base, err := os.UserCacheDir()
	if err != nil || base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "adbctl", appVersion)
}

func ensurePayload() (string, error) {
	dir := cacheHome()
	marker := filepath.Join(dir, ".extracted-"+appVersion)
	if st, err := os.Stat(marker); err == nil && !st.IsDir() {
		return dir, nil
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return "", fmt.Errorf("无法创建缓存目录: %w", err)
	}
	tmp, err := os.MkdirTemp(filepath.Dir(dir), "adbctl-extract-")
	if err != nil {
		return "", fmt.Errorf("无法创建临时目录: %w", err)
	}
	if err := extractZip(embeddedPayload, tmp); err != nil {
		os.RemoveAll(tmp)
		return "", fmt.Errorf("释放内嵌依赖失败: %w", err)
	}
	_ = os.RemoveAll(dir)
	if err := os.Rename(tmp, dir); err != nil {
		os.RemoveAll(tmp)
		if _, e2 := os.Stat(marker); e2 != nil {
			return "", fmt.Errorf("无法放置内嵌依赖: %w", err)
		}
		return dir, nil
	}
	_ = os.WriteFile(marker, []byte("ok\n"), 0o644)
	return dir, nil
}

func extractZip(data []byte, dest string) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	for _, f := range zr.File {
		name := filepath.FromSlash(f.Name)
		if name == "" || strings.Contains(name, "..") || filepath.IsAbs(name) {
			continue
		}
		target := filepath.Join(dest, name)
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		mode := f.Mode()
		if mode == 0 {
			mode = 0o644
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
		if err != nil {
			rc.Close()
			return err
		}
		if _, err := io.Copy(out, rc); err != nil {
			out.Close()
			rc.Close()
			return err
		}
		out.Close()
		rc.Close()
		if !isWindows() && strings.HasPrefix(filepath.ToSlash(name), "bin/") {
			_ = os.Chmod(target, 0o755)
		}
	}
	return nil
}

func buildEnv(binDir string) []string {
	env := os.Environ()
	if binDir != "" {
		env = prependPath(env, "PATH", binDir)
	}
	if payloadRoot != "" {
		libDir := filepath.Join(payloadRoot, "lib")
		if st, err := os.Stat(libDir); err == nil && st.IsDir() {
			env = prependPath(env, "LD_LIBRARY_PATH", libDir)
		}
	}
	// 只有确实存在 scrcpy-server 时才设置，避免污染系统 scrcpy 的查找
	if serverPath != "" && os.Getenv("SCRCPY_SERVER_PATH") == "" {
		if _, err := os.Stat(serverPath); err == nil {
			env = setEnv(env, "SCRCPY_SERVER_PATH", serverPath)
		}
	}
	return env
}

func prependPath(env []string, key, value string) []string {
	for i, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		if strings.EqualFold(k, key) {
			if v == "" {
				env[i] = k + "=" + value
			} else {
				env[i] = k + "=" + value + string(os.PathListSeparator) + v
			}
			return env
		}
	}
	return append(env, key+"="+value)
}

func setEnv(env []string, key, value string) []string {
	for i, kv := range env {
		k, _, _ := strings.Cut(kv, "=")
		if strings.EqualFold(k, key) {
			env[i] = key + "=" + value
			return env
		}
	}
	return append(env, key+"="+value)
}

func checkExecutable(path string) error {
	if strings.ContainsAny(path, "/\\") {
		st, err := os.Stat(path)
		if err != nil {
			return err
		}
		if st.IsDir() {
			return fmt.Errorf("%s 是目录", path)
		}
		return nil
	}
	_, err := exec.LookPath(path)
	return err
}

// ---------------- 子进程 ----------------

func adbCaptureAll(args ...string) (string, error) {
	cmd := exec.Command(adbPath, args...)
	cmd.Env = childEnv
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func adbCaptureOut(args ...string) (string, error) {
	cmd := exec.Command(adbPath, args...)
	cmd.Env = childEnv
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = io.Discard
	err := cmd.Run()
	return buf.String(), err
}

func scrcpyCaptureOut(args ...string) (string, error) {
	cmd := exec.Command(scrcpyPath, args...)
	cmd.Env = childEnv
	cmd.Stdin = nil
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = io.Discard
	err := cmd.Run()
	return buf.String(), err
}

// runAttached 前台运行子进程并把终端交给它，返回其退出码。
func runAttached(path string, args []string) int {
	cmd := exec.Command(path, args...)
	cmd.Env = childEnv
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "adbctl: 无法启动 %s: %v\n", path, err)
		return 1
	}
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case s := <-sigCh:
				_ = cmd.Process.Signal(s)
			case <-done:
				return
			}
		}
	}()
	err := cmd.Wait()
	close(done)
	signal.Stop(sigCh)
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "adbctl: %s 运行失败: %v\n", path, err)
		return 1
	}
	return 0
}

// ---------------- 设备发现 ----------------

func mdnsRaw() string {
	out, _ := adbCaptureAll("mdns", "services")
	return strings.ReplaceAll(out, "\r", "")
}

func extractAddrs(raw, service string) []string {
	var out []string
	for _, line := range strings.Split(raw, "\n") {
		if !strings.Contains(line, service) {
			continue
		}
		out = append(out, ipPortRe.FindAllString(line, -1)...)
	}
	return out
}

func connectCandidates() []string {
	return dedupe(extractAddrs(mdnsRaw(), connSvc))
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func pairingAddress() string {
	if cmd := os.Getenv("PAIR_IP"); cmd != "" {
		if p, err := exec.LookPath(cmd); err == nil {
			c := exec.Command(p)
			c.Env = childEnv
			out, err := c.Output()
			if err == nil {
				line := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
				if line != "" {
					return line
				}
			}
		}
	}
	addrs := extractAddrs(mdnsRaw(), pairSvc)
	if len(addrs) > 0 {
		return addrs[0]
	}
	return ""
}

func usbSerial() string {
	out, _ := adbCaptureOut("devices", "-l")
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == "device" && strings.Contains(line, " usb:") {
			return fields[0]
		}
	}
	out, _ = adbCaptureOut("devices")
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == "device" &&
			!strings.Contains(fields[0], ":") && !strings.Contains(fields[0], "_adb-tls-") {
			return fields[0]
		}
	}
	return ""
}

// ---------------- 应用解析 ----------------

type appEntry struct {
	pkg   string
	label string
	score int
}

func parseAppSpec(spec string) (string, bool) {
	stop := false
	for {
		switch {
		case strings.HasPrefix(spec, "+"):
			stop = true
			spec = spec[1:]
		case strings.HasPrefix(spec, "?"):
			spec = spec[1:]
		default:
			return spec, stop
		}
	}
}

func appsWithLabels(target string) []appEntry {
	out, _ := scrcpyCaptureOut("-s", target, "--list-apps")
	var apps []appEntry
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if !strings.HasPrefix(line, " - ") {
			continue
		}
		line = line[3:]
		trimmed := strings.TrimRight(line, " \t")
		if trimmed == "" {
			continue
		}
		var pkg, label string
		if i := strings.LastIndexAny(trimmed, " \t"); i >= 0 {
			pkg = trimmed[i+1:]
			label = strings.TrimRight(trimmed[:i], " \t")
		} else {
			pkg = trimmed
			label = trimmed
		}
		if pkg == "" {
			continue
		}
		apps = append(apps, appEntry{pkg: pkg, label: label})
	}
	return apps
}

func scoreApps(q string, apps []appEntry) []appEntry {
	var out []appEntry
	for _, a := range apps {
		lp := strings.ToLower(a.pkg)
		ll := strings.ToLower(a.label)
		s := 0
		switch {
		case lp == q:
			s = 100
		case ll == q:
			s = 95
		case strings.HasPrefix(ll, q):
			s = 85
		default:
			parts := strings.Split(lp, ".")
			switch {
			case parts[len(parts)-1] == q:
				s = 82
			case strings.Contains(lp, "."+q):
				s = 80
			case strings.Contains(lp, q):
				s = 65
			case strings.Contains(ll, q):
				s = 60
			}
		}
		if s > 0 {
			a.score = s
			out = append(out, a)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].score > out[j].score })
	return out
}

// resolveApp 返回选中的包名与退出码：0 成功，1 无匹配，2 需选择但不可选/序号无效，3 用户取消。
func resolveApp(target, q string) (string, int) {
	qLC := strings.ToLower(q)

	if out, err := adbCaptureOut("-s", target, "shell", "-n", "pm", "list", "packages"); err == nil {
		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimRight(line, "\r")
			if !strings.HasPrefix(line, "package:") {
				continue
			}
			pkg := strings.TrimPrefix(line, "package:")
			if strings.EqualFold(pkg, q) {
				return pkg, 0
			}
		}
	}

	fmt.Fprintln(os.Stderr, "adbctl: 正在获取设备应用清单（约 1~2 秒）…")
	all := scoreApps(qLC, appsWithLabels(target))
	if len(all) == 0 {
		return "", 1
	}

	top := all[0].score
	cnt := 0
	for _, a := range all {
		if a.score == top {
			cnt++
		}
	}
	if cnt == 1 {
		return all[0].pkg, 0
	}

	total := len(all)
	shown := total
	if shown > 30 {
		shown = 30
	}
	if !stdinIsTerminal() {
		fmt.Fprintf(os.Stderr, "adbctl: 匹配到 %d 个应用，但当前不是交互终端，无法选择。请改用完整包名：\n", total)
		for _, a := range all[:shown] {
			fmt.Fprintf(os.Stderr, "         %-36s %s\n", a.label, a.pkg)
		}
		return "", 2
	}
	fmt.Fprintf(os.Stderr, "adbctl: 匹配到 %d 个应用（按匹配度排序）:\n", total)
	for i, a := range all[:shown] {
		fmt.Fprintf(os.Stderr, "  %2d) %-36s %s\n", i+1, a.label, a.pkg)
	}
	if total > 30 {
		fmt.Fprintf(os.Stderr, "        …共 %d 个，上面只显示前 30 个；也可直接输入后续序号，或换更精确的关键词\n", total)
	}
	fmt.Fprintf(os.Stderr, "请输入序号 [1-%d]，q 取消: ", shown)
	ans := readLine()
	switch {
	case ans == "" || ans == "q" || ans == "Q":
		fmt.Fprintln(os.Stderr, "adbctl: 已取消。")
		return "", 3
	}
	n, err := strconv.Atoi(ans)
	if err != nil {
		fmt.Fprintln(os.Stderr, "adbctl: 序号无效。")
		return "", 2
	}
	if n < 1 || n > total {
		fmt.Fprintf(os.Stderr, "adbctl: 序号超出范围（1-%d）。\n", total)
		return "", 2
	}
	return all[n-1].pkg, 0
}

func stdinIsTerminal() bool {
	return isTerminal(os.Stdin.Fd())
}

func readLine() string {
	var buf [1]byte
	var sb strings.Builder
	for {
		n, err := os.Stdin.Read(buf[:])
		if n > 0 {
			if buf[0] == '\n' {
				break
			}
			if buf[0] != '\r' {
				sb.WriteByte(buf[0])
			}
		}
		if err != nil {
			break
		}
	}
	return strings.TrimSpace(sb.String())
}

// ---------------- 启动 ----------------

func launchOrExit(target string, doScrcpy, doListApps bool, app string) int {
	if doListApps {
		return runAttached(scrcpyPath, []string{"-s", target, "--list-apps"})
	}
	var appargs []string
	if app != "" {
		spec, stop := parseAppSpec(app)
		if spec == "" {
			fmt.Fprintln(os.Stderr, "adbctl: -a 需要应用名或包名（-h 看用法）")
			return 2
		}
		pkg, rc := resolveApp(target, spec)
		if rc != 0 {
			if rc == 1 {
				fmt.Fprintf(os.Stderr, "adbctl: 没有匹配到应用「%s」。用 adbctl --list-apps 查看全部应用。\n", spec)
			}
			return rc
		}
		fmt.Fprintf(os.Stderr, "adbctl: 应用已解析 -> %s\n", pkg)
		if !strings.Contains(" "+scrcpyArgsValue()+" ", " --new-display") {
			appargs = append(appargs, "--new-display")
		}
		if stop {
			appargs = append(appargs, "--start-app=+"+pkg)
		} else {
			appargs = append(appargs, "--start-app="+pkg)
		}
		fmt.Fprintf(os.Stderr, "adbctl: 单应用模式：虚拟显示 + 启动 %s（手机主屏不受影响）\n", pkg)
	}
	if doScrcpy {
		args := append([]string{"-s", target}, appargs...)
		args = append(args, strings.Fields(scrcpyArgsValue())...)
		fmt.Fprintf(os.Stderr, "adbctl: 启动 scrcpy -s %s %s\n", target, strings.Join(args[2:], " "))
		return runAttached(scrcpyPath, args)
	}
	return 0
}

func scrcpyArgsValue() string {
	if v, ok := os.LookupEnv("SCRCPY_ARGS"); ok {
		return v
	}
	return scrcpyArgsDefault
}
