# Installation

Install the Uncloud command-line utility to manage your machines and deploy apps using `uc` commands. You will run `uc`
locally so choose the appropriate installation method for your operating system.

## Homebrew (macOS, Linux)

If you have [Homebrew](https://brew.sh/) package manager installed, this is the recommended installation method on macOS
and Linux:

```bash
brew install psviderski/tap/uncloud
```

To upgrade to the latest version:

```bash
brew upgrade uncloud
```

## Install script (macOS, Linux)

For a quick automated installation, use the install script:

```bash
curl -fsS https://get.uncloud.run/install.sh | sh
```

The script will:

- Detect your operating system and architecture
- Download the appropriate latest binary from [GitHub releases](https://github.com/psviderski/uncloud/releases)
- Install it to `/usr/local/bin/uncloud` using `sudo` (you may need to enter your user password)
- Create a shortcut `uc` in `/usr/local/bin` for convenience

Don't like `curl | sh`? You can download and review the [install script](https://get.uncloud.run/install.sh) first and
then run it:

```bash
curl -fsSO https://get.uncloud.run/install.sh
cat install.sh
sh install.sh
```

## GitHub download (macOS, Linux)

You can manually download and use a pre-built binary from the
[latest release](https://github.com/psviderski/uncloud/releases/latest) on GitHub.

Make sure to replace `(macos|linux)` and `(amd64|arm64)` with your OS and architecture.

```bash
curl -L https://github.com/psviderski/uncloud/releases/latest/download/uncloud_(macos|linux)_(amd64|arm64).tar.gz | tar xz
mv uncloud uc
```

You can use the `./uc` binary directly from the current directory, or move it to a directory in your system's `PATH`
to run it as `uc` from any location.

For example, move it to `/usr/local/bin` which is a common location for user-installed binaries:

```bash
sudo mv ./uc /usr/local/bin
```

## Verify installation

After installation, verify that `uc` command is working:

```bash
uc --version
```

## Next steps

Now that you have `uc` installed, you're ready to:

- [Quick start](./quick-start) — Deploy your first application
