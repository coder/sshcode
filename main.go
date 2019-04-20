package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/pkg/browser"
	"go.coder.com/flog"
	"golang.org/x/xerrors"
)

func init() {
	rand.Seed(time.Now().Unix())
}

func main() {
	var (
		skipSyncFlag = flag.Bool("skipsync", false, "skip syncing local settings and extensions to remote host")
		sshFlags     = flag.String("ssh-flags", "", "custom SSH flags")
		syncBack     = flag.Bool("b", false, "sync extensions back on termination")
	)

	flag.Usage = func() {
		fmt.Printf(`Usage: [-skipsync] %v HOST [DIR] [SSH ARGS...]

Start code-server over SSH.
More info: https://github.com/codercom/sshcode
`, os.Args[0],
		)
	}

	flag.Parse()
	host := flag.Arg(0)

	if host == "" {
		// If no host is specified output the usage.
		flag.Usage()
		os.Exit(1)
	}

	dir := flag.Arg(1)
	if dir == "" {
		dir = "~"
	}

	flog.Info("ensuring code-server is updated...")

	const codeServerPath = "/tmp/codessh-code-server"

	// Downloads the latest code-server and allows it to be executed.
	sshCmd := exec.Command("ssh",
		"-tt",
		host,
		`/bin/bash -c 'set -euxo pipefail || exit 1
# Make sure any currently running code-server is gone so we can overwrite
# the binary.
pkill -9 `+filepath.Base(codeServerPath)+` || true
wget -q https://codesrv-ci.cdr.sh/latest-linux -O `+codeServerPath+`
mkdir -p ~/.local/share/code-server
cd `+filepath.Dir(codeServerPath)+`
wget -N https://codesrv-ci.cdr.sh/latest-linux
[ -f `+codeServerPath+` ] && rm `+codeServerPath+`
ln latest-linux `+codeServerPath+`
chmod +x `+codeServerPath+`
'`,
	)
	sshCmd.Stdout = os.Stdout
	sshCmd.Stderr = os.Stderr
	err := sshCmd.Run()
	if err != nil {
		flog.Fatal("failed to update code-server: %v", err)
	}

	if !*skipSyncFlag {
		start := time.Now()
		flog.Info("syncing settings")
		err = syncUserSettings(host, false)
		if err != nil {
			flog.Fatal("failed to sync settings: %v", err)
		}
		flog.Info("synced settings in %s", time.Since(start))

		flog.Info("syncing extensions")
		err = syncExtensions(host, false)
		if err != nil {
			flog.Fatal("failed to sync extensions: %v", err)
		}
		flog.Info("synced extensions in %s", time.Since(start))
	}

	flog.Info("starting code-server...")
	localPort, err := randomPort()
	if err != nil {
		flog.Fatal("failed to find available port: %v", err)
	}

	sshCmdStr := fmt.Sprintf("ssh -tt -q -L %v %v %v 'cd %v; %v --host 127.0.0.1 --allow-http --no-auth --port=%v'",
		localPort+":localhost:"+localPort, *sshFlags, host, dir, codeServerPath, localPort,
	)

	// Starts code-server and forwards the remote port.
	sshCmd = exec.Command("sh", "-c",
		sshCmdStr,
	)
	sshCmd.Stdin = os.Stdin
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

	ctx, cancel = context.WithCancel(context.Background())
	openBrowser(url)

	go func() {
		defer cancel()
		sshCmd.Wait()
	}()

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt)

	select {
	case <-ctx.Done():
	case <-c:
	}

	if !*syncBack {
		flog.Info("shutting down")
		return
	}

	flog.Info("synchronizing VS Code back to local")

	err = syncExtensions(host, true)
	if err != nil {
		flog.Fatal("failed to sync extensions back: %v", err)
	}

	err = syncUserSettings(host, true)
	if err != nil {
		flog.Fatal("failed to user settigns extensions back: %v", err)
	}
}

