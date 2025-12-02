package datasource

import (
	"testing"
)

func TestParseCountQuantity(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int64
	}{
		{"empty string", "", 0},
		{"plain integer", "16", 16},
		{"zero", "0", 0},
		{"large number", "1000", 1000},
		{"with decimal", "0.5", 1},        // resource.ParseQuantity rounds 0.5 up to 1
		{"kubernetes quantity", "8", 8},   // Plain number
		{"negative", "-5", -5},            // Negative should work
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseCountQuantity(tt.input)
			if result != tt.expected {
				t.Errorf("parseCountQuantity(%q) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseResourceQuantity(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int64
	}{
		{"empty string", "", 0},
		{"millicores", "1000m", 1000},
		{"cores to millicores", "4", 4000},    // 4 cores = 4000m
		{"half core millicores", "500m", 500},
		{"zero", "0", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseResourceQuantity(tt.input)
			if result != tt.expected {
				t.Errorf("parseResourceQuantity(%q) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseMemoryQuantity(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int64
	}{
		{"empty string", "", 0},
		{"kilobytes", "1024Ki", 1024 * 1024},
		{"megabytes", "1Mi", 1024 * 1024},
		{"gigabytes", "1Gi", 1024 * 1024 * 1024},
		{"terabytes", "1Ti", 1024 * 1024 * 1024 * 1024},
		{"plain bytes", "1048576", 1048576},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseMemoryQuantity(tt.input)
			if result != tt.expected {
				t.Errorf("parseMemoryQuantity(%q) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

// TestNPUNotMultipliedBy1000 specifically tests that NPU counts are not incorrectly multiplied by 1000
func TestNPUNotMultipliedBy1000(t *testing.T) {
	// This is the critical test - NPU "16" should be 16, not 16000
	result := parseCountQuantity("16")
	if result != 16 {
		t.Errorf("NPU count 16 was incorrectly parsed as %d (should not be multiplied by 1000)", result)
	}

	// Also test pods
	podsResult := parseCountQuantity("100")
	if podsResult != 100 {
		t.Errorf("Pod count 100 was incorrectly parsed as %d (should not be multiplied by 1000)", podsResult)
	}

	// Contrast with CPU which SHOULD be multiplied by 1000
	cpuResult := parseResourceQuantity("4")
	if cpuResult != 4000 {
		t.Errorf("CPU count 4 should be converted to millicores (4000), got %d", cpuResult)
	}
}
