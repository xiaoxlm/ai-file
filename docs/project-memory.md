# 项目记忆：ai-file v1

**用途**：保存已确认、会影响后续实现的项目决策。它是开发 Agent 的持久化上下文，不是应用运行时的 Memory 实现。

**来源**：[PRD](prd/prdv1.0.0.md) 与 [技术方案](technical-design-v1.0.0.md)。后两者是完整依据；本文件仅保留高频决策索引。

## 当前阶段

- 已完成：产品需求与技术方案。
- 未开始：任何 Go 业务代码、`go.mod`、CLI 或 LLM 联调。
- 下一步：收到实施指令后，按技术方案第 12 节分阶段开发。

## 产品边界

- 本地单二进制 CLI，一次只分析一个 UTF-8 文本文件。
- 按空行确定性切段：trim 后丢弃空块；无空行即一段。
- 每段生成一条简洁要点，维持原顺序。
- API Key 是启动前置条件；配置校验通过后，空文件不调用 LLM，直接输出 `段数: 0` 和 `无有效段落`。
- 不做 Web、用户系统、多文件/目录、OCR、embedding、向量检索、跨进程 Memory。

## Agent 不变量

- 必须体现 ReAct：Goal → Think → Act → Observe → Memory，直到成功 `finish` 或达到上限。
- 默认最大 8 步；同轮 ToolCall 按模型返回顺序串行执行。
- Loop 不包含任何具体工具或厂商业务逻辑。
- `read_file` 是唯一正文来源；只可读取规范化后等于 Goal 的目标文件。
- `finish.items` 条数必须等于最近一次成功读取的段数；不匹配时返回 Observation 并继续 Loop。
- Memory 仅当前进程：消息列表 + KV；单条 Observation 最多 32KiB。

## LLM 与配置

- 抽象：自有 `llm.Client` / `ChatRequest` / `ChatResponse` / `ToolSpec`，禁止厂家 SDK 类型越过 `internal/llm`。
- v1 协议：OpenAI-compatible `POST /chat/completions` 与 tool calling。
- 默认：`provider=deepseek`，`base_url=https://api.deepseek.com`，`model=deepseek-v4-pro`。
- 同协议切换厂家只修改 `provider`、`base_url`、`model`、`api_key` 配置。
- DeepSeek thinking/reasoning 字段 v1 不发送。
- 配置优先级：CLI flags > `AI_FILE_*` 环境变量 > `./ai-file.yaml` > `$HOME/.ai-file.yaml` > 默认。
- API Key 不设 CLI flag；优先使用 `AI_FILE_API_KEY`。

## 错误、输出与安全

- 退出码：0 成功；1 LLM/超轮次/内部失败；2 输入或配置错误。
- 成功只写 stdout；verbose 与错误只写 stderr；失败不输出摘要、不写 `-out`。
- 输入预检：普通文件、≤512KiB、无 NUL 字节、合法 UTF-8。
- HTTP：60 秒请求超时；仅网络临时错误、429、5xx 重试两次（共三次）；其余 4xx 不重试。
- 不记录、输出或提交 API Key；用户指定文件正文会发送给所选 Provider。

## 开发约束

- Go 1.22+，标准库优先；唯一允许的第三方依赖：`gopkg.in/yaml.v3`。
- 计划包边界：`cmd/ai-file`、`internal/app`、`config`、`agent`、`llm`、`memory`、`split`、`tools`。
- 先测试后实现；默认测试用 fake LLM，真实 DeepSeek 只作手动或 integration tag 联调。
- 验收以 PRD AC1–AC10 为准。
