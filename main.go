package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"golang.org/x/xerrors"

	"go.coder.com/flog"
)

func main() {
	skipSyncFlag := flag.Bool("skipsync", false, "skip syncing local settings and extensions to remote host")
	flag.Usage = func() {
		fmt.Printf(`Usage: [-skipsync] %v HOST [SSH ARGS...]

Start code-server over SSH.
More info: https://github.com/codercom/sshcode
`, os.Args[0])
	}

	flag.Parse()
	host := flag.Arg(0)

	if host == "" {
		// If no host is specified output the usage.
		flag.Usage()
		os.Exit(1)
	}

	flog.Info("ensuring code-server is updated...")

	// Downloads the latest code-server and allows it to be executed.
	sshCmd := exec.Command("ssh",
		"-tt",
		host,
		`/bin/bash -c 'set -euxo pipefail || exit 1
mkdir -p ~/bin
wget -q https://codesrv-ci.cdr.sh/latest-linux -O ~/bin/code-server
chmod +x ~/bin/code-server
mkdir -p ~/.local/share/code-server
'`,
	)
	output, err := sshCmd.CombinedOutput()
	if err != nil {
		flog.Fatal("failed to update code-server: %v: %s", err, output)
	}

	if !(*skipSyncFlag) {
		start := time.Now()
		flog.Info("syncing settings")
		err = syncUserSettings(host)
		if err != nil {
			flog.Fatal("failed to sync settings: %v", err)
		}
		flog.Info("synced settings in %s", time.Since(start))

		flog.Info("syncing extensions")
		err = syncExtensions(host)
		if err != nil {
			flog.Fatal("failed to sync extensions: %v", err)
		}
		flog.Info("synced extensions in %s", time.Since(start))
	}

	flog.Info("starting code-server...")
	localPort, err := scanAvailablePort()
	if err != nil {
		flog.Fatal("failed to scan available port: %v", err)
	}

	// Starts code-server and forwards the remote port.
	sshCmd = exec.Command("ssh",
		"-tt",
		"-q",
		"-L",
		localPort+":localhost:"+localPort,
		host,
		"~/bin/code-server --host 127.0.0.1 --allow-http --no-auth --port="+localPort,
	)
	sshCmd.Stdout = os.Stdout
	sshCmd.Stderr = os.Stderr
	err = sshCmd.Start()
	if err != nil {
		flog.Fatal("failed to start code-server: %v", err)
	}

	url := "http://127.0.0.1:" + localPort
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for {
		if ctx.Err() != nil {
			flog.Fatal("code-server didn't start in time %v", ctx.Err())
		}
		// Waits for code-server to be available before opening the browser.
		r, _ := http.NewRequest("GET", url, nil)
		r = r.WithContext(ctx)
		resp, err := http.DefaultClient.Do(r)
		if err != nil {
			continue
		}
		resp.Body.Close()
		break
	}

	openBrowser(url)
	sshCmd.Wait()
}

func openBrowser(url string) {
	var openCmd *exec.Cmd
	if commandExists("google-chrome") {
		openCmd = exec.Command("google-chrome", fmtChromeOptions(url)...)

	} else if commandExists("chromium") {
		openCmd = exec.Command("chromium", fmtChromeOptions(url)...)

	} else if commandExists("chromium-browser") {
		openCmd = exec.Command("chromium-browser", fmtChromeOptions(url)...)

	} else if commandExists("firefox") {
		openCmd = exec.Command("firefox", "--url="+url, "-safe-mode")

	} else {
		flog.Info("unable to find a browser to open: sshcode only supports firefox, chrome, and chromium")

		return
	}

	err := openCmd.Start()
	if err != nil {
		flog.Fatal("failed to open browser: %v", err)
	}
}

func fmtChromeOptions(url string) []string {
	return []string{"--app=" + url, "--disable-extensions", "--disable-plugins"}
}

// Checks if a command exists locally.
func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// scanAvailablePort scans 1024-4096 until an available port is found.
func scanAvailablePort() (string, error) {
	for port := 1024; port < 4096; port++ {
		l, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err != nil {
			// If we have an error the port is taken.
			continue
		}
		_ = l.Close()

		return strconv.Itoa(port), nil
	}

	return "", errors.New("no ports available")
}

func syncUserSettings(host string) error {
	localConfDir, err := configDir()
	if err != nil {
		return err
	}
	const remoteSettingsDir = ".local/share/code-server/User"

	// Append "/" to have rsync copy the contents of the dir.
	return rsync(localConfDir+"/", remoteSettingsDir, host, "workspaceStorage", "logs", "CachedData")
}

func syncExtensions(host string) error {
	localExtensionsDir, err := extensionsDir()
	if err != nil {
		return err
	}
	const remoteExtensionsDir = ".local/share/code-server/extensions"

	return rsync(localExtensionsDir+"/", remoteExtensionsDir, host)
}

func rsync(src string, dest string, host string, excludePaths ...string) error {
	remoteDest := fmt.Sprintf("%s:%s", host, dest)
	excludeFlags := make([]string, len(excludePaths))
	for i, path := range excludePaths {
		excludeFlags[i] = "--exclude=" + path
	}

	cmd := exec.Command("rsync", append(excludeFlags, "-azv", "--copy-unsafe-links", src, remoteDest)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return xerrors.Errorf("failed to rsync '%s' to '%s': %w", src, remoteDest, err)
	}

	return nil
}

func configDir() (string, error) {
	var path string
	switch runtime.GOOS {
	case "linux":
		path = os.ExpandEnv("$HOME/.config/Code/User")
	case "darwin":
		path = os.ExpandEnv("$HOME/Library/Application Support/Code/User")
	default:
		return "", xerrors.Errorf("unsupported platform: %s", runtime.GOOS)
	}
	return filepath.Clean(path), nil
}

func extensionsDir() (string, error) {
	var path string
	switch runtime.GOOS {
	case "linux", "darwin":
		path = os.ExpandEnv("$HOME/.vscode/extensions")
	default:
		return "", xerrors.Errorf("unsupported platform: %s", runtime.GOOS)
	}
	return filepath.Clean(path), nil
}
