package main

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/pkg/browser"
	"go.coder.com/flog"
	"golang.org/x/xerrors"
)

const codeServerPath = "~/.cache/sshcode/sshcode-server"

const (
	sshDirectory               = "~/.ssh"
	sshDirectoryUnsafeModeMask = 0022
	sshControlPath             = sshDirectory + "/control-%h-%p-%r"
)

type options struct {
	skipSync         bool
	syncBack         bool
	noOpen           bool
	reuseConnection  bool
	bindAddr         string
	remotePort       string
	sshFlags         string
	uploadCodeServer string
}

func sshCode(host, dir string, o options) error {
	host, extraSSHFlags, err := parseHost(host)
	if err != nil {
		return xerrors.Errorf("failed to parse host IP: %w", err)
	}
	if extraSSHFlags != "" {
		o.sshFlags = strings.Join([]string{extraSSHFlags, o.sshFlags}, " ")
	}

	o.bindAddr, err = parseBindAddr(o.bindAddr)
	if err != nil {
		return xerrors.Errorf("failed to parse bind address: %w", err)
	}

	if o.remotePort == "" {
		o.remotePort, err = randomPort()
	}
	if err != nil {
		return xerrors.Errorf("failed to find available remote port: %w", err)
	}

	// Check the SSH directory's permissions and warn the user if it is not safe.
	o.reuseConnection = checkSSHDirectory(sshDirectory, o.reuseConnection)

	// Start SSH master connection socket. This prevents multiple password prompts from appearing as authentication
	// only happens on the initial connection.
	if o.reuseConnection {
		flog.Info("starting SSH master connection...")
		newSSHFlags, cancel, err := startSSHMaster(o.sshFlags, sshControlPath, host)
		defer cancel()
		if err != nil {
			flog.Error("failed to start SSH master connection: %v", err)
			o.reuseConnection = false
		} else {
			o.sshFlags = newSSHFlags
		}
	}

	// Upload local code-server or download code-server from CI server.
	if o.uploadCodeServer != "" {
		flog.Info("uploading local code-server binary...")
		err = copyCodeServerBinary(o.sshFlags, host, o.uploadCodeServer, codeServerPath)
		if err != nil {
			return xerrors.Errorf("failed to upload local code-server binary to remote server: %w", err)
		}

		sshCmdStr :=
			fmt.Sprintf("ssh %v %v 'chmod +x %v'",
				o.sshFlags, host, codeServerPath,
			)

		sshCmd := exec.Command("sh", "-l", "-c", sshCmdStr)
		sshCmd.Stdout = os.Stdout
		sshCmd.Stderr = os.Stderr
		err = sshCmd.Run()
		if err != nil {
			return xerrors.Errorf("failed to make code-server binary executable:\n---ssh cmd---\n%s: %w",
				sshCmdStr,
				err,
			)
		}
	} else {
		flog.Info("ensuring code-server is updated...")
		dlScript := downloadScript(codeServerPath)

		// Downloads the latest code-server and allows it to be executed.
		sshCmdStr := fmt.Sprintf("ssh %v %v '/usr/bin/env bash -l'", o.sshFlags, host)

		sshCmd := exec.Command("sh", "-l", "-c", sshCmdStr)
		sshCmd.Stdout = os.Stdout
		sshCmd.Stderr = os.Stderr
		sshCmd.Stdin = strings.NewReader(dlScript)
		err = sshCmd.Run()
		if err != nil {
			return xerrors.Errorf("failed to update code-server:\n---ssh cmd---\n%s"+
				"\n---download script---\n%s: %w",
				sshCmdStr,
				dlScript,
				err,
			)
		}
	}

	if !o.skipSync {
		start := time.Now()
		flog.Info("syncing settings")
		err = syncUserSettings(o.sshFlags, host, false)
		if err != nil {
			return xerrors.Errorf("failed to sync settings: %w", err)
		}

		flog.Info("synced settings in %s", time.Since(start))

		flog.Info("syncing extensions")
		err = syncExtensions(o.sshFlags, host, false)
		if err != nil {
			return xerrors.Errorf("failed to sync extensions: %w", err)
		}
		flog.Info("synced extensions in %s", time.Since(start))
	}

	flog.Info("starting code-server...")

	flog.Info("Tunneling remote port %v to %v", o.remotePort, o.bindAddr)

	sshCmdStr :=
		fmt.Sprintf("ssh -tt -q -L %v:localhost:%v %v %v 'cd %v; %v --host 127.0.0.1 --auth none --port=%v'",
			o.bindAddr, o.remotePort, o.sshFlags, host, dir, codeServerPath, o.remotePort,
		)

	// Starts code-server and forwards the remote port.
	sshCmd := exec.Command("sh", "-l", "-c", sshCmdStr)
	sshCmd.Stdin = os.Stdin
	sshCmd.Stdout = os.Stdout
	sshCmd.Stderr = os.Stderr
	err = sshCmd.Start()
	if err != nil {
		return xerrors.Errorf("failed to start code-server: %w", err)
	}

	url := fmt.Sprintf("http://%s", o.bindAddr)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	client := http.Client{
		Timeout: time.Second * 3,
	}
	for {
		if ctx.Err() != nil {
			return xerrors.Errorf("code-server didn't start in time: %w", ctx.Err())
		}
		// Waits for code-server to be available before opening the browser.
		resp, err := client.Get(url)
		if err != nil {
			continue
		}
		resp.Body.Close()
		break
	}

	ctx, cancel = context.WithCancel(context.Background())

	if !o.noOpen {
		openBrowser(url)
	}

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

	flog.Info("shutting down")
	if !o.syncBack || o.skipSync {
		return nil
	}

	flog.Info("synchronizing VS Code back to local")

	err = syncExtensions(o.sshFlags, host, true)
	if err != nil {
		return xerrors.Errorf("failed to sync extensions back: %w", err)
	}

	err = syncUserSettings(o.sshFlags, host, true)
	if err != nil {
		return xerrors.Errorf("failed to sync user settings back: %w", err)
	}

	return nil
}

