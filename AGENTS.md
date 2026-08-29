# ai-file 项目规则

## 需求依据

编码前先阅读：

1. `docs/prd/prdv1.0.0.md`：产品需求与验收标准。
2. `docs/technical-design-v1.0.0.md`：唯一技术实现依据。
3. `docs/project-memory.md`：已确认的持久化项目决策。

需求与技术方案冲突时，停止编码并请求确认；确认后同步更新相应文档。

## v1 架构红线

- 使用 Go 1.22+；标准库优先，唯一允许的第三方依赖为 `gopkg.in/yaml.v3`。
- 禁止引入 AI Agent 框架、向量数据库、embedding、厂商 Agent SDK、CGO。
- Agent Loop 只依赖自有 `llm.Client`、`tools.Executor`、`memory.Memory` 接口。
- `agent`、`tools`、`memory` 禁止依赖具体 LLM 厂商或 `llm/openaicompat`。
- v1 仅实现 OpenAI-compatible Chat Completions tool calling；默认 Provider 为 DeepSeek、模型为 `deepseek-v4-pro`。
- Tool 通过 Registry 注册；Loop 不得硬编码 `read_file` 或厂家逻辑。

## 安全与行为

- `read_file` 仅可读取 Goal 指定且规范化后的单一文件。
- API Key 不得写入源码、测试夹具、文档样例、stdout、stderr、Memory 或提交记录。
- 成功结果只写 stdout；诊断只写 stderr；失败不得生成半成品或写入 `-out`。
- 不执行用户文件内容；不新增写文件、shell、网络抓取等 Tool，除非需求文档先获更新。

## 开发与验证

- 每个行为变更先写能失败的测试，再实现最小代码。
- 切段、配置、Memory、Tools、Loop、HTTP Adapter 分层测试；默认测试禁止真实网络请求。
- `go test ./...`、`go vet ./...`、`gofmt` 是每次代码交付的最低验证。
- 代码保持小包、窄接口、显式错误处理；公共类型和跨包契约必须有 GoDoc。
- 不修改 PRD、技术方案或本文件以规避测试失败；需求变化应先更新文档并获确认。
