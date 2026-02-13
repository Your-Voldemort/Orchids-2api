# 实现计划: 性能优化

## 概述

本计划将性能优化分为 6 个主要阶段，按优先级从高到低实现。每个阶段包含实现任务和对应的测试任务。

## 任务

- [ ] 1. 扩展对象池
  - [x] 1.1 修改 MapPool 初始容量为 64
    - 修改 `internal/perf/pools.go` 中 MapPool 的 New 函数
    - 将 `make(map[string]interface{}, 16)` 改为 `make(map[string]interface{}, 64)`
    - _需求: 1.1_

  - [x] 1.2 实现 SlicePool
    - 在 `internal/perf/pools.go` 中添加 SlicePool 变量
    - 实现 AcquireSlice 和 ReleaseSlice 函数
    - 初始容量 32，归还时清空切片
    - _需求: 1.2_

  - [x] 1.3 实现 MapSlicePool
    - 在 `internal/perf/pools.go` 中添加 MapSlicePool 变量
    - 实现 AcquireMapSlice 和 ReleaseMapSlice 函数
    - 初始容量 16，归还时清空切片
    - _需求: 1.3_

  - [x] 1.4 实现 LargeByteBufferPool
    - 在 `internal/perf/pools.go` 中添加 LargeByteBufferPool 变量
    - 实现 AcquireLargeByteBuffer 和 ReleaseLargeByteBuffer 函数
    - 缓冲区容量 16KB (16384 字节)
    - _需求: 1.4_

  - [x] 1.5 编写对象池属性测试
    - **Property 1: 对象池获取归还一致性**
    - **验证: 需求 1.2, 1.3, 1.5**

- [ ] 2. 实现分片 Map
  - [x] 2.1 创建 ShardedMap 泛型结构
    - 创建 `internal/handler/sharded_map.go` 文件
    - 实现 ShardedMap[V any] 结构，包含 16 个分片
    - 使用 FNV-1a 哈希算法选择分片
    - _需求: 2.1, 2.2, 2.3, 2.4_

  - [x] 2.2 实现 ShardedMap 方法
    - 实现 NewShardedMap、Get、Set、Delete、Range 方法
    - 每个分片使用独立的 RWMutex
    - _需求: 2.4, 2.5_

  - [x] 2.3 重构 Handler 使用 ShardedMap
    - 修改 `internal/handler/handler.go`
    - 将 sessionWorkdirs、sessionConvIDs、sessionLastAccess 替换为 ShardedMap
    - 更新所有访问这些 map 的代码
    - _需求: 2.1, 2.2, 2.3_

  - [x] 2.4 编写分片 Map 属性测试
    - **Property 2: 分片 Map 键映射一致性**
    - **Property 3: 分片 Map 并发安全性**
    - **验证: 需求 2.4, 2.5**

- [x] 3. 检查点 - 确保所有测试通过
  - 运行 `go test ./internal/perf/... ./internal/handler/...`
  - 如有问题请询问用户

- [ ] 4. 实现异步清理器
  - [x] 4.1 创建 AsyncCleaner 结构
    - 创建 `internal/handler/async_cleaner.go` 文件
    - 实现 AsyncCleaner 结构，包含 interval、stopCh、wg
    - 实现 NewAsyncCleaner、Start、Stop 方法
    - _需求: 3.1, 3.4_

  - [x] 4.2 重构 recentRequests 清理逻辑
    - 修改 `internal/handler/handler.go`
    - 移除 registerRequest 和 finishRequest 中的 cleanupRecentLocked 调用
    - 在 Handler 初始化时启动 AsyncCleaner
    - 清理间隔设为 5 秒
    - _需求: 3.2, 3.3_

  - [x] 4.3 编写异步清理器测试
    - **Property 5: 异步清理器生命周期**
    - **验证: 需求 3.1, 3.4**

- [ ] 5. Redis 批量操作优化
  - [x] 5.1 提高并行处理阈值
    - 修改 `internal/store/redis_store.go`
    - 将 parallelThreshold 从 8 改为 32
    - _需求: 4.1_

  - [x] 5.2 实现 Pipeline 批量获取
    - 在 getAccountsByIDs 中使用 Pipeline 替代 MGet
    - 添加错误处理，失败时回退到单命令模式
    - _需求: 4.2, 4.3_

- [ ] 6. HTTP 连接池优化
  - [x] 6.1 配置 Transport 连接池参数
    - 修改 `internal/grok/client.go` 中的 newHTTPClient 函数
    - 设置 MaxIdleConns=100, MaxIdleConnsPerHost=20, IdleConnTimeout=90s
    - _需求: 5.1, 5.2, 5.3_

  - [x] 6.2 预分配请求头模板
    - 创建 baseHeaders 变量存储固定请求头
    - 修改 headers 方法基于模板克隆并添加动态头
    - _需求: 5.4_

- [x] 7. 检查点 - 确保所有测试通过
  - 运行 `go test ./internal/...`
  - 如有问题请询问用户

- [ ] 8. LRU 缓存优化
  - [x] 8.1 添加访问时间戳字段
    - 修改 `internal/tokencache/memory.go` 中的 cacheItem 结构
    - 添加 accessedAt 字段
    - 在 Get 方法中更新 accessedAt
    - _需求: 6.2_

  - [x] 8.2 实现 LRU 驱逐策略
    - 修改 evictOldestLocked 为 evictLRULocked
    - 基于 accessedAt 而非 expiresAt 选择驱逐条目
    - _需求: 6.1_

  - [x] 8.3 优化后台清理频率
    - 修改 cleanupLoop 间隔为 30 秒
    - _需求: 6.3_

  - [x] 8.4 编写 LRU 缓存属性测试
    - **Property 4: LRU 驱逐正确性**
    - **验证: 需求 6.1, 6.2**

- [ ] 9. 前端 CSS 性能优化
  - [x] 9.1 移除 fixed 背景
    - 修改 `web/static/css/main.css`
    - 删除 body 的 `background-attachment: fixed` 属性
    - _需求: 7.1_

  - [x] 9.2 简化背景渐变
    - 将三层 radial-gradient 简化为单层
    - 使用 `radial-gradient(ellipse at 50% 0%, rgba(124, 92, 252, 0.06) 0%, transparent 60%)`
    - _需求: 7.2_

- [ ] 10. 前端 JS 渲染优化
  - [x] 10.1 实现 DOM 缓存
    - 修改 `web/static/js/accounts.js`
    - 创建 domCache 对象缓存常用 DOM 元素
    - 添加 initDOMCache 函数在页面加载时初始化
    - _需求: 8.2_

  - [x] 10.2 使用 DocumentFragment 批量渲染
    - 修改 renderAccounts 函数
    - 使用 DocumentFragment 构建表格行
    - 一次性插入到 DOM
    - _需求: 8.1, 8.3_

- [x] 11. 最终检查点 - 确保所有测试通过
  - 运行 `go test ./...`
  - 验证前端页面正常加载
  - 如有问题请询问用户

## 备注

- 标记 `*` 的任务为可选测试任务，可跳过以加快 MVP 进度
- 每个任务引用具体需求以便追溯
- 检查点确保增量验证
- 属性测试验证通用正确性属性
- 单元测试验证具体示例和边界情况
