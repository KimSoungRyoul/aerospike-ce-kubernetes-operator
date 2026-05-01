/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// This file lives in the external test package (v1alpha1_test) so it can
// import internal/utils. internal/utils already depends on api/v1alpha1, so
// adding utils to the api package itself would create an import cycle. The
// external test package side-steps that cycle while still exercising the
// real package-internal helper via the ServiceMonitorNameForTest export.
package v1alpha1_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/ksr/aerospike-ce-kubernetes-operator/api/v1alpha1"
	"github.com/ksr/aerospike-ce-kubernetes-operator/internal/utils"
)

// TestServiceMonitorNameMatchesUtilsHelper pins the webhook-side
// serviceMonitorName helper to internal/utils.ServiceMonitorName so future
// drift between the two implementations is caught at test time. The two
// helpers must stay identical; the duplication only exists to dodge the
// api -> internal/utils -> api import cycle.
func TestServiceMonitorNameMatchesUtilsHelper(t *testing.T) {
	cluster := &v1alpha1.AerospikeCluster{ObjectMeta: metav1.ObjectMeta{Name: "test"}}
	want := utils.ServiceMonitorName(cluster.Name)
	got := v1alpha1.ServiceMonitorNameForTest(cluster)
	if got != want {
		t.Fatalf("serviceMonitorName drift: webhook=%q, utils=%q", got, want)
	}
}
