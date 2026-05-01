/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package v1alpha1

// ServiceMonitorNameForTest exposes the package-internal serviceMonitorName
// helper to external test packages (v1alpha1_test). External tests can then
// import internal/utils to compare results without creating an import cycle
// (internal/utils already depends on api/v1alpha1).
var ServiceMonitorNameForTest = serviceMonitorName
