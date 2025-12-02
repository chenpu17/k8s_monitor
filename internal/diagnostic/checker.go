package diagnostic

import (
	"github.com/yourusername/k8s-monitor/internal/model"
)

// GetRecommendedAction returns the recommended action for a specific alert type
func GetRecommendedAction(alertType model.AlertType, namespace, resourceName string) string {
	switch alertType {
	// Node alerts
	case model.AlertTypeNodeNotReady:
		return "kubectl describe node " + resourceName + " # Check node conditions and events"
	case model.AlertTypeNodeMemoryPressure:
		return "kubectl top pods --all-namespaces --sort-by=memory # Find memory-intensive pods"
	case model.AlertTypeNodeDiskPressure:
		return "kubectl describe node " + resourceName + " # Check disk usage, consider cleanup or expansion"
	case model.AlertTypeNodePIDPressure:
		return "kubectl top pods -A | Sort by running processes, check for process leaks"
	case model.AlertTypeNodeCPUCritical, model.AlertTypeNodeCPUHigh:
		return "kubectl top pods --all-namespaces --sort-by=cpu # Find CPU-intensive pods"
	case model.AlertTypeNodeMemoryCritical, model.AlertTypeNodeMemoryHigh:
		return "kubectl top pods --all-namespaces --sort-by=memory # Find memory-intensive pods"

	// Pod alerts
	case model.AlertTypePodOOMKilled:
		if namespace != "" {
			return "kubectl logs -n " + namespace + " " + resourceName + " --previous # Check logs before OOM"
		}
		return "kubectl logs " + resourceName + " --previous # Check logs before OOM, consider increasing memory limits"
	case model.AlertTypePodCrashLoopBackOff:
		if namespace != "" {
			return "kubectl logs -n " + namespace + " " + resourceName + " --previous # Check crash logs"
		}
		return "kubectl logs " + resourceName + " --previous # Check crash logs"
	case model.AlertTypePodImagePullBackOff:
		if namespace != "" {
			return "kubectl describe pod -n " + namespace + " " + resourceName + " # Check image name and pull secrets"
		}
		return "kubectl describe pod " + resourceName + " # Check image name and pull secrets"
	case model.AlertTypePodHighRestarts:
		if namespace != "" {
			return "kubectl logs -n " + namespace + " " + resourceName + " --previous # Check restart causes"
		}
		return "kubectl logs " + resourceName + " --previous # Check restart causes"
	case model.AlertTypePodPendingTooLong:
		if namespace != "" {
			return "kubectl describe pod -n " + namespace + " " + resourceName + " # Check scheduling issues"
		}
		return "kubectl describe pod " + resourceName + " # Check scheduling issues (resources, affinity, taints)"
	case model.AlertTypePodFailed:
		if namespace != "" {
			return "kubectl describe pod -n " + namespace + " " + resourceName + " # Check failure reason"
		}
		return "kubectl describe pod " + resourceName + " # Check failure reason"
	case model.AlertTypePodEvicted:
		if namespace != "" {
			return "kubectl describe pod -n " + namespace + " " + resourceName + " # Check eviction reason (likely resource pressure)"
		}
		return "kubectl describe pod " + resourceName + " # Check eviction reason"
	case model.AlertTypePodUnschedulable:
		if namespace != "" {
			return "kubectl describe pod -n " + namespace + " " + resourceName + " # Check node selectors, affinity, taints"
		}
		return "kubectl describe pod " + resourceName + " # Check node selectors, affinity, taints"

	// Service alerts
	case model.AlertTypeServiceNoEndpoints:
		if namespace != "" {
			return "kubectl get endpoints -n " + namespace + " " + resourceName + " # Check selector matches pod labels"
		}
		return "kubectl get endpoints " + resourceName + " # Check selector matches pod labels"

	// Storage alerts
	case model.AlertTypePVCPendingTooLong:
		if namespace != "" {
			return "kubectl describe pvc -n " + namespace + " " + resourceName + " # Check storage class and provisioner"
		}
		return "kubectl describe pvc " + resourceName + " # Check storage class and provisioner"
	case model.AlertTypePVCNearCapacity:
		return "Consider expanding the PVC or cleaning up data"

	// Cluster resource alerts
	case model.AlertTypeClusterCPUCritical:
		return "kubectl top nodes # Consider adding nodes or optimizing workloads"
	case model.AlertTypeClusterMemoryCritical:
		return "kubectl top nodes # Consider adding nodes or increasing memory limits"
	case model.AlertTypeClusterPodCapacity:
		return "kubectl get pods -A | wc -l # Check pod distribution across nodes"

	default:
		return ""
	}
}

