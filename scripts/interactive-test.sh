#!/bin/bash
# k8s-monitor 交互式测试脚本

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}======================================${NC}"
echo -e "${BLUE}   k8s-monitor 交互式测试脚本${NC}"
echo -e "${BLUE}======================================${NC}"
echo ""

# 检查先决条件
echo -e "${YELLOW}[1/7] 检查先决条件...${NC}"

if [ ! -f "./bin/k8s-monitor" ]; then
    echo -e "${RED}错误: ./bin/k8s-monitor 不存在${NC}"
    echo "请先运行: make build"
    exit 1
fi
echo -e "${GREEN}✓ 二进制文件存在${NC}"

if ! kubectl cluster-info &> /dev/null; then
    echo -e "${YELLOW}⚠ kubectl 无法连接到集群${NC}"
    echo "某些测试可能会失败，但配置测试仍可运行"
else
    echo -e "${GREEN}✓ Kubernetes 集群可访问${NC}"
    NODES=$(kubectl get nodes --no-headers | wc -l)
    PODS=$(kubectl get pods --all-namespaces --no-headers | wc -l)
    echo -e "  集群规模: ${NODES} 节点, ${PODS} Pods"
fi

echo ""

# 测试1: 版本检查
echo -e "${YELLOW}[2/7] 测试版本命令...${NC}"
VERSION=$(./bin/k8s-monitor --version)
echo -e "${GREEN}✓ 版本: ${VERSION}${NC}"
echo ""

# 测试2: 帮助命令
echo -e "${YELLOW}[3/7] 测试帮助命令...${NC}"
if ./bin/k8s-monitor --help > /dev/null 2>&1; then
    echo -e "${GREEN}✓ --help 工作正常${NC}"
fi
if ./bin/k8s-monitor console --help > /dev/null 2>&1; then
    echo -e "${GREEN}✓ console --help 工作正常${NC}"
fi
echo ""

# 测试3: 创建测试配置
echo -e "${YELLOW}[4/7] 创建测试配置文件...${NC}"
TEST_CONFIG="/tmp/k8s-monitor-test-$(date +%s).yaml"
cat > "$TEST_CONFIG" <<'EOF'
cluster:
  kubeconfig: ""
  context: ""
  namespace: ""

refresh:
  interval: 5s
  timeout: 3s
  max_concurrent: 5

cache:
  ttl: 30s
  max_entries: 500

ui:
  color_mode: auto
  default_view: overview
  max_rows: 50

logging:
  level: debug
  file: /tmp/k8s-monitor-interactive-test.log
EOF

echo -e "${GREEN}✓ 配置文件创建: ${TEST_CONFIG}${NC}"
echo ""

# 测试4: 配置文件加载测试
echo -e "${YELLOW}[5/7] 测试配置文件加载...${NC}"
echo "启动应用 3 秒后自动退出..."

timeout 3 ./bin/k8s-monitor console --config "$TEST_CONFIG" 2>&1 | head -10 || true

if [ -f "/tmp/k8s-monitor-interactive-test.log" ]; then
    echo -e "${GREEN}✓ 日志文件已创建${NC}"

    # 验证配置值
    if grep -q "cache_ttl.*30" /tmp/k8s-monitor-interactive-test.log; then
        echo -e "${GREEN}✓ cache_ttl 正确加载 (30s)${NC}"
    fi

    if grep -q "max_concurrent.*5" /tmp/k8s-monitor-interactive-test.log; then
        echo -e "${GREEN}✓ max_concurrent 正确加载 (5)${NC}"
    fi

    if grep -q "log_level.*debug" /tmp/k8s-monitor-interactive-test.log; then
        echo -e "${GREEN}✓ log_level 正确加载 (debug)${NC}"
    fi
else
    echo -e "${YELLOW}⚠ 日志文件未创建 (可能因为没有 TTY)${NC}"
fi
echo ""

# 测试5: CLI 参数测试
echo -e "${YELLOW}[6/7] 测试 CLI 参数覆盖...${NC}"
echo "测试 --refresh 参数..."

# 清理旧日志
rm -f /tmp/k8s-monitor-interactive-test.log

timeout 3 ./bin/k8s-monitor console --config "$TEST_CONFIG" --refresh 20 --verbose 2>&1 | head -10 || true

if [ -f "/tmp/k8s-monitor-interactive-test.log" ]; then
    if grep -q "refresh_interval.*20" /tmp/k8s-monitor-interactive-test.log; then
        echo -e "${GREEN}✓ --refresh 参数覆盖成功 (20s)${NC}"
    elif grep -q "refresh_interval" /tmp/k8s-monitor-interactive-test.log; then
        ACTUAL=$(grep "refresh_interval" /tmp/k8s-monitor-interactive-test.log | head -1)
        echo -e "${YELLOW}⚠ refresh_interval 值: ${ACTUAL}${NC}"
    fi
fi
echo ""

# 测试6: 查看日志
echo -e "${YELLOW}[7/7] 查看应用日志...${NC}"
if [ -f "/tmp/k8s-monitor-interactive-test.log" ]; then
    echo "日志文件最后 15 行:"
    echo "----------------------------------------"
    tail -15 /tmp/k8s-monitor-interactive-test.log | while IFS= read -r line; do
        echo -e "${BLUE}${line}${NC}"
    done
    echo "----------------------------------------"
    echo ""
    echo -e "${GREEN}✓ 完整日志文件: /tmp/k8s-monitor-interactive-test.log${NC}"
else
    echo -e "${YELLOW}⚠ 日志文件不存在${NC}"
fi
echo ""

# 总结
echo -e "${BLUE}======================================${NC}"
echo -e "${BLUE}        测试完成${NC}"
echo -e "${BLUE}======================================${NC}"
echo ""
echo "下一步操作:"
echo ""
echo "1. 手动运行应用 (需要 TTY):"
echo -e "   ${GREEN}./bin/k8s-monitor console${NC}"
echo ""
echo "2. 使用自定义配置:"
echo -e "   ${GREEN}./bin/k8s-monitor console --config ${TEST_CONFIG}${NC}"
echo ""
echo "3. 测试不同参数:"
echo -e "   ${GREEN}./bin/k8s-monitor console --refresh 30 --verbose${NC}"
echo ""
echo "4. 指定 namespace:"
echo -e "   ${GREEN}./bin/k8s-monitor console -n kube-system${NC}"
echo ""
echo "5. 查看完整日志:"
echo -e "   ${GREEN}tail -f /tmp/k8s-monitor-interactive-test.log${NC}"
echo ""
echo "6. 清理测试文件:"
echo -e "   ${GREEN}rm -f ${TEST_CONFIG} /tmp/k8s-monitor-interactive-test.log${NC}"
echo ""

# 询问是否启动交互模式
echo -e "${YELLOW}是否要立即启动 k8s-monitor? (需要 TTY) [y/N]${NC}"
read -t 10 -r response || response="n"

if [[ "$response" =~ ^[Yy]$ ]]; then
    echo ""
    echo -e "${GREEN}启动 k8s-monitor (按 q 或 Ctrl+C 退出)...${NC}"
    echo ""
    sleep 2
    ./bin/k8s-monitor console --config "$TEST_CONFIG"
else
    echo ""
    echo -e "${BLUE}跳过交互启动${NC}"
    echo -e "手动启动: ${GREEN}./bin/k8s-monitor console${NC}"
fi

echo ""
echo -e "${GREEN}测试脚本执行完毕!${NC}"
