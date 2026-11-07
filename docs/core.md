# Clara Code 公共 Go 包

原 Clara Core 的可嵌入能力现位于当前仓库的 `pkg`，与 Clara Code 共用一个 Go module。运行时移植自 Yuzu `f4855d7`，保留 `NewAgent`、`Generate`、`Stream`、`WithTools` 和 `PrepareStep`。终端、Web 和 ACP 示例共用一套 Coding Agent 装配。宿主选择模型、工具、文件系统、执行器和会话存储。

## 运行 Coding Agent

```sh
export CLARA_CORE_ENDPOINT='https://your-model-gateway.example/v1'
export CLARA_CORE_PROTOCOL='responses'
export CLARA_CORE_MODEL='your-model'
export CLARA_CORE_API_KEY='your-key'

go run ./examples/coding-agent -root /path/to/workspace -store .clara-core/sessions
go run ./examples/web -root /path/to/workspace -store .clara-core/sessions
```

`CLARA_CORE_PROTOCOL` 可选 `responses` 或 `completions`。示例优先读取 `CLARA_CORE_*`，新变量未设置时回退到对应的 `CLARY_*`。

终端用 `-resume SESSION_ID` 恢复会话，Web 可粘贴会话 ID。首次创建 `-store` 指向的目录时，宿主会建立缺少的父目录。已有会话可通过 `-store` 指向原目录继续使用。

示例的文件工具和 `exec_command.workdir` 使用工作区路径。若以 `-root examples/web` 启动，文件工具中的 `/` 对应 `examples/web`，`/main.go` 对应其中的 `main.go`。Shell 输出仍可显示宿主机路径。

终端支持 `/plan`、`/agent`、`/cancel`、`/follow TEXT`、`/answer JSON`、`/goal TEXT`、`/pause`、`/resume` 和 `/wait`。运行中的普通消息作为 steering。Web 用同一会话接口，通过 SSE 显示模型和工具事件。示例 Web 面向单人本地使用，默认监听 loopback。Shell 命令按宿主用户权限运行。需要隔离时由宿主注入受限 Runner 或权限回调。

`-demo` 用确定性模型测试交互。运行 `go run ./examples/coding` 可以在临时 Go 项目中验证实际的失败复现、搜索、Hashline 修改和测试重跑。

## 在 Go 宿主中装配

```go
import (
    "clara-code/pkg/core"
    "clara-code/pkg/models/provider"
    "clara-code/pkg/tools"
    "clara-code/pkg/tools/fs/local"
)

client, err := provider.New(provider.WithBaseURL(endpoint),
    provider.WithAPI(provider.APIResponses), provider.WithAPIKey(apiKey))
if err != nil { return err }
model, err := client.LanguageModel(ctx, modelName)
if err != nil { return err }
workspace, err := local.New(projectDir)
if err != nil { return err }

files := tools.NewFiles(workspace, nil)
engine := core.NewAgent(model, core.WithTools(files.Read(), files.Grep(), files.Edit()))
result, err := engine.Generate(ctx, core.AgentCall{Prompt: "Find and fix the failing test."})
```

Go 基线为 1.27。其他 Go 模块可以通过 `replace` 指向本地工作树。迁移旧项目时，将原导入路径改为新的模块路径。

```sh
go mod edit -require=clara-code@v0.0.0
go mod edit -replace=clara-code=/absolute/path/to/clara-code
go mod tidy
```

当前使用短模块路径 `clara-code`。独立项目通过本地 `replace` 或 Go workspace 接入。

- 无会话调用使用 `Generate` 或 `Stream`。`PrepareStep` 可以逐步选择模型、上下文和工具。工具可以实现 `models.AgentTool`，也可以使用 `models.NewAgentTool` 或 `tools.Func`。
- 有会话调用使用 `core.NewSession`、`Session.Start`、`Run.Wait`、`Run.Cancel`、`Run.Steer` 和 `Run.FollowUp`。`core.OpenSession` 从 Store 恢复。订阅用 `Session.NextEvent` 从序号继续读取，超过保留窗口会得到带最早序号的 `GapError`。
- 默认会话用内存。文件 Store 由宿主提供 `core.Blobs`。示例通过根目录限制的本地 FS 写入 checkpoint。版本冲突检测覆盖共享同一个 Store 的会话，跨进程共享目录需宿主提供带互斥的 Store。
- `tools.NewFiles` 接受可读后端。存储可以换成 memory、virtual、mount、restricted 或 SFTP。Shell 由宿主另行注册。SSH 使用 `tools.NewSSH` 与明确的 HostKeyCallback，SFTP 使用 `tools/fs/sftp`。
- `tools.PlanOnly` 是执行入口的权限回调，`tools.PlanTools` 负责筛选定义。执行清单使用独立的 `PlanStore`。`Goal` 通过宿主调用 `Start`、`NextTurn` 和 `Charge` 接入。`TaskManager` 通过工厂创建子会话，每个 scope 按父级工具名单限制能力。
- `pkg/core/prompt` 提供嵌入式片段、动态上下文和可选压缩。终端和 Web 可用 `-context-bytes` 启用压缩。自定义宿主可以在 `PrepareStep` 中接入 `Compactor`。终端和 Web 的 Plan、Goal 与子任务状态属于当前宿主进程。持久化会话保存通用 transcript 和事件。

模型工具参数使用严格 JSON 对象。结构化流只在累积内容通过 JSON 和 schema 校验时发出预览，收到正常终止事件后再校验最终结果。宿主可以显式配置 `RepairToolCall` 或 `RepairText`。会话文件和事件 JSON 格式保持兼容。

