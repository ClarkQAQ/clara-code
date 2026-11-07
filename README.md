# Clara Code

Clara Code 是基于 Clara Core 的终端编码助手。它使用终端主屏幕，已完成的对话保留在原生滚动历史中。调整窗口大小时按逻辑消息重建当前终端缓冲区并重排会话，当前问题、菜单和输入草稿随窗口重新布局。交互版支持 macOS 和 Linux。

## 启动

需要 Go 1.27 和相邻目录里的 `clara-core` 工作树。

```sh
go run ./cmd/clara-code --demo
go build -o clara-code ./cmd/clara-code
./clara-code --cwd /path/to/project
./clara-code resume --cwd /path/to/project
```

真实模型可用环境变量配置。

```sh
export CLARA_CODE_ENDPOINT='https://gateway.example/v1'
export CLARA_CODE_PROTOCOL='responses' # 或 completions
export CLARA_CODE_MODEL='your-model'
export CLARA_CODE_API_KEY='your-key'
./clara-code
```

## 配置

Linux 数据目录默认为 `~/.local/share/clara-code`，设置 `XDG_DATA_HOME` 时使用其下的 `clara-code`。macOS 使用 `~/Library/Application Support/clara-code`。两个平台都可通过 `CLARA_CODE_HOME` 覆盖。

在数据目录中创建 `config.toml`。模型环境变量优先，未配置的模型连接参数可回退到同名的 `CLARA_CORE_*` 变量。也可用 `api_key_file` 指定仅当前用户可访问的密钥文件（权限 `0600`）。相对路径以数据目录为基准，环境变量中的密钥优先。

```toml
endpoint = "https://gateway.example/v1"
protocol = "responses"
api_key_env = "MY_GATEWAY_API_KEY"
model = "coding"
context_bytes = 524288

[models.coding]
id = "your-model"
effort = "high"
context_window = 128000
effective_context_window_percent = 95

[models.fast]
id = "another-model"

# MCP stdio 示例
[mcp_servers.files]
command = "/path/to/mcp-server"
args = ["--stdio"]
required = false

# MCP Streamable HTTP 示例
[mcp_servers.docs]
url = "http://127.0.0.1:8765/mcp"

# 外部 ACP 子代理示例。模型可用 reviewer-external 作为任务角色。
[acp_agents.reviewer-external]
command = "/path/to/another-agent"
args = ["acp"]
```

`context_bytes` 是自动压缩的序列化字节预算，设为 `0` 关闭自动压缩。`context_window` 是模型原始 token 上限。状态栏默认使用其 95% 作为有效窗口，可通过 `effective_context_window_percent` 调整。当前上下文使用最近一次有效请求的总 token，百分比与参考 Codex 一样扣除 12,000 token 的固定基线，累计用量在 `/status` 单独显示。未取得用量时显示 unknown。全局规则文件放在数据目录的 `AGENTS.md`，全局技能放在 `skills/<name>/SKILL.md`。项目规则从 Git 根目录向下加载，项目技能放在相应目录的 `.agents/skills/`。技能文件用 YAML 或 TOML front matter 声明 `name` 和 `description`。

## 交互

| 操作 | 按键或命令 |
| --- | --- |
| 提交或引导运行中的回合 | Enter |
| 添加换行 | Alt+Enter，支持区分按键的终端也可用 Shift+Enter |
| 切换 Agent 与 Plan | Shift+Tab，或 `/agent`、`/plan` |
| 中断运行 | Esc 或 `/cancel` |
| 查看、关闭后台终端（运行中也可用） | `/ps`、`/stop [ID]`，不带 ID 关闭调用时已有的全部后台终端 |
| 编辑历史消息并创建分支 | 空输入时按两次 Esc，左右键选消息，上下键滚动，Ctrl+T 切换详情，Enter 确认 |
| 新建、恢复、从当前会话分支 | `/new`、`/resume [ID]`、`/fork` |
| 清理终端并新开会话 | `/clear`，旧会话仍可恢复 |
| 查看用量、配置、当前目录 | `/status`、`/pwd` |
| 查看变更 | `/diff`，Tab 切换暂存、未暂存与未跟踪文件，n/p 翻页 |
| 选模型和推理强度 | `/model [名称 [强度]]` |
| 只读审查当前 Git 变更 | `/review` |
| 主动压缩模型上下文 | `/compact` |
| 运行目标或排队后续指令 | `/goal`、`/pause`、`/continue`、`/follow` |
| 查看连接状态、选择技能 | `/mcp`、`/skills` |
| 查看帮助或退出 | `/help`、`/exit` 或 `/quit` |


输入 `/` 显示双列命令菜单，上下键选择，Tab 补全，Enter 确认，Esc 关闭并保留草稿。Plan 模式显示在状态栏右侧。

主输入框、问题备注和自由回答使用真实终端光标，中文输入法候选框及中英文切换提示可按插入位置定位。光标沿用终端默认形状；只读选择器隐藏光标，退出时恢复可见。中文、组合 emoji、多行和窄窗口均按终端显示列计算位置。候选框外观与选词操作仍由终端和系统输入法负责。