// GetRecommendedActionChinese returns the recommended action in Chinese
func GetRecommendedActionChinese(alertType model.AlertType, namespace, resourceName string) string {
	switch alertType {
	// Node alerts
	case model.AlertTypeNodeNotReady:
		return "kubectl describe node " + resourceName + " # 检查节点状态和事件"
	case model.AlertTypeNodeMemoryPressure:
		return "kubectl top pods --all-namespaces --sort-by=memory # 查找高内存占用 Pod"
	case model.AlertTypeNodeDiskPressure:
		return "kubectl describe node " + resourceName + " # 检查磁盘使用，考虑清理或扩容"
	case model.AlertTypeNodePIDPressure:
		return "检查进程泄漏问题，考虑清理僵尸进程"
	case model.AlertTypeNodeCPUCritical, model.AlertTypeNodeCPUHigh:
		return "kubectl top pods --all-namespaces --sort-by=cpu # 查找高 CPU 占用 Pod"
	case model.AlertTypeNodeMemoryCritical, model.AlertTypeNodeMemoryHigh:
		return "kubectl top pods --all-namespaces --sort-by=memory # 查找高内存占用 Pod"

	// Pod alerts
	case model.AlertTypePodOOMKilled:
		if namespace != "" {
			return "kubectl logs -n " + namespace + " " + resourceName + " --previous # 查看 OOM 前日志，考虑增加内存限制"
		}
		return "kubectl logs " + resourceName + " --previous # 查看 OOM 前日志，考虑增加内存限制"
	case model.AlertTypePodCrashLoopBackOff:
		if namespace != "" {
			return "kubectl logs -n " + namespace + " " + resourceName + " --previous # 查看崩溃日志"
		}
		return "kubectl logs " + resourceName + " --previous # 查看崩溃日志"
	case model.AlertTypePodImagePullBackOff:
		if namespace != "" {
			return "kubectl describe pod -n " + namespace + " " + resourceName + " # 检查镜像名称和拉取密钥"
		}
		return "kubectl describe pod " + resourceName + " # 检查镜像名称和拉取密钥"
	case model.AlertTypePodHighRestarts:
		if namespace != "" {
			return "kubectl logs -n " + namespace + " " + resourceName + " --previous # 检查重启原因"
		}
		return "kubectl logs " + resourceName + " --previous # 检查重启原因"
	case model.AlertTypePodPendingTooLong:
		if namespace != "" {
			return "kubectl describe pod -n " + namespace + " " + resourceName + " # 检查调度问题"
		}
		return "kubectl describe pod " + resourceName + " # 检查调度问题（资源、亲和性、污点）"
	case model.AlertTypePodFailed:
		if namespace != "" {
			return "kubectl describe pod -n " + namespace + " " + resourceName + " # 检查失败原因"
		}
		return "kubectl describe pod " + resourceName + " # 检查失败原因"
	case model.AlertTypePodEvicted:
		if namespace != "" {
			return "kubectl describe pod -n " + namespace + " " + resourceName + " # 检查驱逐原因（可能是资源压力）"
		}
		return "kubectl describe pod " + resourceName + " # 检查驱逐原因"
	case model.AlertTypePodUnschedulable:
		if namespace != "" {
			return "kubectl describe pod -n " + namespace + " " + resourceName + " # 检查节点选择器、亲和性、污点"
		}
		return "kubectl describe pod " + resourceName + " # 检查节点选择器、亲和性、污点"

	// Service alerts
	case model.AlertTypeServiceNoEndpoints:
		if namespace != "" {
			return "kubectl get endpoints -n " + namespace + " " + resourceName + " # 检查选择器是否匹配 Pod 标签"
		}
		return "kubectl get endpoints " + resourceName + " # 检查选择器是否匹配 Pod 标签"

	// Storage alerts
	case model.AlertTypePVCPendingTooLong:
		if namespace != "" {
			return "kubectl describe pvc -n " + namespace + " " + resourceName + " # 检查存储类和供应商"
		}
		return "kubectl describe pvc " + resourceName + " # 检查存储类和供应商"
	case model.AlertTypePVCNearCapacity:
		return "考虑扩展 PVC 或清理数据"

	// Cluster resource alerts
	case model.AlertTypeClusterCPUCritical:
		return "kubectl top nodes # 考虑增加节点或优化工作负载"
	case model.AlertTypeClusterMemoryCritical:
		return "kubectl top nodes # 考虑增加节点或提高内存限制"
	case model.AlertTypeClusterPodCapacity:
		return "kubectl get pods -A | wc -l # 检查 Pod 在节点上的分布"

	default:
		return ""
	}
}

// DiagnosticResult represents the result of a diagnostic check
type DiagnosticResult struct {
	Title       string
	Description string
	Severity    model.AlertSeverity
	Actions     []string
}

// GetAlertPriority returns a priority score for sorting alerts (higher = more urgent)
func GetAlertPriority(alertType model.AlertType, severity model.AlertSeverity) int {
	// Base priority from severity
	basePriority := int(severity) * 100

	// Additional priority based on alert type (some issues are more urgent than others)
	switch alertType {
	// Highest priority - immediate action needed
	case model.AlertTypeNodeNotReady:
		return basePriority + 50
	case model.AlertTypePodOOMKilled:
		return basePriority + 45
	case model.AlertTypePodCrashLoopBackOff:
		return basePriority + 40
	case model.AlertTypeNodeMemoryPressure:
		return basePriority + 35
	case model.AlertTypeNodeDiskPressure:
		return basePriority + 30

	// High priority - should address soon
	case model.AlertTypeNodeCPUCritical:
		return basePriority + 25
	case model.AlertTypeNodeMemoryCritical:
		return basePriority + 25
	case model.AlertTypePodImagePullBackOff:
		return basePriority + 20
	case model.AlertTypeServiceNoEndpoints:
		return basePriority + 15

	// Medium priority
	case model.AlertTypePodPendingTooLong:
		return basePriority + 10
	case model.AlertTypePodHighRestarts:
		return basePriority + 10
	case model.AlertTypePVCPendingTooLong:
		return basePriority + 5

	default:
		return basePriority
	}
}
