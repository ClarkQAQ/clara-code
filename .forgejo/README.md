# Forgejo 自动发布

向 `master` 推送或在 Forgejo Actions 手动运行 [发布流程](workflows/release.yml)，会串行构建
并发布 pacman（Linux x86_64 和 aarch64）与 Homebrew（macOS arm64）。版本格式为
`r<提交数>.g<12 位提交哈希>`，与 `clara-code --version` 一致。

发布使用 `default` Runner。Runner 需要 Go 模块访问权限、Docker、GNU tar、gzip、curl
（支持 `--retry-all-errors`）、Git、jq、sha256sum 和 base64。Arch 包由
`archlinux:base-devel` 容器组装，Homebrew 的归档上传到 Forgejo Generic Registry，公式
使用从 Registry 下载的归档的 SHA-256。重跑同一版本时，已有制品会保留，公式有变化才更新。

在 `gulugulu/clara-code` 的 Actions 设置中配置：

| 类型 | 名称 | 值 |
| --- | --- | --- |
| Variable | `PUBLISH_URL` | `https://forgejo.gulugulu.moe` |
| Variable | `PUBLISH_OWNER` | `gulugulu` |
| Variable | `PUBLISH_USER` | `clark` |
| Variable | `HOMEBREW_TAP` | `homebrew-clara-code` |
| Secret | `PUBLISH_TOKEN` | 属于发布账号，具有 `read:user`、`write:package` 和 `write:repository` 权限的 Token |

发布流程使用上述服务地址和所有者，并在 `gulugulu/homebrew-clara-code` 的默认分支
创建或更新 `Formula/clara-code.rb`。

## 安装

**Linux：**打开 [Arch Registry 页面](https://forgejo.gulugulu.moe/gulugulu/-/packages/arch/clara-code)，
按页面指示配置仓库、读取权限和签名密钥，然后运行：

```sh
sudo pacman -Syu clara-code
```

**macOS（Apple Silicon）：**导出 `HOMEBREW_CLARA_CODE_TOKEN`，值为独立的 Forgejo 包读取
Token。使用有权限的 Git 账号添加 Tap，再安装：

```sh
brew tap gulugulu/clara-code https://forgejo.gulugulu.moe/gulugulu/homebrew-clara-code.git
brew install gulugulu/clara-code/clara-code
```
