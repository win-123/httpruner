package browser

// runtime.go — ydf-browser runtime 的发现与调用。
// Engine 抽象执行层：RuntimeEngine 通过调用已安装的 ydf-browser CLI 执行
// 浏览器自动化（与 Python 版行为 100% 一致）；未来可替换为基于 hrp uixt
// 的原生 Go 引擎，命令层无需改动。

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Engine 是浏览器自动化执行层抽象。
type Engine interface {
	// Name 返回引擎名称。
	Name() string
	// Run 使用给定参数执行，参数与 ydf-browser CLI 保持一致。
	Run(args []string) error
}

// RuntimeEngine 调用本机安装的 ydf-browser runtime。
type RuntimeEngine struct {
	// BinPath 是 ydf-browser 可执行文件路径。
	BinPath string
}

// Name 实现 Engine。
func (e *RuntimeEngine) Name() string { return "ydf-browser runtime (" + e.BinPath + ")" }

// Run 实现 Engine：透传执行 ydf-browser 命令，继承标准输入输出。
func (e *RuntimeEngine) Run(args []string) error {
	cmd := exec.Command(e.BinPath, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// LocateRuntime 按顺序查找 ydf-browser 可执行文件：
// 环境变量 YDF_BROWSER_BIN > PATH > ~/.local/bin > ~/.ydf-browser/bin。
func LocateRuntime() (string, error) {
	if custom := strings.TrimSpace(os.Getenv("YDF_BROWSER_BIN")); custom != "" {
		if info, err := os.Stat(custom); err == nil && !info.IsDir() {
			return custom, nil
		}
		return "", fmt.Errorf("YDF_BROWSER_BIN 指向的文件不存在: %s", custom)
	}
	if path, err := exec.LookPath("ydf-browser"); err == nil {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	candidates := []string{
		filepath.Join(home, ".local", "bin", "ydf-browser"),
		filepath.Join(home, ".ydf-browser", "bin", "ydf-browser"),
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("未找到 ydf-browser runtime；请先安装 ydf-browser，或通过 YDF_BROWSER_BIN 指定可执行文件路径")
}

// DefaultEngine 返回默认执行引擎（当前为 runtime 透传）。
func DefaultEngine() (Engine, error) {
	binPath, err := LocateRuntime()
	if err != nil {
		return nil, err
	}
	return &RuntimeEngine{BinPath: binPath}, nil
}