func openBrowser(url string) {
	var openCmd *exec.Cmd
	switch {
	case commandExists("google-chrome"):
		openCmd = exec.Command("google-chrome", chromeOptions(url)...)
	case commandExists("chromium"):
		openCmd = exec.Command("chromium", chromeOptions(url)...)
	case commandExists("chromium-browser"):
		openCmd = exec.Command("chromium-browser", chromeOptions(url)...)
	case pathExists("/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"):
		openCmd = exec.Command("/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", chromeOptions(url)...)
	default:
		err := browser.OpenURL(url)
		if err != nil {
			flog.Error("failed to open browser: %v", err)
		}
		return
	}

	// We do not use CombinedOutput because if there is no chrome instance, this will block
	// and become the parent process instead of using an existing chrome instance.
	err := openCmd.Start()
	if err != nil {
		flog.Error("failed to open browser: %v", err)
	}
}

func chromeOptions(url string) []string {
	return []string{"--app=" + url, "--disable-extensions", "--disable-plugins", "--incognito"}
}

// Checks if a command exists locally.
func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func pathExists(name string) bool {
	_, err := os.Stat(name)
	return err == nil
}

// randomPort picks a random port to start code-server on.
func randomPort() (string, error) {
	const (
		minPort  = 1024
		maxPort  = 65535
		maxTries = 10
	)
	for i := 0; i < maxTries; i++ {
		port := rand.Intn(maxPort-minPort+1) + minPort
		l, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err == nil {
			_ = l.Close()
			return strconv.Itoa(port), nil
		}
		flog.Info("port taken: %d", port)
	}

	return "", xerrors.Errorf("max number of tries exceeded: %d", maxTries)
}

func syncUserSettings(host string, back bool) error {
	localConfDir, err := configDir()
	if err != nil {
		return err
	}
	const remoteSettingsDir = ".local/share/code-server/User/"

	var (
		src  = localConfDir + "/"
		dest = host + ":" + remoteSettingsDir
	)

	if back {
		dest, src = src, dest
	}

	// Append "/" to have rsync copy the contents of the dir.
	return rsync(src, dest, "workspaceStorage", "logs", "CachedData")
}

func syncExtensions(host string, back bool) error {
	localExtensionsDir, err := extensionsDir()
	if err != nil {
		return err
	}
	const remoteExtensionsDir = ".local/share/code-server/extensions/"

	var (
		src  = localExtensionsDir + "/"
		dest = host + ":" + remoteExtensionsDir
	)
	if back {
		dest, src = src, dest
	}

	return rsync(src, dest)
}

func rsync(src string, dest string, excludePaths ...string) error {
	excludeFlags := make([]string, len(excludePaths))
	for i, path := range excludePaths {
		excludeFlags[i] = "--exclude=" + path
	}

	cmd := exec.Command("rsync", append(excludeFlags, "-azvr",
		// Only update newer directories, and sync times
		// to keep things simple.
		"-u", "--times",
		"--copy-unsafe-links",
		src, dest,
	)...,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return xerrors.Errorf("failed to rsync '%s' to '%s': %w", src, dest, err)
	}

	return nil
}

func configDir() (string, error) {
	var path string
	switch runtime.GOOS {
	case "linux":
		path = os.ExpandEnv("$HOME/.config/Code/User/")
	case "darwin":
		path = os.ExpandEnv("$HOME/Library/Application Support/Code/User/")
	default:
		return "", xerrors.Errorf("unsupported platform: %s", runtime.GOOS)
	}
	return filepath.Clean(path), nil
}

func extensionsDir() (string, error) {
	var path string
	switch runtime.GOOS {
	case "linux", "darwin":
		path = os.ExpandEnv("$HOME/.vscode/extensions/")
	default:
		return "", xerrors.Errorf("unsupported platform: %s", runtime.GOOS)
	}
	return filepath.Clean(path), nil
}
