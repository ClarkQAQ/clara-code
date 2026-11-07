# Clara Core 验收记录

| 需求 | 验证 |
|---|---|
| 可复用 Runtime、模型和工具 | 独立 Go module 通过本地 replace 导入 `clara-code/pkg/core`，注入自定义模型和工具，并用 PrepareStep 换模型。同步、流式、自定义工具和自定义模型示例已运行。 |
| Coding 工具闭环 | `go run ./examples/coding` 和 `TestCodingWorkflow` 在临时 Go 项目中运行测试。用 grep 定位。用 Hashline 修复。第二次测试通过。Hashline 回归测试覆盖过期和未读锚点。 |
| Session 与宿主 | 迁移前的 CLI 写入 Format 1 会话，迁移后的 CLI 和 Web 使用同一会话继续对话，Web 重启后通过 SSE 回放三轮回复。关键回归覆盖 checkpoint 错误、事件游标和中断后保留工具对。 |
| Plan、Goal 和委派 | `TestPlanAndGoalUseHostHooks` 覆盖只读权限、宿主提问、Goal 续跑和实际用量。Subagent 示例运行过委派和结果回传。任务测试覆盖工具权限、作用域和 follow-up。 |
| FS 与 Exec | 内存 FS 示例已运行。本地后台进程在工具返回后继续运行，能写 stdin 并收集退出状态。保留 local、memory、virtual、mount、restricted 和 SFTP 后端。 |
| Prompt 和上下文 | 片段来源对照 Codex 和 Clara。回归测试覆盖 Plan Mode、工具可用性和片段替换。目录规则、执行清单、Goal 和任务状态单独装配。压缩为可选的 PrepareStep 钩子。 |
| 严格 JSON | 工具回归覆盖畸形参数、不合法的顶层类型、原调用 ID 和显式修复后的权限检查。Responses 与 Completions 的结构化流覆盖分片、最终校验、显式修复和传输中断。 |
| MCP | 使用官方 Go SDK 的内存 transport 验证初始化、发现、调用和关闭。ACP TCK 另行启动 stdio MCP fixture。 |
| Responses、Completions 与 Relay | 本地 HTTP/SSE fixture 覆盖模型协议和工具调用。Relay 测试覆盖双向转换、WebSocket、续接及压缩。上游 OpenResponses 套件用 Relay 与本地 Responses fixture 运行。 |
| 搜索、语音、图片、文档及技能 | 提供独立工具和宿主注入入口。图片及文档处理有本地检查。外部搜索和语音服务未实连。 |

## OpenResponses

在 `openresponses/openresponses` 的 `dce9ddb91921427c2ba06901c7dfaf0cc7056799` 运行 `bash test/conformance/openresponses.sh`。17 项通过，0 项失败，0 项跳过。

## ACP

在 `agentclientprotocol/acp-tck` 的 `b7f40fb3c003c45926547be5a1f7e2b374035815` 运行 `bash test/conformance/acp.sh`。56 项通过，18 项跳过，0 项失败。31 个必测要求全部通过。结果为 `CONFORMANT`。跳过项涵盖未声明的可选能力和需要认证配置的场景。

## 实连范围

当前环境未配置真实模型、SSH 或 SFTP 端点。这些端点的认证、模型行为和远程进程未实连验证。示例的 transcript 和事件可以持久化。Plan、Goal 和任务扩展状态保存在宿主进程内。ACP 外部进程按宿主配置启动。ACP 后端可排队 follow-up，进行中的 steering 由协议能力限制。

