package metering

import "testing"

func TestIsComputeInstanceBillable(t *testing.T) {
	tests := []struct {
		state string
		want  bool
	}{
		{"COMPUTE_INSTANCE_STATE_RUNNING", true},
		{"RUNNING", true},
		{"COMPUTE_INSTANCE_STATE_STOPPED", false},
		{"STOPPED", false},
		{"COMPUTE_INSTANCE_STATE_CREATING", false},
		{"DELETED", false},
		{"", false},
	}
	for _, tc := range tests {
		t.Run(tc.state, func(t *testing.T) {
			if got := IsComputeInstanceBillable(tc.state); got != tc.want {
				t.Errorf("IsComputeInstanceBillable(%q) = %v, want %v", tc.state, got, tc.want)
			}
		})
	}
}

func TestIsClusterBillable(t *testing.T) {
	tests := []struct {
		state string
		want  bool
	}{
		{"CLUSTER_STATE_READY", true},
		{"CLUSTER_STATE_PROGRESSING", true},
		{"READY", true},
		{"PROGRESSING", true},
		{"CLUSTER_STATE_CREATING", false},
		{"CLUSTER_STATE_STOPPED", false},
		{"DELETED", false},
		{"", false},
	}
	for _, tc := range tests {
		t.Run(tc.state, func(t *testing.T) {
			if got := IsClusterBillable(tc.state); got != tc.want {
				t.Errorf("IsClusterBillable(%q) = %v, want %v", tc.state, got, tc.want)
			}
		})
	}
}

func TestIsModelBillable(t *testing.T) {
	tests := []struct {
		state string
		want  bool
	}{
		{"MODEL_STATE_RUNNING", true},
		{"RUNNING", true},
		{"MODEL_STATE_STOPPED", false},
		{"CREATING", false},
		{"", false},
	}
	for _, tc := range tests {
		t.Run(tc.state, func(t *testing.T) {
			if got := IsModelBillable(tc.state); got != tc.want {
				t.Errorf("IsModelBillable(%q) = %v, want %v", tc.state, got, tc.want)
			}
		})
	}
}

func TestIsBareMetalBillable(t *testing.T) {
	tests := []struct {
		state string
		want  bool
	}{
		{"BARE_METAL_INSTANCE_STATE_RUNNING", true},
		{"RUNNING", true},
		{"BARE_METAL_INSTANCE_STATE_STOPPED", false},
		{"PROVISIONING", false},
		{"", false},
	}
	for _, tc := range tests {
		t.Run(tc.state, func(t *testing.T) {
			if got := IsBareMetalBillable(tc.state); got != tc.want {
				t.Errorf("IsBareMetalBillable(%q) = %v, want %v", tc.state, got, tc.want)
			}
		})
	}
}
