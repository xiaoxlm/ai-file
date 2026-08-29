# 产品需求文档（PRD）

**产品名称**：文件内容分析 Agent（工作名：ai-file）  
**版本**：v1.1  
**状态**：待评审  
**来源**：`docs/first-project-desc.md`  
**文档用途**：统一产品边界与可实现契约，作为后续设计与编码的唯一需求依据。

---

## 1. 背景与问题

用户手头有文本文件，需要快速抓住「每一段在说什么」，而不是通读全文。现有做法要么人工摘抄，要么把整篇丢给聊天窗口、结果不可复现、也没有可扩展的工具层。

v1 要交付一个**可本地运行的轻量 Agent**：给定文件路径，读取内容，按段提炼核心，用最短文字列出。不引入任何 AI Agent 框架（LangChain、LlamaIndex、Haystack 等），核心能力必须手写：`LLM + Loop + Tools + Memory + Goal`，行为上体现 **ReAct**（推理 → 行动 → 观察，循环直到达成目标）。

---

## 2. 目标与成功标准

### 2.1 产品目标

| ID | 目标 |
|----|------|
| G1 | 用户一条命令即可对指定文件得到「逐段核心要点」列表 |
| G2 | Agent 运行时可见 ReAct：有明确 Goal、会调 Tool、会把 Observation 写入 Memory、循环直到结束 |
| G3 | Tools / Memory / LLM 以接口隔离：加工具、换模型厂家都不改 Loop；v1 Memory 仅内存，LLM 默认 DeepSeek |
| G4 | 实现保持轻量：CLI、无 Web、无向量库、无第三方 Agent 框架 |

### 2.2 成功标准（可验收）

1. 对一份含 ≥3 个自然段的 UTF-8 文本，命令退出码为 0，stdout 输出与段数对应的要点列表（空段不计）。
2. 运行日志（stderr 或 `-verbose`）能看到至少一轮：Thought / Action(工具名+参数) / Observation。
3. 更换 LLM 厂家/模型或新增 Tool 时，**不必修改** Agent Loop 的控制流代码（只改配置、Provider 注册或适配器）。
4. 进程内 Memory 不依赖 Qdrant / Milvus / 任何外部向量库。
5. `go.mod` 中不出现 LangChain、云厂商 Agent SDK 等 Agent 框架依赖。
6. 切换到另一家 **OpenAI 兼容** 接口时，只改 `provider` / `base_url` / `model` / `api_key`，不改业务代码。

### 2.3 非目标（v1 明确不做）

- Web / GUI、多用户、鉴权、云端服务化
- 向量检索、Embedding、外部向量数据库
- 多文件批量、目录递归分析（可作为后续 Tool 扩展）
- 二进制文件、扫描件 OCR、音视频
- 流式打字机 UI、对话式多轮闲聊（v1 是「一次任务跑完」）
- 自动写入原文件或生成新文件（默认只打 stdout；见 6.4）
- 多线协议并存（Anthropic Messages 等）；v1 只做 OpenAI 兼容传输，换厂家靠配置预设

---

## 3. 用户与场景

**唯一用户**：本机开发者 / 产品经理，在终端分析自己的文档。

**主路径**：把一份说明、纪要或笔记文件交给 Agent，立刻得到逐段摘要，用于评审或二次加工。

**次路径**：加 `-verbose` 观察 Agent 如何读文件、如何决策，用于调试与教学（验证 ReAct 是否真实存在）。

---

## 4. 范围

### 4.1 In Scope

- CLI：指定文件路径，执行分析，打印结果
- Agent 内核：Goal 注入、ReAct Loop、Tool 调用、Memory 读写、终止条件
- 内置 Tool：`read_file`（必选）；`finish`（提交最终答案，见 7.3）
- Memory：进程内实现（对话轨迹 + KV 工作记忆）
- LLM：厂家无关的 `Client` 接口 + Provider 预设；v1 默认 DeepSeek `deepseek-v4-pro`，传输层用 OpenAI 兼容 Chat Completions（含 tool calling）
- 配置：环境变量 + 可选本地配置文件
- 结构化输出：逐段核心要点列表

### 4.2 Out of Scope

见 2.3。另：不承诺「摘要质量接近专职编辑」；质量受模型与提示词约束，本 PRD 约束的是**流程与契约**，不是文笔上限。

---

## 5. 产品决策与假设（编码必须遵守）

原文未写清的点，v1 **锁定如下**。若评审要改，先改本表再改代码。

