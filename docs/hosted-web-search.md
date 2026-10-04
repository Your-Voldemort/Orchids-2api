# Responses 联网搜索

WorkBuddy、Qoder 和 Cline 的 Responses 桥接可使用 Bright Data SERP API。Grok 原生 Responses 仍使用其上游托管搜索。

配置服务器环境变量 `BRIGHTDATA_API_KEY` 和 `BRIGHTDATA_ZONE`，或设置 `BRIGHTDATA_CONFIG_FILE` 指向私有 JSON 文件：

```json
{"api_key":"YOUR_API_KEY","zone":"serp_api1"}
```

配置管理 → 联网搜索支持启用开关、SERP Zone 和 API Key。保存后新请求立即采用新配置；正在执行的请求继续使用原快照。API Key 不回显，留空保留，输入新值替换；关闭搜索保留凭据。

首次启动没有已保存搜索配置时，从环境变量或 `.tools/brightdata.local.json` 导入，该目录被 Git 忽略。管理保存的配置持久化到 Redis，重启后优先使用，不再被私有文件覆盖。配置读取 API 仅返回是否已配置密钥，审计与诊断脱敏密钥。

客户端声明 `web_search` 或 `web_search_preview` 时，网关先使用相同模型和渠道规划最多两个公开搜索关键词；仅将关键词和搜索参数发送到 `https://api.brightdata.com/request`。最终模型收到最多八条结果的标题、链接和摘要，回答应引用来源。规划模型调用单独预留及结算；搜索服务额度由 Bright Data 账户管理。

`tool_choice: none` 或指定客户端函数时不搜索；强制 web_search 必须产生查询。搜索失败返回明确错误，不伪造联网成功。JSON 和 SSE 均返回包含来源的 `web_search_call`，支持存储与续接。当前不支持域名过滤、地理位置限制、打开页面或页面内查找；不支持的搜索选项明确拒绝。

免费额度目前为每月 5,000 credits，与其他符合条件的 Bright Data 产品共享，以其账户套餐为准。

`web_search.external_web_access` 省略或为 `true` 时执行实时搜索；为 `false` 时只读取本进程已有结果缓存，不请求 Bright Data，缓存未命中返回空来源。缓存保留一小时、最多 256 条，按密钥与 Zone 隔离，重启后清空；这是网关缓存，不是 OpenAI 的离线索引。Preview 工具按 OpenAI 语义忽略该布尔开关，始终允许实时搜索。非布尔值拒绝。

默认测试使用本地模拟。真实接口测试需显式设置 `BRIGHTDATA_LIVE_TEST=1` 和私有配置路径后运行 `go test ./internal/grok -run TestBrightDataLiveSearch -v`，会消耗一次真实搜索请求。
