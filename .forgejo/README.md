# Forgejo Automated Publishing

Pushing to `master` or manually running the [release workflow](workflows/release.yml) in Forgejo Actions builds and publishes pacman (Linux x86_64 and aarch64) and Homebrew (macOS arm64) packages sequentially. Versions use the format `r<commit count>.g<12-character commit hash>`, matching `clara-code --version`.

Publishing uses the `default` runner. The runner needs Go module access, Docker, GNU tar, gzip, curl (with `--retry-all-errors` support), Git, jq, sha256sum, and base64. Arch packages are assembled in an `archlinux:base-devel` container, and the Homebrew archive is uploaded to the Forgejo Generic Registry; the formula uses the SHA-256 of the archive downloaded from the registry. Rerunning the same version keeps existing artifacts and updates the formula only when it changes.

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

**macOS (Apple Silicon):** Export `HOMEBREW_CLARA_CODE_TOKEN`, set to a dedicated Forgejo package read token. Add the tap with a Git account that has access, then install:

```sh
brew tap gulugulu/clara-code https://forgejo.gulugulu.moe/gulugulu/homebrew-clara-code.git
brew install gulugulu/clara-code/clara-code
```