// expandPath returns an expanded version of path.
func expandPath(path string) string {
	path = filepath.Clean(os.ExpandEnv(path))

	// Replace tilde notation in path with the home directory. You can't replace the first instance of `~` in the
	// string with the homedir as having a tilde in the middle of a filename is valid.
	homedir := os.Getenv("HOME")
	if homedir != "" {
		if path == "~" {
			path = homedir
		} else if strings.HasPrefix(path, "~/") {
			path = filepath.Join(homedir, path[2:])
		}
	}

	return filepath.Clean(path)
}

func parseBindAddr(bindAddr string) (string, error) {
	if !strings.Contains(bindAddr, ":") {
		bindAddr += ":"
	}

	host, port, err := net.SplitHostPort(bindAddr)
	if err != nil {
		return "", err
	}

	if host == "" {
		host = "127.0.0.1"
	}

	if port == "" {
		port, err = randomPort()
	}
	if err != nil {
		return "", err
	}

	return net.JoinHostPort(host, port), nil
}

func openBrowser(url string) {
	var openCmd *exec.Cmd

	const (
		macPath = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
		wslPath = "/mnt/c/Program Files (x86)/Google/Chrome/Application/chrome.exe"
	)

	switch {
	case commandExists("google-chrome"):
		openCmd = exec.Command("google-chrome", chromeOptions(url)...)
	case commandExists("google-chrome-stable"):
		openCmd = exec.Command("google-chrome-stable", chromeOptions(url)...)
	case commandExists("chromium"):
		openCmd = exec.Command("chromium", chromeOptions(url)...)
	case commandExists("chromium-browser"):
		openCmd = exec.Command("chromium-browser", chromeOptions(url)...)
	case pathExists(macPath):
		openCmd = exec.Command(macPath, chromeOptions(url)...)
	case pathExists(wslPath):
		openCmd = exec.Command(wslPath, chromeOptions(url)...)
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

// checkSSHDirectory performs sanity and safety checks on sshDirectory, and
// returns a new value for o.reuseConnection depending on the checks.
func checkSSHDirectory(sshDirectory string, reuseConnection bool) bool {
	sshDirectoryMode, err := os.Lstat(expandPath(sshDirectory))
	if err != nil {
		if reuseConnection {
			flog.Info("failed to stat %v directory, disabling connection reuse feature: %v", sshDirectory, err)
		}
		reuseConnection = false
	} else {
		if !sshDirectoryMode.IsDir() {
			if reuseConnection {
				flog.Info("%v is not a directory, disabling connection reuse feature", sshDirectory)
			} else {
				flog.Info("warning: %v is not a directory", sshDirectory)
			}
			reuseConnection = false
		}
		if sshDirectoryMode.Mode().Perm()&sshDirectoryUnsafeModeMask != 0 {
			flog.Info("warning: the %v directory has unsafe permissions, they should only be writable by "+
				"the owner (and files inside should be set to 0600)", sshDirectory)
		}
	}
	return reuseConnection
}

// startSSHMaster starts an SSH master connection and waits for it to be ready.
// It returns a new set of SSH flags for child SSH processes to use.
func startSSHMaster(sshFlags string, sshControlPath string, host string) (string, func(), error) {
	ctx, cancel := context.WithCancel(context.Background())

	newSSHFlags := fmt.Sprintf(`%v -o "ControlPath=%v"`, sshFlags, sshControlPath)

	// -MN means "start a master socket and don't open a session, just connect".
	sshCmdStr := fmt.Sprintf(`exec ssh %v -MNq %v`, newSSHFlags, host)
	sshMasterCmd := exec.CommandContext(ctx, "sh", "-c", sshCmdStr)
	sshMasterCmd.Stdin = os.Stdin
	sshMasterCmd.Stderr = os.Stderr

	// Gracefully stop the SSH master.
	stopSSHMaster := func() {
		if sshMasterCmd.Process != nil {
			if sshMasterCmd.ProcessState != nil && sshMasterCmd.ProcessState.Exited() {
				return
			}
			err := sshMasterCmd.Process.Signal(syscall.SIGTERM)
			if err != nil {
				flog.Error("failed to send SIGTERM to SSH master process: %v", err)
			}
		}
		cancel()
	}

	// Start ssh master and wait. Waiting prevents the process from becoming a zombie process if it dies before
	// sshcode does, and allows sshMasterCmd.ProcessState to be populated.
	err := sshMasterCmd.Start()
	go sshMasterCmd.Wait()
	if err != nil {
		return "", stopSSHMaster, err
	}
	err = checkSSHMaster(sshMasterCmd, newSSHFlags, host)
	if err != nil {
		stopSSHMaster()
		return "", stopSSHMaster, xerrors.Errorf("SSH master wasn't ready on time: %w", err)
	}
	return newSSHFlags, stopSSHMaster, nil
}

// checkSSHMaster polls every second for 30 seconds to check if the SSH master
// is ready.
func checkSSHMaster(sshMasterCmd *exec.Cmd, sshFlags string, host string) error {
	var (
		maxTries = 30
		sleepDur = time.Second
		err      error
	)
	for i := 0; i < maxTries; i++ {
		// Check if the master is running.
		if sshMasterCmd.Process == nil || (sshMasterCmd.ProcessState != nil && sshMasterCmd.ProcessState.Exited()) {
			return xerrors.Errorf("SSH master process is not running")
		}

		// Check if it's ready.
		sshCmdStr := fmt.Sprintf(`ssh %v -O check %v`, sshFlags, host)
		sshCmd := exec.Command("sh", "-c", sshCmdStr)
		err = sshCmd.Run()
		if err == nil {
			return nil
		}
		time.Sleep(sleepDur)
	}
	return xerrors.Errorf("max number of tries exceeded: %d", maxTries)
}

// copyCodeServerBinary copies a code-server binary from local to remote.
func copyCodeServerBinary(sshFlags string, host string, localPath string, remotePath string) error {
	if err := validateIsFile(localPath); err != nil {
		return err
	}

	var (
		src  = localPath
		dest = host + ":" + remotePath
	)

	return rsync(src, dest, sshFlags)
}

func syncUserSettings(sshFlags string, host string, back bool) error {
	localConfDir, err := configDir()
	if err != nil {
		return err
	}

	err = ensureDir(localConfDir)
	if err != nil {
		return err
	}

	const remoteSettingsDir = "~/.local/share/code-server/User/"

	var (
		src  = localConfDir + "/"
		dest = host + ":" + remoteSettingsDir
	)

	if back {
		dest, src = src, dest
	}

	// Append "/" to have rsync copy the contents of the dir.
	return rsync(src, dest, sshFlags, "workspaceStorage", "logs", "CachedData")
}

func syncExtensions(sshFlags string, host string, back bool) error {
	localExtensionsDir, err := extensionsDir()
	if err != nil {
		return err
	}

	err = ensureDir(localExtensionsDir)
	if err != nil {
		return err
	}

	const remoteExtensionsDir = "~/.local/share/code-server/extensions/"

	var (
		src  = localExtensionsDir + "/"
		dest = host + ":" + remoteExtensionsDir
	)
	if back {
		dest, src = src, dest
	}

	return rsync(src, dest, sshFlags)
}

func rsync(src string, dest string, sshFlags string, excludePaths ...string) error {
	excludeFlags := make([]string, len(excludePaths))
	for i, path := range excludePaths {
		excludeFlags[i] = "--exclude=" + path
	}

	cmd := exec.Command("rsync", append(excludeFlags, "-azvr",
		"-e", "ssh "+sshFlags,
		// Only update newer directories, and sync times
		// to keep things simple.
		"-u", "--times",
		// This is more unsafe, but it's obnoxious having to enter VS Code
		// locally in order to properly delete an extension.
		"--delete",
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

func downloadScript(codeServerPath string) string {
	return fmt.Sprintf(
		`set -euxo pipefail || exit 1

[ "$(uname -m)" != "x86_64" ] && echo "Unsupported server architecture $(uname -m). code-server only has releases for x86_64 systems." && exit 1
pkill -f %v || true
mkdir -p ~/.local/share/code-server %v
cd %v
curlflags="-o latest-linux"
if [ -f latest-linux ]; then
	curlflags="$curlflags -z latest-linux"
fi
curl $curlflags https://codesrv-ci.cdr.sh/latest-linux
[ -f %v ] && rm %v
ln latest-linux %v
chmod +x %v`,
		codeServerPath,
		filepath.Dir(codeServerPath),
		filepath.Dir(codeServerPath),
		codeServerPath,
		codeServerPath,
		codeServerPath,
		codeServerPath,
	)
}

// ensureDir creates a directory if it does not exist.
func ensureDir(path string) error {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		err = os.MkdirAll(path, 0750)
	}

	if err != nil {
		return err
	}

	return nil
}

// validateIsFile tries to stat the specified path and ensure it's a file.
func validateIsFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return xerrors.New("path is a directory")
	}
	return nil
}

// parseHost parses the host argument. If 'gcp:' is prefixed to the
// host then a lookup is done using gcloud to determine the external IP and any
// additional SSH arguments that should be used for ssh commands. Otherwise, host
// is returned.
func parseHost(host string) (parsedHost string, additionalFlags string, err error) {
	host = strings.TrimSpace(host)
	switch {
	case strings.HasPrefix(host, "gcp:"):
		instance := strings.TrimPrefix(host, "gcp:")
		return parseGCPSSHCmd(instance)
	default:
		return host, "", nil
	}
}

// parseGCPSSHCmd parses the IP address and flags used by 'gcloud' when
// ssh'ing to an instance.
func parseGCPSSHCmd(instance string) (ip, sshFlags string, err error) {
	dryRunCmd := fmt.Sprintf("gcloud compute ssh --dry-run %v", instance)

	out, err := exec.Command("sh", "-l", "-c", dryRunCmd).CombinedOutput()
	if err != nil {
		return "", "", xerrors.Errorf("%s: %w", out, err)
	}

	toks := strings.Split(string(out), " ")
	if len(toks) < 2 {
		return "", "", xerrors.Errorf("unexpected output for '%v' command, %s", dryRunCmd, out)
	}

	// Slice off the '/usr/bin/ssh' prefix and the '<user>@<ip>' suffix.
	sshFlags = strings.Join(toks[1:len(toks)-1], " ")

	// E.g. foo@1.2.3.4.
	userIP := toks[len(toks)-1]

	return strings.TrimSpace(userIP), sshFlags, nil
}