| # | 决策 | 选择 |
|---|------|------|
| A1 | 形态 | 单二进制 CLI，不是 HTTP 服务 |
| A2 | 一次任务 | 只分析 **一个文件** |
| A3 | 输入类型 | UTF-8 文本（`.md` / `.txt` / `.go` 等）；扩展名不限，以能否按 UTF-8 读入为准 |
| A4 | 「段」的定义 | 按空行切分（连续空行视为一个分隔）；切分后 trim，空块丢弃。若全文无空行，则整篇视为 **1 段** |
| A5 | 要点粒度 | **一段 → 一条**核心要点；一条尽量一句话，原则上不超过 40 个汉字或 80 个英文字符 |
| A6 | 输出语言 | 与源文件主要语言一致；中英混排时用中文 |
| A7 | 切分位置 | **本地确定性切分**（读入后由代码切段），不把切段交给 LLM，避免段数漂移 |
| A8 | Agent 如何分析 | 切段后把带序号的段列表交给 LLM；LLM 只做提炼，不负责发现文件、不负责切段 |
| A9 | LLM 协议 | Agent 只认自有 `llm.Client`，**不认**任何厂家 SDK 类型。v1 传输层：OpenAI 兼容 Chat Completions + **tool calling**（不用纯文本伪 JSON 作为主路径） |
| A10 | 框架 / SDK | 禁止 Agent 框架。LLM 调用用自有 HTTP 客户端或薄封装；**禁止**把 OpenAI/DeepSeek SDK 类型泄漏进 `agent` / `tools` / `memory` |
| A11 | Memory | 仅内存；进程退出即清空；无持久化、无向量 |
| A12 | 默认输出 | stdout 为用户结果；诊断走 stderr |
| A13 | 失败策略 | 文件不存在 / 超限 / LLM 失败 / 超过最大轮次 → 非 0 退出，stderr 说明原因，stdout 不输出半成品列表 |
| A14 | 默认厂家 | Provider=`deepseek`，Base URL=`https://api.deepseek.com`，Model=`deepseek-v4-pro` |
| A15 | 换厂家 | 同一协议内（OpenAI 兼容）只改配置；非 OpenAI 协议（如 Anthropic Messages）v1 不做，但接口预留，后续只加 Adapter |

---

## 6. 用户体验

### 6.1 命令

```text
ai-file <文件路径>
ai-file -verbose <文件路径>
ai-file -out <结果文件> <文件路径>
```

- 路径相对于当前工作目录，也接受绝对路径。
- `-verbose`：在 stderr 打印每轮 Thought / Action / Observation（截断过长 Observation，默认头 2KB）。
- `-out`：可选；若提供，将与 stdout 相同的结果再写一份到该路径（不覆盖源文件）。未指定则只打 stdout。

不提供交互式 REPL。

### 6.2 用户可见结果格式（stdout）

纯文本，便于复制：

```text
文件: /abs/or/given/path
段数: N

1. <要点>
2. <要点>
…
N. <要点>
```

- 编号从 1 开始，与切段顺序一致。
- 要点中禁止再套多层编号；禁止复述原文大段。
- 不得输出 Thought、工具参数、原始文件全文。

### 6.3 配置

优先级：**命令行 flag > 环境变量 > 配置文件 > 内置默认**。

| 项 | 环境变量 / flag | 默认 |
|----|-----------------|------|
| Provider | `AI_FILE_PROVIDER` / `-provider` | `deepseek` |
| API Key | `AI_FILE_API_KEY` | 无，缺失则启动失败 |
| Base URL | `AI_FILE_BASE_URL` / `-base-url` | 随 Provider 预设（deepseek → `https://api.deepseek.com`） |
| Model | `AI_FILE_MODEL` / `-model` | 随 Provider 预设（deepseek → `deepseek-v4-pro`） |
| 最大 ReAct 轮次 | `AI_FILE_MAX_STEPS` | `8` |
| 文件大小上限 | `AI_FILE_MAX_BYTES` | `512KiB` |
| 单段最大字符 | `AI_FILE_MAX_PARA_CHARS` | `8000`（超出截断并在该段要点前加 `[截断]`） |

可选配置文件：`./ai-file.yaml` 或 `$HOME/.ai-file.yaml`（后者仅当前者不存在）。字段与上表对应，snake_case。

**换厂家（v1，同一协议）**：改 `provider`（吃预设）或显式覆盖 `base_url` + `model` + `api_key`。示例：

```yaml
# 切到另一家 OpenAI 兼容接口，不必改代码
provider: openai          # 或 custom
base_url: https://api.openai.com/v1
model: gpt-4o-mini
api_key: "..."            # 也可用环境变量，禁止把 key 写入仓库
```

