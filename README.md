# dnspyre

一个高性能的DNS基准测试工具，支持多种DNS协议和可视化分析。

## 功能特性

- **多协议支持**: 支持传统DNS (UDP/TCP)、DNS-over-TLS (DoT)、DNS-over-HTTPS (DoH)、DNS-over-QUIC (DoQ)
- **批量测试**: 同时测试多个DNS服务器并生成对比结果
- **Web可视化**: 内置Web界面用于结果分析和图表展示
- **地理位置**: 自动检测DNS服务器地理位置信息
- **多种输出**: 支持JSON、CSV、HTML、图表等多种输出格式
- **高并发**: 支持大量并发查询测试
- **实时监控**: 支持Prometheus指标监控

## 安装使用

### 基本DNS测试

测试单个DNS服务器：

```bash
./dnspyre -s 8.8.8.8 -c 10 -n 100 google.com
```

### 批量测试多个DNS服务器

生成多服务器对比JSON结果：

```bash
./dnspyre --batch-json "8.8.8.8,1.1.1.1,114.114.114.114" -n 100 google.com > results.json
```

### Web前端界面

启动Web界面查看测试结果：

```bash
# 启动Web服务器
./dnspyre frontend --port 8080

# 预加载JSON数据文件启动
./dnspyre frontend --file results.json --port 8080
```

访问 <http://localhost:8080> 查看可视化结果。

## 主要参数说明

### 基础参数

- `-s, --server`: DNS服务器地址，默认 127.0.0.1
- `-c, --concurrency`: 并发查询数量，默认 1
- `-n, --number`: 重复查询次数
- `-t, --type`: DNS查询类型 (A, AAAA, CNAME等)
- `-d, --duration`: 测试持续时间 (如: 30s, 5m, 1h)

### 协议选项

- **传统DNS**: `8.8.8.8:53` 或 `8.8.8.8`
- **DNS-over-TLS**: `8.8.8.8:853`
- **DNS-over-HTTPS**: `https://8.8.8.8/dns-query`
- **DNS-over-QUIC**: `quic://8.8.8.8:853`

### 输出格式

- `--json`: 输出JSON格式结果
- `--csv`: 输出CSV格式结果
- `--html`: 生成HTML报告
- `--plot`: 生成图表文件
- `--batch-json`: 批量测试多服务器JSON输出

### 前端选项

- `--port`: Web服务端口，默认 8080
- `--host`: 绑定主机地址，默认 localhost
- `--file`: 预加载JSON数据文件
- `--open`: 自动打开浏览器，默认 true

## 使用示例

### 1. 基础性能测试

```bash
# 测试Google DNS，100个并发，每个发送50次查询
./dnspyre -s 8.8.8.8 -c 100 -n 50 google.com baidu.com
```

### 2. DoH协议测试

```bash
# 测试Cloudflare DoH服务
./dnspyre -s https://1.1.1.1/dns-query -c 10 -n 100 google.com
```

### 3. 批量对比测试

```bash
# 同时测试多个知名DNS服务商
./dnspyre --batch-json "8.8.8.8,1.1.1.1,114.114.114.114,223.5.5.5" \
  -c 20 -n 100 google.com baidu.com > dns_comparison.json
```

### 4. 持续时间测试

```bash
# 持续测试5分钟
./dnspyre -s 8.8.8.8 -c 50 -d 5m google.com
```

### 5. 启动Web界面分析结果

```bash
# 启动Web服务并预加载测试结果
./dnspyre frontend --file dns_comparison.json --port 9999

# 或者先启动服务，再手动上传JSON文件
./dnspyre frontend --port 8080
```

### 6. 生成可视化图表

```bash
# 生成PNG格式图表到指定目录
./dnspyre -s 8.8.8.8 -c 10 -n 100 --plot ./charts --plotf png google.com
```

### 7. 导出多种格式报告

```bash
# 同时生成JSON、CSV和HTML报告
./dnspyre -s 8.8.8.8 -c 10 -n 100 \
  --json \
  --csv results.csv \
  --html report.html \
  google.com
```

## 数据源

查询数据可以来自多种源：

- 直接指定域名: `google.com baidu.com`
- 本地文件: `@data/1000-domains`
- 远程URL: `https://example.com/domains.txt`
- 组合使用: `google.com @data/domains https://example.com/list.txt`

## 高级功能

### 限速控制

```bash
# 全局限速100 QPS
./dnspyre -s 8.8.8.8 -l 100 -c 10 -n 1000 google.com

# 每个并发工作者限速10 QPS
./dnspyre -s 8.8.8.8 --rate-limit-worker 10 -c 5 -n 500 google.com
```

### 请求延迟

```bash
# 每次请求前延迟500ms
./dnspyre -s 8.8.8.8 --request-delay 500ms -n 100 google.com

# 随机延迟1-3秒
./dnspyre -s 8.8.8.8 --request-delay 1s-3s -n 100 google.com
```

### 失败条件控制

```bash
# 有IO错误或DNS错误时退出
./dnspyre -s 8.8.8.8 --fail ioerror --fail error -n 100 google.com
```

### Prometheus监控

```bash
# 启用Prometheus指标端点
./dnspyre -s 8.8.8.8 --prometheus :9090 -c 10 -d 5m google.com
# 访问 http://localhost:9090/metrics 查看指标
```

通过这个工具，你可以全面测试和分析DNS服务器的性能表现，为网络优化提供数据支撑。
