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

- `pow/deepseek_hash.go`：DeepSeekHashV1。
- `pow/keccak_generic.go`：keccakF23 纯 Go 实现（23 轮 Keccak-f[1600]，跳过 round 0）。
- `pow/keccak_amd64.s`：预留 amd64 SIMD 汇编（AVX-512 路径仍在开发中）。
- `pow/deepseek_pow.go`：`SolvePow`、`BuildPowHeader`、`SolveAndBuildHeader`。
- `internal/deepseek/client/pow.go`：服务侧适配层，校验 `algorithm == DeepSeekHashV1` 并调用 `pow.SolvePow`。
- `internal/deepseek/client/pow_runtime.go`：运行时 CPU 保护层，限制本地 PoW 并发；上传目标会短时间缓存并合并同账号同目标的并发求解。

## 运行时保护

- `runtime.pow_max_concurrency` 控制外部 PoW 求解并发；未配置时默认约为 `GOMAXPROCS/2`，上限 4。
- `DS2API_POW_MAX_CONCURRENCY` 可通过环境变量覆盖默认值。
- 上传文件目标的 PoW 响应缓存有效期很短，遇到上游 `INVALID_POW_RESPONSE` / `40301` 会立即失效。
- 聊天生成目标仍每次请求获取并计算自己的 PoW，只经过并发阀门。

## 多核并行

`SolvePow` 默认使用所有可用 CPU 核心并行搜索 nonce。

- **自动**：默认使用 `GOMAXPROCS` 个 worker 并行搜索。虚拟机自动适配 vCPU 数量。
- **手动**：设置 `DS2API_POW_INTERNAL_PARALLEL` 环境变量：
  - `0` 或 `1` → 禁用并行（串行搜索）
  - `N` → 固定 N 个 worker
  - `-1` → 显式使用全部核心
- **信号量联动**：当内部并行 ≥ 2 时，外部并发信号量自动降为 1，防止 M×N 过度订阅。

## 指令集优化

keccakF23 在以下平台有专用实现：

| 平台 | 实现 | 状态 |
|------|------|------|
| amd64 (通用) | 纯 Go（编译器展开 + 寄存器分配） | 已稳定 |
| amd64 AVX-512 (Zen 4/5) | SIMD 汇编（VPROLQ + VPTERNLOG） | 开发中 |
| arm64 NEON | SIMD 汇编（BIC + ROR） | 开发中 |

## 测试

```bash
cd pow && go test -v ./... && go test -bench=. -benchmem
```
