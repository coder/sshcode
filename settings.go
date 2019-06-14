package main

import (
	"os"
	"path/filepath"
	"runtime"

	"golang.org/x/xerrors"
)

const (
	vsCodeConfigDirEnv     = "VSCODE_CONFIG_DIR"
	vsCodeExtensionsDirEnv = "VSCODE_EXTENSIONS_DIR"
)

func configDir() (string, error) {
	if env, ok := os.LookupEnv(vsCodeConfigDirEnv); ok {
		return os.ExpandEnv(env), nil
	}

	var path string
	switch runtime.GOOS {
	case "linux":
		path = os.ExpandEnv("$HOME/.config/Code/User/")
	case "darwin":
		path = os.ExpandEnv("$HOME/Library/Application Support/Code/User/")
	case "windows":
		// Can't use the filepath.Clean function to keep Linux format path that works well with gitbash
		return gitbashWindowsDir(os.ExpandEnv("$HOME/.config/Code/User")), nil
	default:
		return "", xerrors.Errorf("unsupported platform: %s", runtime.GOOS)
	}
	return filepath.Clean(path), nil
}

func extensionsDir() (string, error) {
	if env, ok := os.LookupEnv(vsCodeExtensionsDirEnv); ok {
		return os.ExpandEnv(env), nil
	}

	var path string
	switch runtime.GOOS {
	case "linux", "darwin":
		path = os.ExpandEnv("$HOME/.vscode/extensions/")
	case "windows":
		// Can't use the filepath.Clean function to keep Linux format path that works well with gitbash
		return gitbashWindowsDir(os.ExpandEnv("$HOME/.vscode/extensions")), nil
	default:
		return "", xerrors.Errorf("unsupported platform: %s", runtime.GOOS)
	}
	return filepath.Clean(path), nil
}