### 6.4 边界体验

| 情况 | 行为 |
|------|------|
| 路径不是普通文件 | 退出 2，说明「需要文件而非目录」 |
| 文件超过大小上限 | 退出 2，提示当前大小与上限 |
| 非 UTF-8 / 明显二进制（NUL 字节） | 退出 2，拒绝分析 |
| 0 段（空文件或仅空白） | 退出 0，stdout 为 `段数: 0` 及一行「无有效段落」 |
| LLM 超时或 4xx/5xx | 退出 1，stderr 带状态/错误摘要，不重试超过 2 次 |
| 达到 max steps 仍未 `finish` | 退出 1，提示「未在限制轮次内完成」 |

---

## 7. 功能需求

### 7.1 Goal

每次运行构造一条不可变 Goal，写入 Memory 的系统侧，并在整个 Loop 中保持不变：

> 读取用户指定文件，按空行将正文分为段落，对每一段用最言简意赅的一句话写出核心内容，按原顺序编号输出。禁止编造文件中不存在的信息。完成后必须调用 `finish`。

Goal 中包含**绝对路径**（解析后），避免相对路径歧义。

### 7.2 ReAct Loop（必须实现，不可省略）

每一轮顺序固定：

1. **Think**：若本轮 `content` 非空，视为 Thought；仅有 tool_calls、content 为空时 Thought 可为空（`-verbose` 打印 `(empty)`），不因此失败。
2. **Act**：执行 0..N 个 tool call（同一轮多个则**按返回顺序串行**执行）。
3. **Observe**：每个工具的返回字符串写入 Memory。
4. 若本轮调用了 `finish` 且参数校验通过 → **终止**，渲染 stdout。
5. 否则继续，直到 `max_steps`。

约束：

- Loop **不知道**具体有哪些业务工具，只认 `Tool` 接口与注册表。
- 禁止在 Loop 里写死 `read_file` 的业务逻辑（读文件只发生在 Tool 内）。
- 切段逻辑属于「读文件之后的本地处理」，放在 `read_file` 的实现内部（见 7.3），这样 Agent 观察到的是「已切段文本」，减少 LLM 胡乱切段。

建议系统提示（实现可微调措辞，语义不可弱化）：

- 你必须通过工具获取文件内容，不要假装已经读过。
- 拿到段落后直接提炼并 `finish`，不要无意义循环。
- `finish` 的 `items` 数量必须等于段落数。

### 7.3 Tools

#### 7.3.1 抽象（后续加工具的契约）

每个 Tool 必须提供：

- `Name()`：稳定、小写、下划线，如 `read_file`
- `Description()`：给 LLM 看的一句话 + 何时使用
- `InputSchema()`：JSON Schema（object）
- `Execute(ctx, jsonArgs) (observation string, err error)`

注册表：

- `Register(Tool)`：重名则启动失败
- `List()`：供 LLM 的 tools 列表
- `Get(name)` / `Execute(name, args)`

v1 内置两个工具。未来新增（如 `list_dir`、`search`）只 `Register`，不改 Loop。

#### 7.3.2 `read_file`

| 项 | 约定 |
|----|------|
| 参数 | `{ "path": string }`，必填 |
| 权限 | 只读；拒绝 path 为空 |
| 路径 | 相对路径相对 **进程 cwd**；不允许读 Goal 指定文件之外的路径（v1 安全默认：规范化后必须与 Goal 文件为同一路径） |
| 成功 Observation | 见下方格式；这是 LLM 后续提炼的唯一正文来源 |
| 失败 | Observation 为 `error: ...` 字符串，**不**把 Go error 直接当成功结束 |

成功 Observation 格式（给 LLM，不是给用户）：

```text
path: <abs>
paragraph_count: N
paragraphs:
[1] <段1全文>
[2] <段2全文>
...
```

单段超过 `AI_FILE_MAX_PARA_CHARS` 时截断，并在该段末尾追加 `\n[truncated]`。

同一路径在同一次运行中再次调用：允许，可走 Memory 缓存，Observation 内容与首次成功时一致。

#### 7.3.3 `finish`

| 项 | 约定 |
|----|------|
| 参数 | `{ "items": [string, ...] }` |
| 校验 | `len(items) == paragraph_count`（以本次运行中最近一次成功 `read_file` 的段数为准）；未读文件就 `finish` → 失败 Observation，Loop 继续 |
| 成功 | Loop 结束；`items[i]` 对应第 i+1 段用户要点 |
| 失败 | 返回明确错误（段数不符 / 空 items 但段数非 0），不退出进程 |

