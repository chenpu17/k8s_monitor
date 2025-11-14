#!/bin/bash
# Integration test script for k8s-monitor
# Tests the application against a real Kubernetes cluster

set -e

echo "=================================="
echo "k8s-monitor Integration Test"
echo "=================================="
echo ""

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Test counters
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

# Helper functions
pass() {
    echo -e "${GREEN}✓ PASS${NC}: $1"
    ((PASSED_TESTS++))
    ((TOTAL_TESTS++))
}

fail() {
    echo -e "${RED}✗ FAIL${NC}: $1"
    ((FAILED_TESTS++))
    ((TOTAL_TESTS++))
}

test_header() {
    echo ""
    echo -e "${YELLOW}Test: $1${NC}"
    echo "---"
}

# Check prerequisites
echo "Checking prerequisites..."

if ! command -v kubectl &> /dev/null; then
    echo "ERROR: kubectl is not installed"
    exit 1
fi

if ! kubectl cluster-info &> /dev/null; then
    echo "ERROR: Cannot connect to Kubernetes cluster"
    exit 1
fi

if [ ! -f "./bin/k8s-monitor" ]; then
    echo "ERROR: k8s-monitor binary not found. Run 'make build' first."
    exit 1
fi

echo "✓ kubectl found"
echo "✓ Cluster accessible"
echo "✓ k8s-monitor binary found"
echo ""

# Get cluster info
CLUSTER_INFO=$(kubectl cluster-info 2>&1)
NODE_COUNT=$(kubectl get nodes --no-headers 2>/dev/null | wc -l)
POD_COUNT=$(kubectl get pods --all-namespaces --no-headers 2>/dev/null | wc -l)

echo "Cluster Information:"
echo "  Nodes: $NODE_COUNT"
echo "  Pods: $POD_COUNT"
echo ""

# Test 1: Binary execution
test_header "Binary Execution"
if ./bin/k8s-monitor --version &> /dev/null; then
    VERSION=$(./bin/k8s-monitor --version)
    pass "Binary executes successfully ($VERSION)"
else
    fail "Binary fails to execute"
fi

# Test 2: Help command
test_header "Help Command"
if ./bin/k8s-monitor --help &> /dev/null; then
    pass "Help command works"
else
    fail "Help command fails"
fi

# Test 3: Console help
test_header "Console Help Command"
if ./bin/k8s-monitor console --help &> /dev/null; then
    pass "Console help works"
else
    fail "Console help fails"
fi

# Test 4: API Server connectivity (via Go code)
test_header "API Server Connectivity"
cat > /tmp/test_api_connection.go << 'EOF'
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func main() {
	var kubeconfig string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = filepath.Join(home, ".kube", "config")
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error building config: %v\n", err)
		os.Exit(1)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating clientset: %v\n", err)
		os.Exit(1)
	}

	nodes, err := clientset.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing nodes: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully connected. Found %d nodes\n", len(nodes.Items))
}
EOF

cd /tmp
if go run test_api_connection.go 2>&1 | grep -q "Successfully connected"; then
    RESULT=$(go run test_api_connection.go 2>&1)
    pass "API Server connection successful ($RESULT)"
else
    fail "API Server connection failed"
fi
cd - > /dev/null
rm -f /tmp/test_api_connection.go

# Test 5: Data source initialization
test_header "Data Source Initialization"
cat > /tmp/test_datasource.go << 'EOF'
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/yourusername/k8s-monitor/internal/datasource"
	"k8s.io/client-go/util/homedir"
)

func main() {
	var kubeconfig string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = filepath.Join(home, ".kube", "config")
	}

	apiClient, err := datasource.NewAPIServerClient(kubeconfig, "", "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating API client: %v\n", err)
		os.Exit(1)
	}

	nodes, err := apiClient.GetNodes()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting nodes: %v\n", err)
		os.Exit(1)
	}

	pods, err := apiClient.GetPods("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting pods: %v\n", err)
		os.Exit(1)
	}

	events, err := apiClient.GetEvents("", "", 5)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting events: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("OK: Nodes=%d, Pods=%d, Events=%d\n", len(nodes), len(pods), len(events))
}
EOF

cd /tmp
if go run test_datasource.go 2>&1 | grep -q "OK:"; then
    RESULT=$(go run test_datasource.go 2>&1 | grep "OK:")
    pass "Data source works ($RESULT)"
