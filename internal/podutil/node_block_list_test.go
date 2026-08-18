package podutil

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
)

// --- spec.k8sNodeBlockList keeps pods off the named nodes ---
//
// The field existed in the CRD and was documented as a working feature, but no
// non-test code read it (#344): setting it mid-incident to evacuate a failing
// node produced no error, no warning, and no effect. It now renders as a
// `kubernetes.io/hostname NotIn` requirement on the pods' required node affinity.
//
// Every case below fails without applyK8sNodeBlockList, since the pre-fix
// template carried no such requirement at all. The "rack affinity" and
// "rack podSpec affinity" cases are the ones that matter most: a block list that
// is silently dropped by an unrelated affinity setting is the same bug again,
// just harder to notice.

const (
	blockedNodeA  = "node-a"
	blockedNodeB  = "node-b"
	blockTestZone = "us-east-1a"
)

func blockListCluster(blockList []string) *v1alpha1.AerospikeCluster {
	return &v1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
		Spec: v1alpha1.AerospikeClusterSpec{
			Size:             3,
			Image:            "aerospike:ce-8.1.1.1",
			K8sNodeBlockList: blockList,
		},
	}
}

// requiredTerms returns the pod's required node-affinity terms, or nil.
func requiredTerms(pt corev1.PodTemplateSpec) []corev1.NodeSelectorTerm {
	if pt.Spec.Affinity == nil || pt.Spec.Affinity.NodeAffinity == nil {
		return nil
	}
	req := pt.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
	if req == nil {
		return nil
	}
	return req.NodeSelectorTerms
}

// blockedValuesIn returns the values of the hostname NotIn requirement in the
// given term, or nil when the term carries none.
func blockedValuesIn(term corev1.NodeSelectorTerm) []string {
	for _, expr := range term.MatchExpressions {
		if expr.Key == hostnameLabelKey && expr.Operator == corev1.NodeSelectorOpNotIn {
			return expr.Values
		}
	}
	return nil
}

func TestBuildPodTemplateSpec_K8sNodeBlockList(t *testing.T) {
	tests := []struct {
		name string
		// mutate adjusts the cluster/rack before the template is built.
		blockList []string
		rack      *v1alpha1.Rack
		podSpec   *v1alpha1.AerospikePodSpec

		wantTerms   int
		wantBlocked []string
		// wantZoneTermToo asserts the pre-existing rack affinity survived
		// alongside the block in the SAME term (terms are OR'd, so a block in a
		// separate term would not block anything).
		wantZoneTermToo bool
	}{
		{
			name:        "block list alone creates the affinity",
			blockList:   []string{blockedNodeA},
			wantTerms:   1,
			wantBlocked: []string{blockedNodeA},
		},
		{
			name:        "multiple nodes go into one NotIn requirement",
			blockList:   []string{blockedNodeA, blockedNodeB},
			wantTerms:   1,
			wantBlocked: []string{blockedNodeA, blockedNodeB},
		},
		{
			name:      "empty block list adds no affinity",
			blockList: nil,
			wantTerms: 0,
		},
		{
			name:            "block joins the rack's zone affinity in the same term",
			blockList:       []string{blockedNodeA},
			rack:            &v1alpha1.Rack{ID: 1, Zone: blockTestZone},
			wantTerms:       1,
			wantBlocked:     []string{blockedNodeA},
			wantZoneTermToo: true,
		},
		{
			// applyRackPodSpecOverrides replaces podSpec.Affinity wholesale, so a
			// block applied before it would be thrown away here.
			name:      "rack podSpec affinity does not discard the block",
			blockList: []string{blockedNodeA},
			rack: &v1alpha1.Rack{
				ID: 1,
				PodSpec: &v1alpha1.RackPodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{{
									MatchExpressions: []corev1.NodeSelectorRequirement{{
										Key:      zoneTopologyKey,
										Operator: corev1.NodeSelectorOpIn,
										Values:   []string{blockTestZone},
									}},
								}},
							},
						},
					},
				},
			},
			wantTerms:       1,
			wantBlocked:     []string{blockedNodeA},
			wantZoneTermToo: true,
		},
		{
			// applyPodSpecSettings also replaces podSpec.Affinity wholesale.
			name:      "cluster podSpec affinity does not discard the block",
			blockList: []string{blockedNodeB},
			podSpec: &v1alpha1.AerospikePodSpec{
				Affinity: &corev1.Affinity{
					NodeAffinity: &corev1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
							NodeSelectorTerms: []corev1.NodeSelectorTerm{{
								MatchExpressions: []corev1.NodeSelectorRequirement{{
									Key:      zoneTopologyKey,
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{blockTestZone},
								}},
							}},
						},
					},
				},
			},
			wantTerms:       1,
			wantBlocked:     []string{blockedNodeB},
			wantZoneTermToo: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := blockListCluster(tt.blockList)
			cluster.Spec.PodSpec = tt.podSpec

			pt := BuildPodTemplateSpec(cluster, tt.rack, 0, "test-config", "abc123")
			terms := requiredTerms(pt)

			if len(terms) != tt.wantTerms {
				t.Fatalf("required node selector terms = %d, want %d (%+v)", len(terms), tt.wantTerms, terms)
			}
			if tt.wantTerms == 0 {
				return
			}

			// Every term must carry the block, or the pod can land on a blocked
			// node by satisfying a term that does not.
			for i, term := range terms {
				got := blockedValuesIn(term)
				if len(got) != len(tt.wantBlocked) {
					t.Fatalf("term %d blocked nodes = %v, want %v", i, got, tt.wantBlocked)
				}
				for j := range got {
					if got[j] != tt.wantBlocked[j] {
						t.Errorf("term %d blocked nodes = %v, want %v", i, got, tt.wantBlocked)
						break
					}
				}
			}

			if tt.wantZoneTermToo {
				found := false
				for _, expr := range terms[0].MatchExpressions {
					if expr.Key == zoneTopologyKey {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("zone affinity was dropped; term = %+v", terms[0])
				}
			}
		})
	}
}
