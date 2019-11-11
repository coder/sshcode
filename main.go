package main

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/spf13/pflag"

	"go.coder.com/cli"
	"go.coder.com/flog"
)

func init() {
	rand.Seed(time.Now().Unix())
}

const helpTabWidth = 5

var (
	helpTab = strings.Repeat(" ", helpTabWidth)
	// version is overwritten by ci/build.sh.
	version string
	res 	string
	dir		string
)

func main() {
	cli.RunRoot(&rootCmd{})
}

var _ interface {
	cli.Command
	cli.FlaggedCommand
} = new(rootCmd)

type rootCmd struct {
	skipSync          bool
	syncBack          bool
	printVersion      bool
	noReuseConnection bool
	bindAddr          string
	sshFlags          string
	uploadCodeServer  string
}

func (c *rootCmd) Spec() cli.CommandSpec {
	return cli.CommandSpec{
		Name:  "sshcode",
		Usage: c.usage(),
		Desc:  c.description(),
	}
}

func (c *rootCmd) RegisterFlags(fl *pflag.FlagSet) {
	fl.BoolVar(&c.skipSync, "skipsync", false, "skip syncing local settings and extensions to remote host")
	fl.BoolVar(&c.syncBack, "b", false, "sync extensions back on termination")
	fl.BoolVar(&c.printVersion, "version", false, "print version information and exit")
	fl.BoolVar(&c.noReuseConnection, "no-reuse-connection", false, "do not reuse SSH connection via control socket")
	fl.StringVar(&c.bindAddr, "bind", "", "local bind address for SSH tunnel, in [HOST][:PORT] syntax (default: 127.0.0.1)")
	fl.StringVar(&c.sshFlags, "ssh-flags", "", "custom SSH flags")
	fl.StringVar(&c.uploadCodeServer, "upload-code-server", "", "custom code-server binary to upload to the remote host")
}

func (c *rootCmd) Run(fl *pflag.FlagSet) {
	if c.printVersion {
		fmt.Printf("%v\n", version)
		os.Exit(0)
	}

	host := fl.Arg(0)
	if host == "" {
		// If no host is specified output the usage.
		fl.Usage()
		os.Exit(1)
	}

	dir := fl.Arg(1)
	if dir == "" {
		dir = "~"
	}

	// Get linux relative path if on windows.
	if runtime.GOOS == "windows" {
		dir = gitbashWindowsDir(dir)
	}

	err := sshCode(host, dir, options{
		skipSync:         c.skipSync,
		sshFlags:         c.sshFlags,
		bindAddr:         c.bindAddr,
		syncBack:         c.syncBack,
		reuseConnection:  !c.noReuseConnection,
		uploadCodeServer: c.uploadCodeServer,
	})

	if err != nil {
		flog.Fatal("error: %v", err)
	}
}

func (c *rootCmd) usage() string {
	return "[FLAGS] HOST [DIR]"
}

func (c *rootCmd) description() string {
	return fmt.Sprintf(`Start VS Code via code-server over SSH.

Environment variables:
%v%v use special VS Code settings dir.
%v%v use special VS Code extensions dir.

More info: https://github.com/cdr/sshcode

Arguments:
%vHOST is passed into the ssh command. Valid formats are '<ip-address>' or 'gcp:<instance-name>'.
%vDIR is optional.`,
		helpTab, vsCodeConfigDirEnv,
		helpTab, vsCodeExtensionsDirEnv,
		helpTab,
		helpTab,
	)
}

// This section translates a windows path such as "C:\Users\user" to "/Users/user"
// and removes the default paths for mingw and git4windows to issues when you
// specify a file path to start code-server in.
func gitbashWindowsDir(dir string) string {
	if dir == "~" { //Special case
		return "~/"
	}
	mountPoints := gitbashMountPointsAndHome()

	// Apply mount points
	absDir, _ := filepath.Abs(dir)
	absDir = filepath.ToSlash(absDir)
	for _, mp := range mountPoints {
		if strings.HasPrefix(absDir, mp[0]) {
			resolved := strings.Replace(absDir, mp[0], mp[1], 1)
			flog.Info("Resolved windows path '%s' to '%s", dir, resolved)
			return resolved
		}
	}
	return dir
}

// This function returns an array with MINGW64 mount points including relative home dir
func gitbashMountPointsAndHome() [][]string {
	// Initialize mount points with home dir
	mountPoints := [][]string{{filepath.ToSlash(os.Getenv("HOME")), "~"}}
	// Load mount points
	out, err := exec.Command("mount").Output()
	if err != nil {
		log.Fatal(err)
	}
	lines := strings.Split(string(out), "\n")
	var mountRx = regexp.MustCompile(`^(.*) on (.*) type`)
	for _, line := range lines {
		extract := mountRx.FindStringSubmatch(line)
		if len(extract) > 0 {
			mountPoints = append(mountPoints, []string{extract[1], extract[2]})
		}
		res = strings.TrimPrefix(dir, line)
	}
	// Sort by size to get more restrictive mount points first
	sort.Slice(mountPoints, func(i, j int) bool {
		return len(mountPoints[i][0]) > len(mountPoints[j][0])
	})
	return mountPoints
}
