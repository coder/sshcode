package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"go.coder.com/flog"
)

func init() {
	rand.Seed(time.Now().Unix())
}

const helpTabWidth = 5

var helpTab = strings.Repeat(" ", helpTabWidth)

// version is overwritten by ci/build.sh.
var version string

func main() {
	var (
		skipSyncFlag = flag.Bool("skipsync", false, "skip syncing local settings and extensions to remote host")
		sshFlags     = flag.String("ssh-flags", "", "custom SSH flags")
		syncBack     = flag.Bool("b", false, "sync extensions back on termination")
		printVersion = flag.Bool("version", false, "print version information and exit")
	)

	flag.Usage = usage

	flag.Parse()
	if *printVersion {
		fmt.Printf("%v\n", version)
		os.Exit(0)
	}

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

	err := sshCode(host, dir, options{
		skipSync: *skipSyncFlag,
		sshFlags: *sshFlags,
		syncBack: *syncBack,
	})

	if err != nil {
		flog.Fatal("error: %v", err)
	}
}

func usage() {
	fmt.Printf(`Usage: %v [FLAGS] HOST [DIR]
Start VS Code via code-server over SSH.

Environment variables:
		%v use special VS Code settings dir.
		%v use special VS Code extensions dir.

More info: https://github.com/cdr/sshcode

Arguments:
%vHOST is passed into the ssh command.
%vDIR is optional.

%v`,
		os.Args[0],
		vsCodeConfigDirEnv,
		vsCodeExtensionsDirEnv,
		helpTab,
		helpTab,
		flagHelp(),
	)

}

// flagHelp generates a friendly help string for all globally registered command
// line flags.
func flagHelp() string {
	var bd strings.Builder

	w := tabwriter.NewWriter(&bd, 3, 10, helpTabWidth, ' ', 0)

	fmt.Fprintf(w, "Flags:\n")
	var count int
	flag.VisitAll(func(f *flag.Flag) {
		count++
		if f.DefValue == "" {
			fmt.Fprintf(w, "\t-%v\t%v\n", f.Name, f.Usage)
		} else {
			fmt.Fprintf(w, "\t-%v\t%v\t(%v)\n", f.Name, f.Usage, f.DefValue)
		}
	})
	if count == 0 {
		return "\n"
	}

	w.Flush()

	return bd.String()
}
