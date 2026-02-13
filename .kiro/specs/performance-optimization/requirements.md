# 需求文档

## 简介

本文档定义了 AI API 代理服务的全面性能优化需求。优化涵盖后端 Go 服务的内存管理、并发控制、数据库操作，以及前端管理界面的渲染性能。目标是显著降低内存分配、减少锁竞争、提升请求响应速度和用户界面流畅度。

## 术语表

- **Object_Pool**: 对象池，用于复用预分配的对象实例，减少 GC 压力
- **Sharded_Map**: 分片 Map，将单一 Map 分割为多个独立分片，每个分片有独立锁
- **LRU_Cache**: 最近最少使用缓存，优先驱逐最久未访问的条目
- **Pipeline**: Redis 管道操作，批量发送命令减少网络往返
- **Connection_Pool**: HTTP 连接池，复用 TCP 连接减少握手开销
- **DocumentFragment**: DOM 文档片段，用于批量 DOM 操作减少重排

## 需求

### 需求 1：对象池扩展

**用户故事：** 作为系统运维人员，我希望减少内存分配和 GC 压力，以便服务在高负载下保持稳定性能。

#### 验收标准

1. WHEN 系统初始化 Object_Pool THEN Object_Pool SHALL 将 MapPool 初始容量从 16 增加到 64
2. WHEN 代码需要复用 []interface{} 切片 THEN Object_Pool SHALL 提供 SlicePool 支持 []interface{} 类型
3. WHEN 代码需要复用 []map[string]interface{} 切片 THEN Object_Pool SHALL 提供 MapSlicePool 支持该类型
4. WHEN 处理大型响应数据 THEN Object_Pool SHALL 提供 LargeByteBufferPool 支持 16KB 大缓冲区
5. WHEN 对象归还到池中 THEN Object_Pool SHALL 正确重置对象状态防止数据泄露

### 需求 2：会话 Map 分片

**用户故事：** 作为系统运维人员，我希望减少高并发下的锁竞争，以便提升请求吞吐量。

#### 验收标准

1. WHEN 系统初始化 Sharded_Map THEN Sharded_Map SHALL 将 sessionWorkdirs 分片为 16 个独立 Map
2. WHEN 系统初始化 Sharded_Map THEN Sharded_Map SHALL 将 sessionConvIDs 分片为 16 个独立 Map
3. WHEN 系统初始化 Sharded_Map THEN Sharded_Map SHALL 将 sessionLastAccess 分片为 16 个独立 Map
4. WHEN 访问分片数据 THEN Sharded_Map SHALL 使用一致性哈希算法选择分片
5. WHEN 并发访问不同分片 THEN Sharded_Map SHALL 允许并行操作不产生锁竞争

### 需求 3：异步重复请求清理

**用户故事：** 作为系统运维人员，我希望减少每个请求的处理开销，以便降低请求延迟。

#### 验收标准

1. WHEN 系统启动 THEN 系统 SHALL 启动后台 goroutine 定时清理过期请求记录
2. WHEN 清理任务执行 THEN 系统 SHALL 每 5 秒执行一次清理操作
3. WHEN 请求到达 THEN 系统 SHALL 不再在请求路径上执行 O(n) 清理操作
4. WHEN 系统关闭 THEN 系统 SHALL 优雅停止清理 goroutine

### 需求 4：Redis 批量操作优化

**用户故事：** 作为系统运维人员，我希望减少 Redis 往返次数，以便提升数据库操作效率。

#### 验收标准

1. WHEN 批量获取账号数据 THEN Redis_Store SHALL 将并行处理阈值从 8 提高到 32
2. WHEN 执行多个独立 Redis 命令 THEN Redis_Store SHALL 使用 Pipeline 批量发送
3. WHEN Pipeline 执行失败 THEN Redis_Store SHALL 回退到单命令模式确保可靠性

### 需求 5：HTTP 连接复用优化

**用户故事：** 作为系统运维人员，我希望减少网络连接开销，以便降低上游 API 调用延迟。

#### 验收标准

1. WHEN 创建 HTTP 客户端 THEN Connection_Pool SHALL 配置 MaxIdleConns 为 100
2. WHEN 创建 HTTP 客户端 THEN Connection_Pool SHALL 配置 MaxIdleConnsPerHost 为 20
3. WHEN 创建 HTTP 客户端 THEN Connection_Pool SHALL 配置 IdleConnTimeout 为 90 秒
4. WHEN 构建请求头 THEN Connection_Pool SHALL 预分配 http.Header 避免运行时分配

### 需求 6：Token 缓存 LRU 优化

**用户故事：** 作为系统运维人员，我希望提升缓存命中率，以便减少重复计算开销。

#### 验收标准

1. WHEN 缓存达到容量上限 THEN LRU_Cache SHALL 驱逐最近最少使用的条目而非最早插入的条目
2. WHEN 访问缓存条目 THEN LRU_Cache SHALL 更新该条目的访问时间戳
3. WHEN 后台清理执行 THEN LRU_Cache SHALL 每 30 秒执行一次而非每次访问时执行

### 需求 7：前端 CSS 性能优化

**用户故事：** 作为管理员用户，我希望页面滚动流畅，以便获得更好的使用体验。

#### 验收标准

1. WHEN 渲染页面背景 THEN 前端 SHALL 移除 background-attachment: fixed 属性
2. WHEN 渲染页面背景 THEN 前端 SHALL 简化背景渐变为单层或两层
3. WHEN 用户滚动页面 THEN 前端 SHALL 保持 60fps 滚动帧率

### 需求 8：前端 JS 渲染性能优化

**用户故事：** 作为管理员用户，我希望账号列表加载快速，以便高效管理大量账号。

#### 验收标准

1. WHEN 渲染账号列表 THEN 前端 SHALL 使用 DocumentFragment 批量构建 DOM 元素
2. WHEN 多次查询相同 DOM 元素 THEN 前端 SHALL 缓存 DOM 查询结果避免重复查询
3. WHEN 更新表格内容 THEN 前端 SHALL 最小化 DOM 操作次数减少重排
