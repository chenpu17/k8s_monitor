//go:build ignore

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/yourusername/k8s-monitor/internal/datasource"
	"k8s.io/client-go/util/homedir"
)

func main() {
	var kubeconfig string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = filepath.Join(home, ".kube", "config")
	}

	fmt.Println("=== k8s-monitor Integration Test ===")
	fmt.Println("")

	// Test 1: API Server Client
	fmt.Println("Test 1: Creating API Server client...")
	apiClient, err := datasource.NewAPIServerClient(kubeconfig, "", "")
	if err != nil {
		fmt.Printf("❌ FAILED: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ PASSED: API Server client created")

	// Test 2: Get Nodes
	fmt.Println("\nTest 2: Fetching nodes...")
	startTime := time.Now()
	nodes, err := apiClient.GetNodes()
	if err != nil {
		fmt.Printf("❌ FAILED: %v\n", err)
		os.Exit(1)
	}
	nodeTime := time.Since(startTime)
	fmt.Printf("✅ PASSED: Retrieved %d nodes in %v\n", len(nodes), nodeTime)

	// Test 3: Get Pods
	fmt.Println("\nTest 3: Fetching pods...")
	startTime = time.Now()
	pods, err := apiClient.GetPods("")
	if err != nil {
		fmt.Printf("❌ FAILED: %v\n", err)
		os.Exit(1)
	}
	podTime := time.Since(startTime)
	fmt.Printf("✅ PASSED: Retrieved %d pods in %v\n", len(pods), podTime)

	// Test 4: Get Events
	fmt.Println("\nTest 4: Fetching events...")
	startTime = time.Now()
	events, err := apiClient.GetEvents("", "", 10)
	if err != nil {
		fmt.Printf("❌ FAILED: %v\n", err)
		os.Exit(1)
	}
	eventTime := time.Since(startTime)
	fmt.Printf("✅ PASSED: Retrieved %d events in %v\n", len(events), eventTime)

	// Test 5: Kubelet Client
	fmt.Println("\nTest 5: Creating kubelet client...")
	kubeletClient, err := datasource.NewKubeletClient(apiClient.GetConfig(), true)
	if err != nil {
		fmt.Printf("❌ FAILED: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ PASSED: Kubelet client created (proxy mode)")

	// Test 6: Aggregated Data Source
	fmt.Println("\nTest 6: Creating aggregated data source...")
	aggSource := datasource.NewAggregatedDataSource(apiClient, kubeletClient)
	fmt.Println("✅ PASSED: Aggregated data source created")

	// Test 7: Get Cluster Data
	fmt.Println("\nTest 7: Fetching complete cluster data...")
	startTime = time.Now()
	clusterData, err := aggSource.GetClusterData()
	if err != nil {
		fmt.Printf("❌ FAILED: %v\n", err)
		os.Exit(1)
	}
	clusterTime := time.Since(startTime)
	
	fmt.Printf("✅ PASSED: Retrieved complete cluster data in %v\n", clusterTime)
	fmt.Println("\nCluster Summary:")
	fmt.Printf("  Nodes: %d total, %d ready\n", clusterData.Summary.NodesTotal, clusterData.Summary.NodesReady)
	fmt.Printf("  Pods: %d total, %d running, %d pending, %d failed\n", 
		clusterData.Summary.PodsTotal,
		clusterData.Summary.PodsRunning,
		clusterData.Summary.PodsPending,
		clusterData.Summary.PodsFailed)
	
	// Test 8: Data Validation
	fmt.Println("\nTest 8: Validating data integrity...")
	if len(clusterData.Nodes) != len(nodes) {
		fmt.Printf("❌ FAILED: Node count mismatch (%d vs %d)\n", len(clusterData.Nodes), len(nodes))
		os.Exit(1)
	}
	if len(clusterData.Pods) != len(pods) {
		fmt.Printf("❌ FAILED: Pod count mismatch (%d vs %d)\n", len(clusterData.Pods), len(pods))
		os.Exit(1)
	}
	fmt.Println("✅ PASSED: Data integrity validated")

	// Summary
	fmt.Println("\n=== All Tests Passed! ===")
	fmt.Printf("Total time: %v\n", clusterTime)
	fmt.Printf("Performance: %.2f nodes/sec, %.2f pods/sec\n",
		float64(len(nodes))/nodeTime.Seconds(),
		float64(len(pods))/podTime.Seconds())
}
