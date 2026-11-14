# Install Clara Code

## GitHub releases

Download `SHA256SUMS` and the archive for your platform from the same [GitHub release](https://github.com/ClarkQAQ/clara-code/releases).

| Platform | Archive |
| --- | --- |
| Linux x64 | `clara-code-linux-amd64.tar.gz` |
| Linux ARM64 | `clara-code-linux-arm64.tar.gz` |
| macOS (Apple Silicon) | `clara-code-darwin-arm64.tar.gz` |
| Windows 11 x64 | `clara-code-windows-amd64.zip` |

From the download directory, set `archive` to your downloaded filename. This example uses Linux x64.

```sh
archive=clara-code-linux-amd64.tar.gz
sha256sum "$archive"
```

On macOS, compute the hash with `shasum -a 256 "$archive"`. Compare the result with the matching entry in `SHA256SUMS` before extracting.

```sh
mkdir clara-code-bin
tar -xzf "$archive" -C clara-code-bin
./clara-code-bin/clara-code --version
```

On Windows, open PowerShell in Windows Terminal and check the hash against `SHA256SUMS`.

```powershell
Get-FileHash .\clara-code-windows-amd64.zip -Algorithm SHA256
```

Once the hash matches, extract the archive and check the version.

```powershell
Expand-Archive .\clara-code-windows-amd64.zip -DestinationPath .\clara-code-bin
.\clara-code-bin\clara-code.exe --version
```

To run Clara Code from other directories, put the executable in a directory on your `PATH`.

## Forgejo packages

Forgejo publishes Arch Linux packages for x64 and ARM64, a Homebrew package for Apple Silicon, and Windows 11 ZIP archives for x64 and ARM64. See the [Forgejo installation instructions](../.forgejo/README.md#installation).

## Build from source

Install Go 1.27 and open a terminal at the repository root. On Linux and macOS, run the following.

```sh
go build -o clara-code .
./clara-code --version
```

On Windows, run the following in PowerShell.

```powershell
go build -o clara-code.exe .
.\clara-code.exe --version
```

Continue with the [quick start](../README.md#quick-start) from the directory containing the executable.
