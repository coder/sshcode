# sshcode

[!["Open Issues"](https://img.shields.io/github/issues-raw/codercom/sshcode.svg)](https://github.com/codercom/sshcode/issues)
[!["Latest Release"](https://img.shields.io/github/release/codercom/sshcode.svg)](https://github.com/codercom/sshcode/releases/latest)
[![MIT license](https://img.shields.io/badge/license-MIT-green.svg)](https://github.com/codercom/sshcode/blob/master/LICENSE)
[![Discord](https://img.shields.io/discord/463752820026376202.svg?label=&logo=discord&logoColor=ffffff&color=7389D8&labelColor=6A7EC2)](https://discord.gg/zxSwN8Z)

`sshcode` is a CLI to automatically install and run [code-server](https://github.com/codercom/code-server) over SSH.

It uploads your extensions and settings automatically, so you can seamlessly use
remote servers as VS Code hosts.

If you have Chrome installed, it opens the browser in app mode. That means
there's no keybind conflicts, address bar, or indication that you're coding within a browser.
**It feels just like native VS Code.**

![Demo](/demo.gif)

## Install

Chrome is recommended.

Install with `go`:

```bash
go get go.coder.com/sshcode
```

Or, grab a [pre-built binary](https://github.com/codercom/sshcode/releases).

## Usage

```bash
sshcode kyle@dev.kwc.io
# Starts code-server on dev.kwc.io and opens in a new browser window.
```

You can specify a remote directory as the second argument:

```bash
sshcode kyle@dev.kwc.io ~/projects/sourcegraph
```

## Extensions & Settings Sync

By default, `sshcode` will `rsync` your local VS Code settings and extensions
to the remote server every time you connect.

This operation may take a while on a slow connections, but will be fast
on follow-up connections to the same server.

To disable this feature entirely, pass the `--skipsync` flag.

You can specify `--insiders` to sync settings and extensions from VS Code Insiders.

### Sync-back

By default, VS Code changes on the remote server won't be synced back
when the connection closes. To synchronize back to local when the connection ends,
pass the `-b` flag.