ACP 入口在 `clara-code/pkg/core`，任务适配器是 `tools.NewACPTaskBackend`。MCP 连接管理从 `clara-code/pkg/tools/mcp` 导入，Relay 从 `clara-code/pkg/models/relay` 导入。

## 后台命令

`Commands.Terminals` 查询后台快照，`Refresh(ctx)` 更新状态，`Stop(ctx, id)` 停止指定终端，空 ID 停止调用时已有的全部终端。`OnChange` 在状态或保留输出变化后通知宿主，耗时增长本身不触发通知。原 `Runner`、`Process` 接口保持兼容；本地与 SSH 进程实现可选的 `SnapshotProcess`，查询不推进 `write_stdin` 的输出游标。自定义后端未实现该接口时，自动监视不消费输出，状态由主动轮询更新。

后台终端自然退出、非零退出、超时及显式停止分别保留最终状态，结束后释放进程资源，并缓存未读输出供后续 `write_stdin` 领取，最多保留最近 32 个完成终端；单终端输出上限仍为 1 MiB。取消 `write_stdin` 仅取消等待，保留进程；显式停止与并发轮询返回停止状态。发送输入的等待可取消，但已交给 stdin 的字节无法撤回。`Commands.Close` 清理全部存活终端，进程不随持久化会话恢复。

Core 默认仍设置 600 秒总时限。宿主可显式设置 `Commands.NoDefaultTimeout=true`，使未传 `timeout_seconds` 的命令没有总时限；`Spec.NoTimeout` 对自定义 Runner 表达相同意图。显式超时继续生效，`yield_seconds` 只控制单次等待时间。宿主应把 `Commands.Context` 设为所属会话的生命周期，并自行安排刷新与通知展示。

## 原生主机路径

`tools/fs/host.New(cwd)` 提供显式选择的开放文件系统后端，按当前用户的操作系统权限访问真实路径。相对路径从 cwd 解析，绝对路径和 `..` 可越出项目，`~/` 展开为主目录。读取跟随符号链接，覆盖写入保留链接并更新其目标；删除和重命名操作链接本身。新建不覆盖已有文件或链接。原 `tools/fs/local.New(root)` 的根目录限制保持不变。

`Files.InstructionsSource` 可定制目标文件的规则来源，`fs.InstructionsFrom` 可限定规则加载的祖先起点；默认宿主行为保持兼容。Hashline 使用文件系统的路径解析器，因此原生路径和受限逻辑路径均可使用。

## 文件搜索契约

`glob` 与 `grep.include` 共用通配规则：`*.go` 在任意深度匹配文件名，`**` 作为完整路径分段匹配零层或多层目录；含 `/` 的相对模式相对于搜索 `path`，前导 `/` 相对于逻辑工作区根。支持 `*`、`?` 和字符集合，不支持 brace expansion。非法模式返回参数错误，不使用后缀匹配兜底。`grep.query` 为保留空白的字面量，默认不区分大小写。

搜索仅读取普通文本文件，跳过符号链接、socket、FIFO、设备和二进制内容。`GlobDetailed` 与 `GrepDetailed` 返回结果及 `SearchDiagnostics`，标明截断、跳过计数和至多 100 条诊断；原 `Glob`、`Grep` 是只返回条目的兼容接口。默认 glob 至多 500 个结果，遍历预算为结果上限的四倍；grep 至多 200 个匹配行、20,000 个遍历条目，每个文件至多 4 MiB。达到预算或读取部分文件失败时，模型工具明确提示搜索不完整。

Core 不依赖 Git，也不默认应用忽略规则。宿主可以在使用工具之前设置 `Files.SearchSource`，为每次搜索提供受过滤的文件系统；模型工具的 `include_ignored` 传递给该策略，未配置策略时不改变范围。直接读取和文件修改不使用搜索策略。参数校验保持严格，并返回工具名、错误原因、允许字段和必填字段；修复调用仍由模型或宿主显式决定。

`stat` 的 `modified` 输出采用 RFC3339Nano，带 `Z` 或明确时区偏移，保留文件时间的时刻与小数秒精度。事件、文件和后台终端的结构化时间继续使用 `time.Time` 的 RFC3339 JSON 编码；宿主负责界面的本地化和向模型提供当前时钟。默认压缩提示要求保留历史日期与时区的原意，不把旧时钟快照带入摘要作为当前时间。PDF 创建时间保留 PDF 专用格式，并显式标记 UTC（[ISO 32000-1 §7.9.4](https://opensource.adobe.com/dc-acrobat-sdk-docs/standards/pdfstandards/pdf/PDF32000_2008.pdf)）。

## 示例和检查

| 命令 | 路径 |
|---|---|
| `go run ./examples/basic` | 无会话同步请求 |
| `go run ./examples/streaming` | 模型增量回调 |
| `go run ./examples/custom-tool` | 自定义工具和参数 |
| `go run ./examples/custom-model` | 逐步替换模型 |
| `go run ./examples/custom-fs` | 内存文件系统和单独注册的读工具 |
| `go run ./examples/subagent` | 同一运行时上的委派和结果回传 |
| `go run ./examples/relay` | HTTP、SSE 和 WebSocket Relay。可设置 `CLARA_CORE_UPSTREAM_URL` |
| `go run ./examples/acp` | ACP stdio、会话恢复和 MCP 注入 |

```sh
make check
make conformance-openresponses
make conformance-acp
```

[提示词来源与适配](../pkg/core/prompt/SOURCES.md) · [能力和验收记录](../test/conformance/STATUS.md)