模型的进度说明与最终回答分别保留。工具执行显示为持续更新的记录，例如 `● Bash(command)`，下方用 `└` 展示输出。绿色代表成功，红色代表失败，取消和后台运行有独立状态。Shell 结果依据实际退出码；后台命令的后续输出更新原记录。长输出预览保留首尾，结构化记录沿用会话的存储上限，可在恢复和窗口重排时回放。

后台终端由所属 Clara 会话统一管理，包含子代理启动的命令。`yield_seconds` 决定首次执行或后续等待多久后返回；省略 `timeout_seconds` 时没有总运行时限，显式超时仍生效（最多 600 秒）。Esc 只中断当前 Agent 回合和等待，已经进入后台的终端继续运行；`/stop` 才会显式停止它们。关闭或切换会话、退出进程时清理其终端。

后台有命令运行时，输入区上方显示 `1 background terminal running · /ps to view · /stop to close`；活动提示可见时合并显示。每 500ms 非消费检查状态，仅在输出或状态变化时刷新，`/ps` 不取走模型尚未读取的输出。模型空输入轮询显示独立的 `Waiting for background terminal`，结束后保留 `Waited for background terminal`；发送输入显示 `Interacted with background terminal`，原 Bash 记录继续显示命令输出及最终状态。取消发送输入只能停止等待，已交给终端的字符无法撤回。

异步命令状态单独原子保存，已完成回合文件不改写。自然退出、失败、停止及超时都会更新原命令记录；未读输出仍可通过 `write_stdin` 领取。完成缓存最多保留最近 32 个终端，单终端输出沿用最多 1 MiB 的保留限制，终端预览保留首尾。恢复和分支只回放记录，不重新启动后台进程；尚无最终状态的旧记录显示为已中断。自定义执行后端若未实现 Core 的 `SnapshotProcess`，只能在模型主动轮询时更新状态，`/ps` 会提示这一限制。

输入区上方的活动行独立显示等待、执行、压缩及重连状态，包含耗时和 Esc 中断提示；交互问题出现时暂时隐藏。公开的 reasoning summary 可提供活动标题，原始推理内容不会作为进度说明显示。连接建立阶段的可重试错误由 Core 最多重试三次，显示次数、等待时间和脱敏原因。流已开始后发生中断会保留收到的文字并报告错误，不会自动重放已执行的工具。最终失败会保留正在编辑的草稿；输入框为空时会填回该轮提示，方便修改后重试。

交互问题包含编号选项和说明。上下键或数字选择，Tab 编辑备注，左右键切换问题，Enter 提交，Esc 中断。技能选择会把 `$技能名` 插入输入框，模型可通过 `read_skill` 和 `read_skill_resource` 读取技能及其附属文本文件。

文件工具与 Shell 使用真实主机路径：相对路径以当前工作目录为基准，`..` 可访问相邻目录，绝对路径按系统原义解析，`/` 是系统根目录，`~/` 展开为当前用户主目录。文件工具的返回路径和 Hashline 头为真实绝对路径。Shell 的 `workdir` 使用相同语义，支持项目外目录。会话列表的时间统一显示为本地时间 `YYYY-MM-DD HH:mm`。

`glob` 与 `grep` 默认通过 Git 搜索跟踪文件及未被忽略的未跟踪文件，包含被忽略但已跟踪的文件。设置 `include_ignored=true` 可包含忽略文件；默认递归遍历跳过 Git 元数据和子模块，明确指定其中的目录为搜索根时允许进入。跨项目搜索使用目标仓库的 Git 规则，直接 `read` 不受过滤影响。非 Git 项目搜索完整目录树。每次搜索刷新 Git 规则；Git 枚举失败会明确报错，超过 20,000 个候选文件或 8 MiB 枚举输出时需缩小搜索路径。

搜索通配支持 `*.go`、`**/*.go`、`?` 和字符集合，`**` 可匹配零层目录。`grep` 使用 `query` 搜索字面量，`include` 过滤文件；例如 `{"path":"internal","query":"cursorPosition","include":["*.go"]}`。结果带有搜索不完整提示时，应查看诊断并缩小范围，不能据此认定项目中没有相关代码。`fs_instructions` 查询项目规则，全局规则由宿主单独注入。

Agent 模式使用 YOLO 权限，文件工具和 Shell 均按当前用户的系统权限访问文件，不以项目目录为边界。读取符号链接会跟随目标，覆盖写入与编辑更新目标；移动或删除链接操作链接本身。新建文件不会覆盖已有链接。旧会话仍可恢复，其原记录中的 `/foo` 曾代表项目内路径，恢复时会显示迁移说明；编辑前需按新语义重新读取目标，旧快照不复用。Plan 模式只提供只读和交互工具。分支恢复选中提示之前的模型上下文。此前的文件修改和命令效果仍留在工作区。每个会话有独立锁，其他进程仍可打开同一项目的不同会话。运行中的目标、子代理和 Shell 进程随当前会话关闭而结束。

在支持 ACP 的编辑器中，将 `clara-code acp` 作为 stdio agent 启动命令。它支持创建和加载会话、发送提示、流式更新和取消。协议会话使用相同的项目识别与持久化目录。
