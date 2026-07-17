package version

// Version is injected at build time via ldflags:
//
//	go build -ldflags "-X github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/internal/version.Version=$(VERSION)"
var Version = "unknown"
