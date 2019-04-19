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
	"strconv"
	"time"

	"go.coder.com/flog"
)

func main() {
	flag.Usage = func() {
		fmt.Printf(`Usage: %v HOST [SSH ARGS...]

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
'`,
	)
	output, err := sshCmd.CombinedOutput()
	if err != nil {
		flog.Fatal("failed to update code-server: %v: %s", err, string(output))
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

	var openCmd *exec.Cmd
	url := "http://127.0.0.1:" + localPort
	if commandExists("google-chrome") {
		openCmd = exec.Command("google-chrome", "--app="+url, "--disable-extensions", "--disable-plugins")
	} else if commandExists("firefox") {
		openCmd = exec.Command("firefox", "--url="+url, "-safe-mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for {
		err := ctx.Err()
		if err != nil {
			flog.Fatal("code-server didn't start in time %v", err)
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

	err = openCmd.Start()
	if err != nil {
		flog.Fatal("failed to open browser: %v", err)
	}
	sshCmd.Wait()
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
			port++
			continue
		}
		_ = l.Close()

		return strconv.Itoa(port), nil
	}

	return "", errors.New("no ports available")
}
