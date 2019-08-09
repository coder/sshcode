package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"time"

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
}

func (c *rootCmd) Spec() cli.CommandSpec {
	return cli.CommandSpec{
		Name:  "sshcode",
		Usage: c.usage(),
		Desc:  c.description(),
	}
}

func (c *rootCmd) RegisterFlags(fl *flag.FlagSet) {
	fl.BoolVar(&c.skipSync, "skipsync", false, "skip syncing local settings and extensions to remote host")
	fl.BoolVar(&c.syncBack, "b", false, "sync extensions back on termination")
	fl.BoolVar(&c.printVersion, "version", false, "print version information and exit")
	fl.BoolVar(&c.noReuseConnection, "no-reuse-connection", false, "do not reuse SSH connection via control socket")
	fl.StringVar(&c.bindAddr, "bind", "", "local bind address for SSH tunnel, in [HOST][:PORT] syntax (default: 127.0.0.1)")
	fl.StringVar(&c.sshFlags, "ssh-flags", "", "custom SSH flags")
}

func (c *rootCmd) Run(fl *flag.FlagSet) {
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

	// Get linux relative path if on windows
	if runtime.GOOS == "windows" {
		dir = relativeWindowsPath(dir)
	}

	err := sshCode(host, dir, options{
		skipSync:        c.skipSync,
		sshFlags:        c.sshFlags,
		bindAddr:        c.bindAddr,
		syncBack:        c.syncBack,
		reuseConnection: !c.noReuseConnection,
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

func relativeWindowsPath(dir string) string {
	usr, err := user.Current()
	if err != nil {
		flog.Error("Could not get user: %v", err)
		return dir
	}
	rel, err := filepath.Rel(usr.HomeDir, dir)
	if err != nil {
		return dir
	}
	rel = "~/" + filepath.ToSlash(rel)
	return rel
}

// gitbashWindowsDir translates a directory similar to `C:\Users\username\path` to `~/path` for compatibility with Git bash.
func gitbashWindowsDir(dir string) (res string) {
	res = filepath.ToSlash(dir)
	res = strings.Replace(res, "C:", "", -1)
	//res2 =
	return res
}