else
    RESULT=$(go run test_datasource.go 2>&1)
    fail "Data source failed: $RESULT"
fi
cd - > /dev/null
rm -f /tmp/test_datasource.go

# Test 6: Aggregated data source
test_header "Aggregated Data Source"
cat > /tmp/test_aggregated.go << 'EOF'
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/yourusername/k8s-monitor/internal/datasource"
	"k8s.io/client-go/util/homedir"
)

func main() {
	var kubeconfig string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = filepath.Join(home, ".kube", "config")
	}

	apiClient, err := datasource.NewAPIServerClient(kubeconfig, "", "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating API client: %v\n", err)
		os.Exit(1)
	}

	kubeletClient, err := datasource.NewKubeletClient(apiClient.GetConfig(), true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating kubelet client: %v\n", err)
		os.Exit(1)
	}

	aggSource := datasource.NewAggregatedDataSource(apiClient, kubeletClient)
	clusterData, err := aggSource.GetClusterData()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting cluster data: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("OK: Nodes=%d, Pods=%d, Events=%d, Summary: Nodes=%d/%d Ready\n",
		len(clusterData.Nodes),
		len(clusterData.Pods),
		len(clusterData.Events),
		clusterData.Summary.NodesReady,
		clusterData.Summary.NodesTotal)
}
EOF

cd /tmp
if go run test_aggregated.go 2>&1 | grep -q "OK:"; then
    RESULT=$(go run test_aggregated.go 2>&1 | grep "OK:")
    pass "Aggregated data source works ($RESULT)"
else
    RESULT=$(go run test_aggregated.go 2>&1)
    fail "Aggregated data source failed: $RESULT"
fi
cd - > /dev/null
rm -f /tmp/test_aggregated.go

# Test 7: Cache functionality
test_header "Cache Functionality"
if go test ./internal/cache -v 2>&1 | grep -q "PASS"; then
    pass "Cache tests pass"
else
    fail "Cache tests fail"
fi

# Test 8: Data model conversion
test_header "Data Model Conversion"
if go test ./internal/datasource -v 2>&1 | grep -q "PASS"; then
    pass "Data model tests pass"
else
    fail "Data model tests fail"
fi

# Test 9: Node data accuracy
test_header "Node Data Accuracy"
KUBECTL_NODES=$(kubectl get nodes --no-headers | wc -l)
GO_TEST_OUTPUT=$(cd /tmp && go run test_datasource.go 2>&1)
GO_NODES=$(echo "$GO_TEST_OUTPUT" | grep -oP 'Nodes=\K\d+')

if [ "$KUBECTL_NODES" -eq "$GO_NODES" ]; then
    pass "Node count matches (kubectl: $KUBECTL_NODES, app: $GO_NODES)"
else
    fail "Node count mismatch (kubectl: $KUBECTL_NODES, app: $GO_NODES)"
fi

# Test 10: Pod data accuracy
test_header "Pod Data Accuracy"
KUBECTL_PODS=$(kubectl get pods --all-namespaces --no-headers | wc -l)
GO_PODS=$(echo "$GO_TEST_OUTPUT" | grep -oP 'Pods=\K\d+')

# Allow for slight differences due to timing
DIFF=$((KUBECTL_PODS - GO_PODS))
if [ ${DIFF#-} -le 5 ]; then
    pass "Pod count roughly matches (kubectl: $KUBECTL_PODS, app: $GO_PODS, diff: $DIFF)"
else
    fail "Pod count significant mismatch (kubectl: $KUBECTL_PODS, app: $GO_PODS, diff: $DIFF)"
fi

# Print summary
echo ""
echo "=================================="
echo "Test Summary"
echo "=================================="
echo "Total Tests: $TOTAL_TESTS"
echo -e "${GREEN}Passed: $PASSED_TESTS${NC}"
if [ $FAILED_TESTS -gt 0 ]; then
    echo -e "${RED}Failed: $FAILED_TESTS${NC}"
else
    echo "Failed: 0"
fi
echo ""

if [ $FAILED_TESTS -eq 0 ]; then
    echo -e "${GREEN}✓ All tests passed!${NC}"
    exit 0
else
    echo -e "${RED}✗ Some tests failed${NC}"
    exit 1
fi
