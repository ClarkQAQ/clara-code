# Forgejo Automated Publishing

Pushing to `master` or manually running the [release workflow](workflows/release.yml) in Forgejo Actions builds and publishes pacman (Linux x86_64 and aarch64) and Homebrew (macOS arm64) packages, plus Windows 11 x64 and ARM64 ZIP archives. Versions use the format `r<commit count>.g<12-character commit hash>`, matching `clara-code --version`.

Publishing uses the `default` runner. The runner needs Go module access, Docker, GNU tar, gzip, zip, unzip, curl (with `--retry-all-errors` support), Git, jq, sha256sum, and base64. Arch packages are assembled in an `archlinux:base-devel` container, and the Homebrew archive is uploaded to the Forgejo Generic Registry; the formula uses the SHA-256 of the archive downloaded from the registry. Rerunning the same version keeps existing artifacts and updates the formula only when it changes.

Configure the following in the Actions settings for `gulugulu/clara-code`:

| Type | Name | Value |
| --- | --- | --- |
| Variable | `PUBLISH_URL` | `https://forgejo.gulugulu.moe` |
| Variable | `PUBLISH_OWNER` | `gulugulu` |
| Variable | `PUBLISH_USER` | `clark` |
| Variable | `HOMEBREW_TAP` | `homebrew-clara-code` |
| Secret | `PUBLISH_TOKEN` | A token owned by the publishing account with `read:user`, `write:package`, and `write:repository` scopes |

The release workflow uses the service URL and owner above and creates or updates `Formula/clara-code.rb` on the default branch of `gulugulu/homebrew-clara-code`.

## Installation

**Linux:** Open the [Arch Registry page](https://forgejo.gulugulu.moe/gulugulu/-/packages/arch/clara-code), follow the instructions there to configure the repository, read access, and signing key, then run:

```sh
sudo pacman -Syu clara-code
```

**Windows 11:** Download `clara-code-windows-amd64.zip` for x64 or `clara-code-windows-arm64.zip` for ARM64 from the [Forgejo package registry](https://forgejo.gulugulu.moe/gulugulu/-/packages). Each archive has a companion `.sha256` file. Compare the hash with `Get-FileHash .\clara-code-windows-amd64.zip -Algorithm SHA256`, extract the archive, and run `.\clara-code.exe --version` in Windows Terminal. Keep the extracted executable in a directory on your user PATH to run `clara-code` from other directories.

The release runner cross-builds both binaries and compiles the Windows test suites. Native runtime checks require a Windows host.

**macOS (Apple Silicon):** Authenticate the private tap with a Forgejo token that can read the repository and packages. The formula first reads `~/.git-credentials`, then falls back to `~/.netrc`; `HOMEBREW_CLARA_CODE_TOKEN` is no longer used.

The preferred setup stores the token with Git's credential helper:

```sh
git config --global credential.helper store
git credential approve <<'EOF'
protocol=https
host=forgejo.gulugulu.moe
username=YOUR_FORGEJO_USER
password=YOUR_FORGEJO_TOKEN
EOF
```

Alternatively, create `~/.netrc` when no valid `~/.git-credentials` entry exists:

```text
machine forgejo.gulugulu.moe
  login YOUR_FORGEJO_USER
  password YOUR_FORGEJO_TOKEN
```

Then restrict the file to the current user:

```sh
chmod 600 ~/.netrc
```

Add the tap, then install:

```sh
brew tap gulugulu/clara-code https://forgejo.gulugulu.moe/gulugulu/homebrew-clara-code.git
brew install gulugulu/clara-code/clara-code
```

If authentication fails, update the first matching `~/.git-credentials` entry before retrying. The formula does not retry package downloads with the `.netrc` credential after a rejected Git credential.
