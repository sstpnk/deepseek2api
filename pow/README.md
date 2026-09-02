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
- `pow/keccak_generic.go`：keccakF23 纯 Go 实现（23 轮 Keccak-f[1600]，跳过 round 0）。所有平台的通用 fallback。
- `pow/keccak_amd64.s`：AVX-512 汇编实现（VPTERNLOGQ 加速 Chi 步）。
- `pow/keccak_arm64.s`：ARM64 NEON 汇编实现（BIC 加速 Chi 步）。
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
| amd64 (通用) | 纯 Go（编译器展开 + 寄存器分配） | 生产可用 |
| amd64 AVX-512F/VL | 手写汇编（VPTERNLOGQ 0xD2 Chi，1 指令/每 lane） | 生产可用，自动检测启用 |
| arm64 NEON | SIMD 汇编（BIC Chi） | 已实现，需 N1 实测 |

## AVX-512 调试

AVX-512 后端为实验特性，默认不启用。要测试并反馈问题：

```bash
# 启用 AVX-512 后端
export DS2API_POW_BACKEND=avx512

# 运行自测（会打印 expected/actual 哈希对比）
go test ./pow/... -v -run TestDeepSeekHashV1

# 如果自测失败，收集以下信息反馈：
# 1. 完整的测试输出（包括 expected_generic / actual_avx512 哈希值）
# 2. CPU 型号：lscpu | grep "Model name"
# 3. CPU flags：lscpu | grep Flags
# 4. Go 版本：go version
# 5. 操作系统：uname -a
```

## 测试

```bash
cd pow && go test -v ./... && go test -bench=. -benchmem
```

## 部署指南

### Docker Compose 快速部署

```bash
# 1. 拉取最新代码
cd /root/ds2api && git pull origin new

# 2. 构建镜像
docker compose build --no-cache

# 3. 重启（用 up -d --force-recreate 而非 down+up，避免 SSH 断开）
docker compose up -d --force-recreate

# 4. 验证
docker logs ds2api | grep PoW
# 期望输出: [PoW] solver ready backend=avx512 validated=true

curl http://localhost:6011/healthz
# 期望输出: {"status":"ok"}
```

### 环境变量速查

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `DS2API_POW_BACKEND` | `auto` | 强制后端：`auto`/`generic`/`avx512`/`neon` |
| `DS2API_POW_MODE` | `latency` | 并发模式：`latency`/`throughput`/`balanced` |
| `DS2API_POW_INTERNAL_PARALLEL` | `GOMAXPROCS` | 内部 worker 数 |
| `DS2API_POW_BACKEND_STRICT` | 关闭 | `1`=自测失败直接退出 |

### 性能调优建议

- **低延迟场景**：保持默认 `POW_MODE=latency`（单请求全核并行）
- **高并发场景**：`POW_MODE=throughput`（每请求串行，多请求并发）
- **通用场景**：`POW_MODE=balanced`（内部 4 worker，外部按配置）
- **AVX-512 机器**：自动检测启用，无需手动配置
