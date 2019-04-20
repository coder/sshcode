# sshcode

`sshcode` is a CLI to automatically install and run [code-server](https://github.com/codercom/code-server) over SSH.

![Demo](/demo.gif)

## Install

Chrome is recommended.

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
on follow-on connections to the same server.

To disable this feature entirely, pass the `--skipsync` flag.
