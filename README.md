# ai-file

一个轻量级、手写实现的文件内容分析 Agent。它读取一个 UTF-8 文本文件，按空行分段，并用大模型为每段生成一条简洁摘要。

核心实现包含 `Goal + LLM + ReAct Loop + Tools + Memory`，不依赖 Agent 框架、embedding 或向量数据库。

## 目录结构

```text
.
├── cmd/
│   └── ai-file/                 # CLI 入口：解析参数、加载配置、映射退出码
├── internal/
│   ├── agent/                   # Goal、系统提示、ReAct Loop 与摘要结果渲染
│   ├── app/                     # 应用编排：输入预检、依赖组装、stdout/stderr 输出
│   ├── config/                  # YAML、环境变量、命令行参数的配置合并与校验
│   ├── llm/                     # 厂家无关的 LLM 接口、DTO 与 Provider 工厂
│   │   └── openaicompat/        # OpenAI-compatible Chat Completions HTTP Adapter
│   ├── memory/                  # 单次运行的会话消息与 KV 工作记忆
│   ├── split/                   # 本地确定性空行切段
│   └── tools/                   # Tool 抽象、注册表、read_file 与 finish
├── docs/                        # PRD、技术方案与项目决策记录
├── AGENTS.md                    # 开发 Agent 必须遵守的项目规则
├── go.mod                       # Go 模块与依赖定义
└── README.md                    # 项目说明与使用指南
```

## 要求

- Go 1.27+
- 一个支持 OpenAI-compatible Chat Completions tool calling 的模型服务
- API Key（仅通过环境变量或本地配置文件提供，不要提交到仓库）

默认模型服务：

| 项 | 默认值 |
|---|---|
| Provider | `deepseek` |
| Base URL | `https://api.deepseek.com` |
| Model | `deepseek-v4-pro` |

## 构建

```bash
go build -o ai-file ./cmd/ai-file
```

## 使用用例

### 1. 使用默认 DeepSeek 配置分析 Markdown

```bash
export AI_FILE_API_KEY='你的 DeepSeek API Key'
./ai-file docs/first-project-desc.md
```

示例输出：

```text
文件: /absolute/path/docs/first-project-desc.md
段数: 3

1. 第一段的核心内容
2. 第二段的核心内容
3. 第三段的核心内容
```

### 2. 查看 Agent 的 ReAct 执行过程

`-verbose` 将 Thought、Tool Action 与 Observation 写到 stderr；正常结果仍只写 stdout。

```bash
./ai-file -verbose docs/first-project-desc.md
```

### 3. 同时输出到终端和结果文件

```bash
./ai-file -out summary.txt docs/first-project-desc.md
```

只有分析完整成功后才会写入 `summary.txt`，源文件不可作为 `-out` 目标。

### 4. 切换到另一家 OpenAI-compatible 服务

```bash
export AI_FILE_API_KEY='你的服务 API Key'
./ai-file \
  -provider custom \
  -base-url https://example.com/v1 \
  -model your-model-name \
  docs/first-project-desc.md
```

`openai` Provider 会预设 `https://api.openai.com/v1`，但必须显式指定模型：

```bash
export AI_FILE_API_KEY='你的 OpenAI API Key'
./ai-file -provider openai -model gpt-4o-mini docs/first-project-desc.md
```

## 配置

配置优先级（从低到高）：

```text
内置默认值 → Provider 预设 → ./ai-file.yaml
→ $HOME/.ai-file.yaml（仅当前者不存在）→ AI_FILE_* 环境变量 → CLI flags
```

本地 `ai-file.yaml` 示例：

```yaml
provider: deepseek
base_url: https://api.deepseek.com
model: deepseek-v4-pro
max_steps: 8
max_bytes: 524288
max_para_chars: 8000
```

API Key 推荐使用环境变量：

```bash
export AI_FILE_API_KEY='你的 API Key'
```

支持的环境变量：

| 变量 | 说明 |
|---|---|
| `AI_FILE_PROVIDER` | `deepseek`、`openai` 或 `custom` |
| `AI_FILE_API_KEY` | API Key，必填 |
| `AI_FILE_BASE_URL` | OpenAI-compatible API Base URL |
| `AI_FILE_MODEL` | 模型名 |
| `AI_FILE_MAX_STEPS` | 最大 ReAct 轮次，默认 `8` |
| `AI_FILE_MAX_BYTES` | 最大文件字节数，默认 `524288` |
| `AI_FILE_MAX_PARA_CHARS` | 单段最大字符数，默认 `8000` |

命令行参数：

```text
ai-file [-provider value] [-base-url value] [-model value] [-verbose] [-out path] <文件路径>
```

## 行为与限制

- 一次仅分析一个普通文件。
- 仅接受有效 UTF-8、无 NUL 字节的文本；默认最大 512KiB。
- 使用空行分段，连续空行视为同一分隔符。
- 文件为空时不调用 LLM，输出 `段数: 0`；但仍须先提供有效 API Key。
- `read_file` 仅能读取用户指定的目标文件，不能访问其他路径。
- 文件内容会被发送至当前配置的 LLM Provider；勿用于不应外发的敏感内容。

退出码：

| 退出码 | 含义 |
|---|---|
| 0 | 成功 |
| 1 | LLM、输出或内部运行失败 |
| 2 | 配置、路径、文件大小或编码错误 |

## 开发验证

```bash
gofmt -w .
go vet ./...
go test ./...
go test -race ./...
go build -o ai-file ./cmd/ai-file
```

默认测试不会请求真实 LLM API。
