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

func configDir(insiders bool) (string, error) {
	if env, ok := os.LookupEnv(vsCodeConfigDirEnv); ok {
		return os.ExpandEnv(env), nil
	}

	var basePath string
	switch runtime.GOOS {
	case "linux":
		basePath = os.ExpandEnv("$HOME/.config")
	case "darwin":
		basePath = os.ExpandEnv("$HOME/Library/Application Support")
	default:
		return "", xerrors.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	if insiders {
		return filepath.Join(basePath, "Code - Insiders", "User"), nil
	}

	return filepath.Join(basePath, "Code", "User"), nil
}

func extensionsDir(insiders bool) (string, error) {
	if env, ok := os.LookupEnv(vsCodeExtensionsDirEnv); ok {
		return os.ExpandEnv(env), nil
	}

	var basePath string
	switch runtime.GOOS {
	case "linux", "darwin":
		basePath = os.ExpandEnv("$HOME")
	default:
		return "", xerrors.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	if insiders {
		return filepath.Join(basePath, ".vscode-insiders", "extensions"), nil
	}

	return filepath.Join(basePath, ".vscode", "extensions"), nil
}
