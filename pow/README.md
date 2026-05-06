# DeepSeek PoW 纯算实现

当前服务端 PoW 已走纯 Go 实现：`internal/deepseek/pow.go` 负责从上游 challenge map 中取字段，调用 `ds2api/pow` 求解 nonce，并组装 `x-ds-pow-response` header。

## 算法

DeepSeekHashV1 = SHA3-256 但 **Keccak-f[1600] 跳过 round 0** (只做 rounds 1..23)。其余参数不变:
rate=136, padding=0x06+0x80, output=32 字节。

PoW 协议:服务端选 answer ∈ [0, difficulty),计算 `challenge = hash(prefix + str(answer))`。
客户端遍历 [0, difficulty) 找到匹配的 nonce。

```
prefix = salt + "_" + str(expire_at) + "_"
input  = (prefix + str(nonce)).encode("utf-8")
hash   = DeepSeekHashV1(input)      → 32 bytes
header = base64(json({algorithm, challenge, salt, answer, signature, target_path}))
```

## 主要入口

- `pow/deepseek_hash.go`：DeepSeekHashV1 / Keccak-f[1600] rounds 1..23。
- `pow/deepseek_pow.go`：`SolvePow`、`BuildPowHeader`、`SolveAndBuildHeader`。
- `internal/deepseek/client/pow.go`：服务侧适配层，校验 `algorithm == DeepSeekHashV1` 并调用 `pow.SolvePow`。
- `internal/deepseek/client/pow_runtime.go`：运行时 CPU 保护层，限制本地 PoW 并发；上传目标会短时间缓存并合并同账号同目标的并发求解，生成目标只限流不复用。

## 运行时保护

- `runtime.pow_max_concurrency` 控制本地 PoW 求解并发；未配置时默认约为 `GOMAXPROCS/2`，上限 4。
- `DS2API_POW_MAX_CONCURRENCY` 可通过环境变量覆盖默认值。
- 上传文件目标的 PoW 响应缓存有效期很短，遇到上游 `INVALID_POW_RESPONSE` / `40301` 会立即失效，避免重复上传上下文文件时反复烧 CPU。
- 聊天生成目标仍每次请求获取并计算自己的 PoW，只经过并发阀门，避免复用导致生成链路被上游拒绝。

## 测试

```bash
cd pow && go test -v ./... && go test -bench=. -benchmem
```
