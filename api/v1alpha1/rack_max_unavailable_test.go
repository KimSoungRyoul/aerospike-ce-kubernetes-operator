package v1alpha1

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/util/intstr"
)

// --- rack.maxUnavailable is validated against the rack's own pod count ---
//
// The cluster-level check compares against spec.size. A rack of 2 pods with
// maxUnavailable: 2 is below the cluster size of 6 and so passes that check,
// while permitting the entire rack to be drained — precisely the failure per-rack
// PDBs exist to prevent (#94).

func TestValidate_RackMaxUnavailable(t *testing.T) {
	ptr := func(v intstr.IntOrString) *intstr.IntOrString { return &v }

	tests := []struct {
		name     string
		size     int32
		rackMaxs []*intstr.IntOrString
		wantErr  string // substring; empty means accepted
	}{
		{
			name:     "a budget that empties the rack is rejected",
			size:     6,
			rackMaxs: []*intstr.IntOrString{ptr(intstr.FromInt32(2)), nil, nil},
			wantErr:  "spec.rackConfig.racks[id=1].maxUnavailable",
		},
		{
			name:     "100% on a rack is rejected",
			size:     6,
			rackMaxs: []*intstr.IntOrString{nil, ptr(intstr.FromString("100%")), nil},
			wantErr:  "spec.rackConfig.racks[id=2].maxUnavailable",
		},
		{
			name:     "a budget below the rack size is accepted",
			size:     6,
			rackMaxs: []*intstr.IntOrString{ptr(intstr.FromInt32(1)), nil, nil},
		},
		{
			name:     "unset rack budgets are accepted",
			size:     6,
			rackMaxs: []*intstr.IntOrString{nil, nil, nil},
		},
		{
			// size 7 over 3 racks: racks 1 and 2 get 3 pods, rack 3 gets 2. A
			// budget of 2 is fine on the first two and fatal on the third.
			name:     "the uneven remainder rack is measured against its own size",
			size:     7,
			rackMaxs: []*intstr.IntOrString{nil, nil, ptr(intstr.FromInt32(2))},
			wantErr:  "spec.rackConfig.racks[id=3].maxUnavailable",
		},
		{
			name:     "the same budget is fine on a rack that got the remainder",
			size:     7,
			rackMaxs: []*intstr.IntOrString{ptr(intstr.FromInt32(2)), nil, nil},
		},
	}

	v := &AerospikeClusterValidator{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			racks := make([]Rack, 0, len(tt.rackMaxs))
			for i, mu := range tt.rackMaxs {
				racks = append(racks, Rack{ID: i + 1, MaxUnavailable: mu})
			}
			cluster := &AerospikeCluster{
				Spec: AerospikeClusterSpec{
					Size:       tt.size,
					Image:      "aerospike:ce-8.1.1.1",
					RackConfig: &RackConfig{Racks: racks},
				},
			}

			_, err := v.validate(cluster)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validate() error = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validate() = nil, want an error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("validate() error = %q, want it to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}