### 7.4 Memory

接口最小集：

- 追加消息：`system` / `user` / `assistant` / `tool`（tool 消息需带 tool_call_id）
- 读取当前对话窗口（供 LLM 请求）
- KV：`Set/Get`，供工具或 Loop 存放 `paragraph_count`、文件绝对路径等
- `Clear`：仅测试需要；CLI 一次进程不用中途清空

实现：Go slice + `map[string]string`，**无锁要求以外的并发**（v1 Loop 单线程；若工具未来并行，再加锁）。禁止在 v1 引入 embedding 或相似度检索。

窗口策略：全量保留本次运行消息。单条 Observation 超过 32KiB 时截断尾部并加标记。v1 不做跨进程摘要压缩。

### 7.5 LLM 抽象（换厂家的核心契约）

用户会频繁切换不同厂家的大模型。**Loop / Tool / Memory 禁止绑定某一厂家。** 换模型的代价必须是：改配置，或最多新增一个 Adapter 文件。

#### 7.5.1 分层

```text
agent.Loop  ──调用──►  llm.Client（自有类型，厂家无关）
                          │
                          ▼
                    llm.Provider 工厂（按 name 选 Adapter）
                          │
              ┌───────────┼───────────┐
              ▼           ▼           ▼
         deepseek      openai       custom
         Adapter      Adapter      Adapter
              └───────────┬───────────┘
                          ▼
              v1 仅实现：OpenAI 兼容 HTTP
              POST /chat/completions + tools
```

- `llm.Client`：Loop 唯一依赖。方法语义是「一轮对话补全」，不是「调用 DeepSeek」。
- `ChatRequest` / `ChatResponse`：项目自有结构（messages、tools、tool_calls、content）。**不得**出现 `openai.ChatCompletionXxx` 或 DeepSeek SDK 类型。
- `Provider`：配置名（`deepseek` / `openai` / `custom`）→ 默认 `base_url`、默认 `model`、鉴权头。厂家特有字段只能留在 Adapter 内，通过可选 `extra` 透传，Loop 不可读。

#### 7.5.2 `Client` 最小接口

```text
Chat(ctx, ChatRequest) → (ChatResponse, error)

ChatRequest:
  Messages []Message          # system/user/assistant/tool
  Tools    []ToolSpec         # name/description/json schema
  Model    string             # 可空，空则用配置默认

ChatResponse:
  Content    string           # Thought，可空
  ToolCalls  []ToolCall       # id/name/arguments_json
  FinishReason string         # stop / tool_calls / 其它（仅诊断）
```

组装：`llm.New(cfg) (Client, error)`。未知 `provider` 名 → 启动失败，列出已注册名。

#### 7.5.3 v1 内置 Provider 预设

| Provider | 默认 Base URL | 默认 Model | 鉴权 |
|----------|---------------|------------|------|
| `deepseek`（默认） | `https://api.deepseek.com` | `deepseek-v4-pro` | `Authorization: Bearer <api_key>` |
| `openai` | `https://api.openai.com/v1` | 无默认，必须显式配 `model` | 同上 |
| `custom` | 无默认，必须显式配 `base_url` 与 `model` | — | 同上 |

说明：

- v1 **只实现一种线协议**：OpenAI 兼容 Chat Completions + tool calling。DeepSeek 官方即此协议，故默认厂家用 DeepSeek Adapter（实质是带预设的同一 HTTP 实现）。
- `openai` / `custom` 与 `deepseek` 共用同一 Adapter 代码路径，差别只在预设与配置；这样切通义、Moonshot、本地 vLLM 等兼容接口时无需新代码。
- Anthropic Messages 等非兼容协议：**v1 不做**；以后新增 `anthropic` Adapter 实现同一个 `Client` 即可。
- DeepSeek 的 `thinking` / `reasoning_effort` 等厂家字段：**不进入** `ChatRequest`。v1 默认不开启（避免与 ReAct Loop 双重思考）。若以后要开，只加在 deepseek Adapter 的可选配置里。

#### 7.5.4 行为与失败

- 输入：messages + tools schema；输出：content（可选）+ tool_calls（可选）
- 无 tool_calls 且无 `finish`：视为模型违规；将该轮 content 记入 Memory，并追加纠偏「必须调用工具或 finish」，进入下一轮（计入 max_steps）
- Token / 费用：v1 不展示；超时默认 60s/请求
- HTTP 4xx/5xx：Adapter 返回 error，错误信息带 status 与厂家响应摘要，**不**暴露 api_key

