# Sion — AI Desktop Companion

Sion 是一个 AI 桌面伙伴应用，运行在 macOS 上，提供情感陪伴、主动关怀、记忆学习和插件扩展能力。

**v2.2** | Go 1.25 | Vue 3 + Electron | SQLite | LLM (Ollama / OpenAI-compatible)

---

## 核心特性

| 模块 | 说明 |
|------|------|
| **情感系统** | 8 维内部情感向量（好感/烦躁/孤独/好奇/自信/困倦/调皮/担心）→ PAD 三维外显 + EMA 平滑 + 昼夜节律 |
| **主动认知** | System 1（纯数学评分）/ System 2（LLM 推理）双轨决策，16 种动作 × 5 维驱动力 dot-product 评分 |
| **记忆管道** | L0 工作记忆 → L1 日记 → L2 事实 → L3 策略，双信号半衰期证据引擎 + BM25/向量混合召回 |
| **DPO 学习** | 从行为结果中批量更新动作权重矩阵，自动审计卡死动作和权重漂移 |
| **策略反思** | 定期 LLM 驱动的策略蒸馏，从日记+事实+线程中提取可复用的行为原则 |
| **好奇引擎** | 扫描知识缺口（事实矛盾/休眠线程/不完整模式），调度探索行为 |
| **插件系统** | SDK（Plugin/PluginContext/FunctionProvider/UIProvider）+ 6 个内置插件（chat/memory/vision/search/qq/timer） |
| **多 Provider LLM** | 支持多 LLM 后端 fallback 链、路由表、健康检查、速率限制、Token 用量追踪 |

---

## 架构

```
Transport  ── HTTP + SSE ──────────────────────────────
App        ── Runtime + ChatOrchestrator + Services ──
Adapter    ── Emotion | Memory | Learning | Proactive | Perception | LLM | Tool
Domain     ── cognition | emotion | memory | types ──
Port       ── 全接口定义 (13 个文件) ──────────────────
Plugin     ── SDK + 6 builtin plugins ─────────────────
```

**铁律**：模块间仅通过 `port` 接口通信，Domain 层零外部依赖，所有模块统一 `Init → Start → Stop` 生命周期。

---

## 项目结构

```
sion-v1/
├── cmd/sion/main.go              # 入口点
├── internal/
│   ├── port/                     # 接口定义层（宪法）
│   │   ├── learning.go           #   Learner / StrategyAgent / CuriosityEngine
│   │   ├── cognition.go          #   FeatureComputer / DriveComputer / ActionScorer / DecisionRouter
│   │   ├── memory.go             #   MemoryStore / EvidenceEngine / MemoryRecall
│   │   ├── emotion.go            #   EmotionStateManager / EmotionSignalSource
│   │   ├── llm.go                #   LLMExecutor / EmbeddingService / ProviderRegistry
│   │   ├── event.go              #   EventBus + 标准 Topic 常量
│   │   └── ...
│   ├── domain/
│   │   ├── cognition/            # 纯领域逻辑
│   │   │   ├── actions.go        #   16 动作注册表 + 权重向量
│   │   │   ├── drives.go         #   52D 特征 → 5D 驱动力
│   │   │   ├── router.go         #   System 1 / System 2 路由
│   │   │   ├── needs.go          #   6D 内在需求（稳态衰减）
│   │   │   ├── motivator.go      #   动作评分（dot-product + 上下文调制）
│   │   │   ├── learner.go        #   DPO 批量权重更新
│   │   │   ├── strategy.go       #   反思调度 + 结果模式分析
│   │   │   └── features.go       #   Tier 1 特征计算
│   │   ├── emotion/              #   情感域逻辑（衰减/平滑/昼夜节律）
│   │   ├── memory/               #   记忆域逻辑（证据/遗忘/合并）
│   │   └── types/                #   共享数据类型
│   ├── adapter/
│   │   ├── learning/             # 学习适配器实现
│   │   ├── emotion/              # 情感存储 + LLM 评估器
│   │   ├── memory/               # SQLite 存储 + 证据引擎 + 召回 + 压缩
│   │   ├── proactive/            # 认知 Tick + 意图调度 + 投递门控
│   │   ├── perception/           # 屏幕观察 + 应用分类
│   │   ├── llm/                  # OpenAI Gateway + Provider Registry + Rate Limiter
│   │   ├── tool/                 # Agent Loop + 内置工具 + Computer Use + Browser
│   │   ├── event/                # EventBus 实现
│   │   └── config/               # 配置管理
│   ├── app/
│   │   ├── runtime.go            # 中央装配点
│   │   └── modules/              # 服务编排 (Memory/Emotion/LLM/Learning)
│   └── transport/
│       ├── http/                 # HTTP API
│       └── sse/                  # Server-Sent Events
├── plugin/
│   ├── sdk/                      # 插件 SDK（Plugin/Context/Lifecycle/Function/UI）
│   └── builtin/                  # 6 个内置插件
│       ├── chat/                 #   系统提示词 + 对话钩子 + 函数工具
│       ├── memory/               #   记忆管道
│       ├── vision/               #   截图 + 视觉分析
│       ├── search/               #   网页搜索
│       ├── qq/                   #   QQ Bot 中继
│       └── timer/                #   定时提醒
├── docs/                         # 架构文档
├── frontend/                     # Vue 3 + Electron 前端
└── Makefile
```

**统计**：178 个 Go 文件，约 28,800 行代码

---

## 快速开始

### 环境要求
- Go 1.25+
- macOS（屏幕感知依赖原生 API）
- SQLite 3
- LLM 后端（Ollama 本地 或 OpenAI-compatible API）

### 构建

```bash
make build
# 或
go build -o sion ./cmd/sion/
```

### 运行

```bash
export SION_LLM_URL="http://localhost:11434/v1"   # Ollama
export SION_LLM_MODEL="qwen2.5:7b"
export SION_DATA_DIR="$HOME/.sion"

./sion
```

### 配置

运行时配置文件位于 `~/.sion/`：
- `sion.db` — SQLite 数据库（记忆、情感、事实）
- `emotion.json` — 情感状态持久化
- `personality.json` — 人格配置

---

## 设计原则

1. **模块间仅通过 `port` 接口通信** — 禁止直接 import 其他模块的 adapter 包
2. **Domain 层零外部依赖** — 纯 Go struct + 纯函数
3. **统一生命周期** — `Init → Start → Stop`，由 Runtime 统一编排
4. **一个 Adapter 实现一个 Port 接口** — 替换实现无需改动业务代码
5. **插件间通过 EventBus 通信** — 插件永远不直接 import 其他插件

---

## License

MIT
