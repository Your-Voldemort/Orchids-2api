# Orchids API 完整流程文档

## 目录
1. [系统架构概览](#系统架构概览)
2. [请求处理流程](#请求处理流程)
3. [核心组件](#核心组件)
4. [数据流向](#数据流向)
5. [关键模块详解](#关键模块详解)

---

## 系统架构概览

Orchids API 是一个代理服务器，用于转发和处理 Claude AI 请求。它支持两种上游服务：
- **Orchids** - 官方 Orchids 服务
- **Warp** - 备用 Warp 服务

### 架构图
```
客户端请求
    ↓
[HTTP Server] (cmd/server/main.go)
    ↓
[中间件层] (Middleware)
    ├── 认证 (Auth)
    ├── 会话管理 (Session)
    ├── 并发限制 (Concurrency Limiter)
    └── 追踪 (Trace)
    ↓
[Handler 层] (internal/handler/)
    ├── 请求解析
    ├── Token 缓存
    ├── 摘要缓存
    └── 负载均衡
    ↓
[上游客户端层] (internal/orchids/ & internal/warp/)
    ├── WebSocket 连接池
    ├── Token 管理
    ├── 文件系统操作
    └── 工具调用处理
    ↓
[上游服务]
    ├── Orchids Server (WebSocket)
    └── Warp Server (HTTP/WebSocket)
```

---

## 请求处理流程

### 1. 启动流程 (cmd/server/main.go)

```
启动服务器
    ↓
1. 加载配置 (config.json/config.yaml)
    ↓
2. 初始化日志系统 (slog)
    ↓
3. 初始化 Redis Store
    ├── 账户管理
    ├── API Key 管理
    └── 配置持久化
    ↓
4. 初始化负载均衡器 (LoadBalancer)
    ├── 账户池管理
    ├── 429 重试策略
    └── 账户切换逻辑
    ↓
5. 初始化缓存系统
    ├── Token 缓存 (Memory)
    └── 摘要缓存 (Memory/Redis)
    ↓
6. 注册路由
    ├── /orchids/v1/messages
    ├── /warp/v1/messages
    ├── /orchids/v1/chat/completions (OpenAI 兼容)
    ├── /api/* (管理 API)
    └── /admin/* (Web UI)
    ↓
7. 启动后台任务
    ├── Token 自动刷新 (每 N 分钟)
    └── 会话清理 (每小时)
    ↓
8. 监听端口 (默认 8080)
```

### 2. 消息请求流程 (/orchids/v1/messages)

#### 阶段 1: 请求接收与验证
```
客户端 POST /orchids/v1/messages
    ↓
[并发限制中间件]
    ├── 检查并发数 (默认 100)
    ├── 超时控制 (默认 300s)
    └── 自适应超时调整
    ↓
[Handler.HandleMessages]
    ├── 解析请求体 (ClaudeRequest)
    ├── 验证 API Key
    ├── 提取 conversation_id
    └── 检测重复请求 (2秒窗口)
```

#### 阶段 2: 账户选择与负载均衡
```
[LoadBalancer.GetAccount]
    ↓
1. 从 Redis 获取可用账户列表
    ├── 过滤禁用账户
    ├── 过滤 429 冷却中的账户
    └── 按优先级排序
    ↓
2. 选择账户策略
    ├── 轮询 (Round Robin)
    ├── 随机 (Random)
    └── 优先级 (Priority)
    ↓
3. 返回选中的账户
```

#### 阶段 3: Prompt 构建与优化
```
[Handler.buildUpstreamRequest]
    ↓
1. 提取消息历史
    ├── 用户消息
    ├── 助手消息
    └── 工具调用结果
    ↓
2. 处理 System Prompt
    ├── 合并多个 system 项
    ├── 清理敏感信息
    └── 注入自定义指令
    ↓
3. 工具处理
    ├── 过滤阻塞的工具
    ├── 映射工具名称 (Orchids 特定)
    └── 添加安全工具限制
    ↓
4. Token 计数与压缩
    ├── 计算总 token 数
    ├── 检查是否超限
    └── 应用摘要压缩 (如果启用)
    ↓
5. 构建 UpstreamRequest
    ├── Prompt (legacy)
    ├── Messages (新格式)
    ├── System
    ├── Tools
    ├── Model
    ├── Workdir (动态工作目录)
    └── ChatSessionID
```

#### 阶段 4: 上游请求发送

##### 4.1 Orchids WebSocket 流程
```
[Client.sendRequestWSAIClient]
    ↓
1. 获取 WebSocket 连接
    ├── 从连接池获取 (wsPool.Get)
    │   ├── 复用空闲连接
    │   └── 创建新连接 (如果池为空)
    ├── 获取 JWT Token
    │   ├── 检查缓存
    │   ├── 从 Clerk API 获取
    │   └── 缓存 Token (5分钟 TTL)
    └── 建立 WebSocket 连接
        ├── URL: wss://orchids-server.../agent/coding-agent
        ├── Headers: User-Agent, Origin
        └── 启动 Ping/Pong 心跳
    ↓
2. 发送请求消息
    ├── 构建 JSON payload
    │   ├── prompt
    │   ├── chatHistory
    │   ├── model
    │   ├── messages
    │   ├── system
    │   ├── tools
    │   ├── projectId
    │   ├── chatSessionId
    │   └── workdir (动态)
    └── WebSocket.WriteJSON(payload)
    ↓
3. 接收响应流
    ├── 循环读取 WebSocket 消息
    ├── 解析事件类型
    └── 调用 onMessage 回调
```

##### 4.2 事件处理流程
```
[WebSocket 消息循环]
    ↓
接收到消息 → 解析 JSON
    ↓
根据事件类型分发:

├── "connected"
│   └── 连接建立确认
│
├── "coding_agent.start"
│   └── AI 开始处理
│
├── "coding_agent.initializing"
│   └── 初始化阶段
│
├── "coding_agent.reasoning.chunk"
│   ├── 思考过程流式输出
│   └── 转换为 thinking 块
│
├── "coding_agent.response.chunk"
│   ├── 文本响应流式输出
│   └── 转换为 text 块
│
├── "fs_operation"
│   ├── 文件系统操作请求
│   ├── 调用 handleFSOperation
│   └── 返回操作结果
│
├── "coding_agent.Edit.edit.started"
│   └── 文件编辑开始
│
├── "coding_agent.Edit.edit.chunk"
│   └── 编辑内容流式输出
│
├── "coding_agent.edit_file.completed"
│   └── 单个文件编辑完成
│
├── "tool_call_output_item"
│   └── 工具调用输出
│
├── "coding_agent.tokens_used"
│   └── Token 使用统计
│
├── "response_done" / "coding_agent.end"
│   └── 响应完成
│
└── "complete"
    └── 会话结束
```

#### 阶段 5: 文件系统操作处理

```
[Client.handleFSOperation]
    ↓
1. 解析操作类型
    ├── read - 读取文件
    ├── write - 写入文件
    ├── edit - 编辑文件
    ├── glob - 文件匹配
    ├── grep - 内容搜索
    ├── list - 列出目录
    └── delete - 删除文件
    ↓
2. 路径解析
    ├── 使用 overrideWorkdir (如果提供)
    ├── 否则使用配置的 LocalWorkdir
    └── 安全检查 (防止路径遍历)
    ↓
3. 执行操作
    ├── 调用 fsExecutor (如果设置)
    ├── 否则执行默认实现
    └── 记录性能指标
    ↓
4. 返回结果
    ├── 构建响应 JSON
    │   ├── type: "fs_operation_response"
    │   ├── id: 操作 ID
    │   ├── success: true/false
    │   ├── data: 操作结果
    │   └── error: 错误信息 (如果有)
    └── WebSocket.WriteJSON(response)
```

##### 文件系统操作详解

**Read 操作**:
```go
operation: "read"
path: "/path/to/file.txt"
    ↓
1. 读取文件内容
2. 返回文件内容字符串
```

**Write 操作**:
```go
operation: "write"
path: "/path/to/file.txt"
content: "file content"
    ↓
1. 创建/覆盖文件
2. 写入内容
3. 返回成功状态
```

**Edit 操作**:
```go
operation: "edit"
path: "/path/to/file.txt"
old_string: "old content"
new_string: "new content"
    ↓
1. 读取文件
2. 查找 old_string
3. 替换为 new_string
4. 写回文件
5. 返回成功状态
```

**Glob 操作**:
```go
operation: "glob"
pattern: "**/*.go"
path: "/project/root"
    ↓
1. 使用 filepath.Glob 匹配
2. 返回匹配的文件列表
```

**Grep 操作**:
```go
operation: "grep"
pattern: "func.*Error"
path: "/project/root"
ripgrepParameters: {
    "glob": "*.go",
    "context": 3
}
    ↓
1. 使用 ripgrep 搜索
2. 返回匹配的行和上下文
```

#### 阶段 6: 响应流式传输

```
[Handler.streamResponse]
    ↓
1. 设置 SSE Headers
    ├── Content-Type: text/event-stream
    ├── Cache-Control: no-cache
    └── Connection: keep-alive
    ↓
2. 启动 Keep-Alive
    ├── 每 15 秒发送心跳
    └── 防止连接超时
    ↓
3. 转换上游事件
    ├── 接收 upstream.SSEMessage
    ├── 转换为 OpenAI 格式
    │   ├── content_block_start
    │   ├── content_block_delta
    │   ├── content_block_stop
    │   └── message_stop
    └── 发送到客户端
    ↓
4. 处理工具调用
    ├── 检测 tool_use 块
    ├── 执行工具 (如果启用)
    └── 返回工具结果
    ↓
5. 完成响应
    ├── 发送 message_stop 事件
    ├── 记录 Token 使用
    └── 关闭连接
```

#### 阶段 7: 错误处理与重试

```
[错误分类]
    ↓
├── 认证错误 (401/403)
│   ├── 不重试
│   └── 返回错误给客户端
│
├── 速率限制 (429)
│   ├── 标记账户冷却
│   ├── 切换到下一个账户
│   └── 重试请求 (最多 3 次)
│
├── 服务器错误 (500/502/503)
│   ├── 重试 (指数退避)
│   └── 切换账户 (如果持续失败)
│
├── 超时错误
│   ├── 取消请求
│   └── 返回超时错误
│
└── 网络错误
    ├── 重试 (最多 3 次)
    └── 返回错误
```

---

## 核心组件

### 1. Handler (internal/handler/handler.go)

**职责**:
- 处理 HTTP 请求
- 管理会话和工作目录
- 协调上游客户端
- 流式响应处理

**关键方法**:
- `HandleMessages` - 主请求处理器
- `HandleCountTokens` - Token 计数
- `HandleModels` - 模型列表
- `buildUpstreamRequest` - 构建上游请求
- `streamResponse` - 流式响应

### 2. Orchids Client (internal/orchids/client.go)

**职责**:
- 管理 Orchids 上游连接
- Token 获取与缓存
- WebSocket 连接池管理
- 文件系统操作

**关键方法**:
- `New` - 创建客户端
- `GetToken` - 获取 JWT Token
- `SendRequest` - 发送请求 (legacy)
- `SendRequestWithPayload` - 发送请求 (新)
- `handleFSOperation` - 处理文件系统操作
- `RefreshFSIndex` - 刷新文件索引

### 3. WebSocket 连接池 (internal/upstream/wspool.go)

**职责**:
- 管理 WebSocket 连接复用
- 连接健康检查
- 自动重连

**关键方法**:
- `Get` - 获取连接
- `Put` - 归还连接
- `Close` - 关闭连接池

### 4. 负载均衡器 (internal/loadbalancer/loadbalancer.go)

**职责**:
- 账户选择策略
- 429 冷却管理
- 账户健康监控

**关键方法**:
- `GetAccount` - 获取可用账户
- `MarkAccount429` - 标记 429 错误
- `GetAccountStatus` - 获取账户状态

### 5. 缓存系统

#### Token 缓存 (internal/tokencache/)
- 缓存 JWT Token
- TTL: 5 分钟
- 减少 Clerk API 调用

#### 摘要缓存 (internal/summarycache/)
- 缓存对话摘要
- 支持 Memory/Redis
- 减少 Token 使用

---

## 数据流向

### 请求数据流
```
客户端请求
    ↓
{
  "model": "claude-sonnet-4-5",
  "messages": [
    {"role": "user", "content": "Hello"}
  ],
  "stream": true
}
    ↓
[Handler 解析]
    ↓
{
  "Prompt": "",
  "Messages": [...],
  "System": [...],
  "Tools": [...],
  "Model": "claude-sonnet-4-5",
  "Workdir": "/path/to/project",
  "ChatSessionID": "abc123"
}
    ↓
[Orchids Client]
    ↓
WebSocket JSON:
{
  "prompt": "",
  "messages": [...],
  "system": [...],
  "tools": [...],
  "model": "claude-sonnet-4-5",
  "projectId": "proj_123",
  "chatSessionId": "abc123",
  "workdir": "/path/to/project"
}
    ↓
[Orchids Server]
```

### 响应数据流
```
[Orchids Server]
    ↓
WebSocket JSON:
{
  "type": "coding_agent.response.chunk",
  "event": {
    "delta": {"text": "Hello!"}
  }
}
    ↓
[Orchids Client]
    ↓
upstream.SSEMessage:
{
  "Type": "coding_agent.response.chunk",
  "Event": {"delta": {"text": "Hello!"}}
}
    ↓
[Handler 转换]
    ↓
OpenAI SSE 格式:
event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello!"}}
    ↓
[客户端]
```

---

## 关键模块详解

### 1. Token 管理流程

```
[GetToken 调用]
    ↓
1. 检查配置中的 UpstreamToken
    ├── 如果存在 → 直接返回
    └── 否则继续
    ↓
2. 检查 Token 缓存
    ├── 缓存命中 → 返回
    └── 缓存未命中 → 继续
    ↓
3. 调用 fetchToken
    ├── 构建 Clerk API URL
    │   └── https://clerk.orchids.app/v1/client/sessions/{sessionId}/tokens
    ├── 设置 Headers
    │   ├── Cookie: __client={clientCookie}; __session={sessionCookie}
    │   └── User-Agent
    ├── 发送 POST 请求
    └── 解析响应
        ├── 提取 JWT
        ├── 解析过期时间
        └── 缓存 Token
    ↓
4. 返回 Token
```

### 2. WebSocket 连接管理

```
[连接池初始化]
    ↓
WSPool {
    factory: createWSConnection,
    minSize: 5,
    maxSize: 20,
    connections: []
}
    ↓
[Get 连接]
    ↓
1. 检查空闲连接
    ├── 有空闲 → 健康检查 → 返回
    └── 无空闲 → 继续
    ↓
2. 检查连接数
    ├── < maxSize → 创建新连接
    └── >= maxSize → 等待或超时
    ↓
3. 创建新连接
    ├── 获取 Token
    ├── 建立 WebSocket
    ├── 启动心跳
    └── 返回连接
    ↓
[Put 连接]
    ↓
1. 检查连接健康
    ├── 健康 → 放回池中
    └── 不健康 → 关闭连接
```

### 3. 文件系统索引

```
[RefreshFSIndex 后台任务]
    ↓
1. 每 60 秒执行一次
    ↓
2. 扫描工作目录
    ├── 递归遍历文件
    ├── 过滤 .git, node_modules 等
    └── 收集文件路径
    ↓
3. 构建索引
    ├── 文件列表 (fsFileList)
    └── 目录映射 (fsIndex)
    ↓
4. 更新缓存
    └── 用于快速文件查找
```

### 4. 工具调用处理

```
[检测到 tool_use 块]
    ↓
1. 提取工具信息
    ├── tool_name
    ├── tool_input
    └── tool_use_id
    ↓
2. 检查工具是否允许
    ├── 检查阻塞列表
    └── 检查安全限制
    ↓
3. 执行工具
    ├── Bash - 执行命令
    ├── Read - 读取文件
    ├── Write - 写入文件
    ├── Edit - 编辑文件
    ├── Glob - 文件匹配
    └── Grep - 内容搜索
    ↓
4. 构建工具结果
    ├── tool_result 块
    ├── content: 执行结果
    └── is_error: 错误标志
    ↓
5. 发送回上游
    └── 继续对话
```

### 5. 摘要压缩流程

```
[Token 超限检测]
    ↓
1. 计算总 Token 数
    ├── System Prompt
    ├── 消息历史
    └── 工具定义
    ↓
2. 检查是否超过限制
    ├── < 限制 → 直接发送
    └── >= 限制 → 压缩
    ↓
3. 生成摘要 Key
    ├── Hash(conversation_id + messages)
    └── 检查缓存
    ↓
4. 压缩消息
    ├── 保留最近 N 条消息
    ├── 压缩旧消息为摘要
    └── 调用 AI 生成摘要
    ↓
5. 缓存摘要
    ├── 存储到 Redis/Memory
    └── TTL: 24 小时
    ↓
6. 构建新请求
    ├── System: 原始 + 摘要
    └── Messages: 最近消息
```

---

## 配置说明

### 关键配置项

```yaml
# 服务器配置
Port: "8080"
AdminPath: "/admin"
AdminUser: "admin"
AdminPass: "password"

# 上游配置
UpstreamMode: "ws"  # ws/http
UpstreamURL: "https://orchids-server.../agent/coding-agent"
RequestTimeout: 120  # 秒

# 账户管理
AutoRefreshToken: true
TokenRefreshInterval: 1  # 分钟

# 负载均衡
LoadBalancerCacheTTL: 60  # 秒
Retry429Interval: 5  # 分钟

# 并发控制
ConcurrencyLimit: 100
ConcurrencyTimeout: 300  # 秒
AdaptiveTimeout: true

# 缓存配置
CacheTTL: 5  # Token 缓存 TTL (分钟)
SummaryCacheMode: "redis"  # off/memory/redis
SummaryCacheSize: 1000
SummaryCacheTTLSeconds: 86400  # 24 小时

# Redis 配置
RedisAddr: "localhost:6379"
RedisPassword: ""
RedisDB: 0
RedisPrefix: "orchids:"

# 工作目录
LocalWorkdir: "/path/to/project"

# 调试
DebugEnabled: false
```

---

## 性能优化

### 1. 连接复用
- WebSocket 连接池 (5-20 连接)
- HTTP Keep-Alive
- 减少握手开销

### 2. Token 缓存
- 5 分钟 TTL
- 减少 Clerk API 调用
- 并发安全 (sync.RWMutex)

### 3. 摘要缓存
- Redis/Memory 双模式
- 24 小时 TTL
- 减少 Token 使用

### 4. 文件系统缓存
- 60 秒 TTL
- 预索引文件列表
- 快速文件查找

### 5. 并发控制
- 限制并发请求数
- 自适应超时
- 防止资源耗尽

---

## 安全机制

### 1. 认证
- API Key 验证
- Session Token
- Admin 密码保护

### 2. 工具限制
- 阻塞危险工具
- 路径遍历防护
- 命令注入防护

### 3. 速率限制
- 429 冷却机制
- 账户轮换
- 请求去重

### 4. 错误处理
- 敏感信息过滤
- 错误分类
- 优雅降级

---

## 监控与日志

### 1. Prometheus 指标
- 请求计数
- 响应时间
- 错误率
- 缓存命中率

### 2. 结构化日志
- JSON 格式
- 请求追踪
- 性能指标
- 错误堆栈

### 3. 调试工具
- pprof 性能分析
- 请求追踪日志
- WebSocket 消息日志

---

## 故障排查

### 常见问题

1. **Token 获取失败**
   - 检查 SessionID/ClientCookie
   - 验证 Clerk API 可达性
   - 查看 Token 缓存状态

2. **WebSocket 连接失败**
   - 检查网络连接
   - 验证 Token 有效性
   - 查看连接池状态

3. **文件系统操作失败**
   - 检查工作目录权限
   - 验证路径安全性
   - 查看操作日志

4. **429 错误频繁**
   - 增加账户数量
   - 调整冷却时间
   - 检查请求频率

5. **响应超时**
   - 增加 RequestTimeout
   - 检查上游服务状态
   - 启用自适应超时

---

## 总结

Orchids API 是一个功能完整的 AI 代理服务器，具有以下特点：

✅ **高性能**: 连接池、缓存、并发控制
✅ **高可用**: 负载均衡、自动重试、账户轮换
✅ **易扩展**: 模块化设计、插件化工具
✅ **易监控**: Prometheus 指标、结构化日志
✅ **易维护**: Web UI、API 管理、配置热更新

主要流程总结：
1. 客户端发送请求 → Handler 接收
2. 负载均衡选择账户 → 构建上游请求
3. WebSocket 发送请求 → 接收流式响应
4. 处理文件系统操作 → 执行工具调用
5. 转换响应格式 → 流式返回客户端
6. 错误处理与重试 → 记录日志与指标