### 7.6 日志与可观测性

- 默认：stderr 仅错误
- `-verbose`：每步打印 step 序号、Thought、Action 名与参数、Observation 预览
- 禁止在任何日志中打印完整 API Key

---

## 8. 信息架构（给实现用的模块边界）

逻辑模块（包名可调整，**依赖方向不可反**）：

```text
cmd/ai-file          → 解析 flag、组装依赖、退出码
internal/config      → 配置加载
internal/agent       → Goal + Loop（只依赖接口）
internal/llm         → Client 接口 + ChatRequest/Response + Provider 工厂
                       + OpenAI 兼容 HTTP Adapter（deepseek/openai/custom 预设）
internal/tools       → Tool 接口 + Registry + read_file/finish
internal/memory      → Memory 接口 + 内存实现
internal/split       → 空行切段（纯函数，供 read_file 调用，可单测）
```

依赖规则：

- `agent` 只 import `llm` 的接口与 DTO，不 import Adapter 实现文件，不出现厂家包名
- `tools` 不 import `agent`
- 新增厂家：优先只改配置；协议不同时只在 `internal/llm` 增加 Adapter 并注册到工厂

---

## 9. 数据流

```text
用户 CLI
  → 解析路径、加载配置
  → 构造 Goal、Memory、Registry(read_file, finish)、LLM
  → Loop:
        Memory 历史 → LLM
        → tool_calls? → Registry.Execute → Observation → Memory
        → finish 成功? → 渲染 6.2 格式 → stdout
  → 否则超轮次失败
```

---

## 10. 非功能需求

| 类别 | 要求 |
|------|------|
| 语言 | Go（与仓库后续代码一致；模块路径以实际 `go.mod` 为准） |
| 体积 | 无 CGO 强依赖；不捆绑模型文件 |
| 性能 | 本地切段 + 通常 2～4 轮 LLM；不设 SLA，但单文件 512KiB 内应在模型延迟主导下完成 |
| 安全 | 默认只能读 Goal 指定的那一个文件；不执行文件内代码；不把 Key 写入 Memory 快照 |
| 测试 | 切段纯函数单测；Registry 重名/未知工具；finish 段数校验；Loop 用 **fake `llm.Client`**（禁止打真实 DeepSeek）测「读→finish」；Provider 工厂：未知名失败、deepseek 预设正确 |
| 兼容 | 开发/运行以当前仓库约定的 Go 版本为准，不低于 Go 1.22 |

---

## 11. 验收用例

| ID | 用例 | 期望 |
|----|------|------|
| AC1 | 三空行分段的中文 `.md` | 3 条要点，顺序正确，无原文整段粘贴 |
| AC2 | 无空行的单段文件 | `段数: 1` 且一条要点 |
| AC3 | 空文件 | `段数: 0`，退出 0 |
| AC4 | 不存在的路径 | 退出 2，无要点列表 |
| AC5 | `-verbose` | stderr 含 `read_file` 与 `finish` 的 Action 记录 |
| AC6 | 模型返回错误段数的 `finish` | Observation 报错，Loop 继续；最终成功或超轮次失败 |
| AC7 | 启动时不注册 `read_file`（测试） | 无法完成金路径，证明业务读文件不在 Loop 内硬编码 |
| AC8 | Loop 单测只注入 fake Client | 不出现 `deepseek` / `openai` 字符串依赖 |
| AC9 | 配置 `provider=custom` + 另一套 base_url/model | 仍走同一 Adapter，Loop 代码零改动 |
| AC10 | 默认配置不改 | 请求发往 `https://api.deepseek.com`，`model=deepseek-v4-pro` |

---

## 12. 里程碑（建议，非排期承诺）

1. 切段 + CLI 骨架 + 配置（无 LLM 也可测切段）
2. Tool/Memory/LLM 接口、内存实现、Provider 工厂（默认 deepseek 预设）
3. ReAct Loop + `read_file` / `finish`
4. DeepSeek `deepseek-v4-pro` 联调与验收用例 AC1–AC5、AC10

---

## 13. 修订记录

| 日期 | 版本 | 说明 |
|------|------|------|
| 2026-08-29 | v1.0 | 根据 `first-project-desc.md` 首版 PRD，补全范围、契约与假设 |
| 2026-08-29 | v1.1 | LLM 厂家抽象：Client/Provider 分层；v1 默认 DeepSeek `deepseek-v4-pro` |
