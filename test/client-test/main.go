package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	aero "github.com/aerospike/aerospike-client-go/v8"
	atypes "github.com/aerospike/aerospike-client-go/v8/types"
)

type clusterTest struct {
	Name      string
	Host      string
	Port      int
	Namespace string
}

func main() {
	clusters := []clusterTest{
		{Name: "aerospike-basic", Host: "aerospike-basic-0-0.aerospike-basic.aerospike.svc.cluster.local", Port: 3000, Namespace: "test"},
		{Name: "aerospike-3node", Host: "aerospike-3node-0-0.aerospike-3node.aerospike.svc.cluster.local", Port: 3000, Namespace: "testns"},
		{Name: "aerospike-multirack", Host: "aerospike-multirack-1-0.aerospike-multirack.aerospike.svc.cluster.local", Port: 3000, Namespace: "testns"},
		{Name: "aerospike-acl", Host: "aerospike-acl-0-0.aerospike-acl.aerospike.svc.cluster.local", Port: 3000, Namespace: "testns"},
	}

	// Allow selecting a specific cluster by index
	if len(os.Args) > 1 {
		idx, err := strconv.Atoi(os.Args[1])
		if err == nil && idx >= 0 && idx < len(clusters) {
			clusters = []clusterTest{clusters[idx]}
		}
	}

	// SKIP_DELETE=1 skips delete and verify-deletion steps
	skipDelete := os.Getenv("SKIP_DELETE") == "1"

	allPassed := true
	for _, c := range clusters {
		fmt.Printf("\n============================================================\n")
		fmt.Printf("=== Testing cluster: %s (host: %s, ns: %s) ===\n", c.Name, c.Host, c.Namespace)
		fmt.Printf("============================================================\n\n")
		if !testCluster(c, skipDelete) {
			allPassed = false
		}
	}

	if allPassed {
		fmt.Println("\n*** ALL TESTS PASSED ***")
	} else {
		fmt.Println("\n*** SOME TESTS FAILED ***")
		os.Exit(1)
	}
}

func testCluster(c clusterTest, skipDelete bool) bool {
	passed := true
	setName := "demo"
	keyVal := "crud-test-key-1"

	// 1. Connection test
	fmt.Printf("[1] Connection test...\n")
	policy := aero.NewClientPolicy()
	policy.Timeout = 15 * time.Second
	policy.IdleTimeout = 10 * time.Second
	policy.LoginTimeout = 10 * time.Second

	client, err := aero.NewClientWithPolicy(policy, c.Host, c.Port)
	if err != nil {
		fmt.Printf("    FAIL: Cannot connect: %v\n", err)
		return false
	}
	defer client.Close()

	if client.IsConnected() {
		fmt.Printf("    PASS: Connected to %s:%d\n", c.Host, c.Port)
	} else {
		fmt.Printf("    FAIL: Not connected\n")
		return false
	}

	// Cluster info
	nodes := client.GetNodes()
	fmt.Printf("    Cluster nodes: %d\n", len(nodes))
	for _, n := range nodes {
		fmt.Printf("      - %s (%s)\n", n.GetName(), n.GetHost().String())
	}

	// 2. Create record
	fmt.Printf("[2] Create record (key=%s)...\n", keyVal)
	key, err := aero.NewKey(c.Namespace, setName, keyVal)
	if err != nil {
		fmt.Printf("    FAIL: Cannot create key: %v\n", err)
		return false
	}

	bins := aero.BinMap{
		"name":    "testuser",
		"age":     25,
		"city":    "Seoul",
		"created": time.Now().Unix(),
	}

	writePolicy := aero.NewWritePolicy(0, 0)
	writePolicy.TotalTimeout = 5 * time.Second

	err = client.Put(writePolicy, key, bins)
	if err != nil {
		fmt.Printf("    FAIL: Cannot write record: %v\n", err)
		passed = false
	} else {
		fmt.Printf("    PASS: Record created with bins: name=testuser, age=25, city=Seoul\n")
	}

	// 3. Read record
	fmt.Printf("[3] Read record (key=%s)...\n", keyVal)
	readPolicy := aero.NewPolicy()
	readPolicy.TotalTimeout = 5 * time.Second

	rec, err := client.Get(readPolicy, key)
	if err != nil {
		fmt.Printf("    FAIL: Cannot read record: %v\n", err)
		passed = false
	} else if rec == nil {
		fmt.Printf("    FAIL: Record not found\n")
		passed = false
	} else {
		fmt.Printf("    PASS: Record read successfully\n")
		fmt.Printf("      name=%v, age=%v, city=%v\n", rec.Bins["name"], rec.Bins["age"], rec.Bins["city"])

		// Verify values
		if rec.Bins["name"] != "testuser" || rec.Bins["city"] != "Seoul" {
			fmt.Printf("    FAIL: Unexpected bin values\n")
			passed = false
		}
	}

	// 4. Update record
	fmt.Printf("[4] Update record (key=%s)...\n", keyVal)
	updateBins := aero.BinMap{
		"age":     30,
		"city":    "Busan",
		"updated": time.Now().Unix(),
	}

	err = client.Put(writePolicy, key, updateBins)
	if err != nil {
		fmt.Printf("    FAIL: Cannot update record: %v\n", err)
		passed = false
	} else {
		fmt.Printf("    PASS: Record updated: age=30, city=Busan\n")
	}

	// 5. Read after update
	fmt.Printf("[5] Read after update (key=%s)...\n", keyVal)
	rec, err = client.Get(readPolicy, key)
	if err != nil {
		fmt.Printf("    FAIL: Cannot read updated record: %v\n", err)
		passed = false
	} else if rec == nil {
		fmt.Printf("    FAIL: Updated record not found\n")
		passed = false
	} else {
		fmt.Printf("    PASS: Updated record read successfully\n")
		fmt.Printf("      name=%v, age=%v, city=%v\n", rec.Bins["name"], rec.Bins["age"], rec.Bins["city"])

		ageVal, ok := rec.Bins["age"].(int)
		if !ok {
			fmt.Printf("    WARN: age type is %T\n", rec.Bins["age"])
		} else if ageVal != 30 {
			fmt.Printf("    FAIL: age should be 30, got %d\n", ageVal)
			passed = false
		}
		if rec.Bins["city"] != "Busan" {
			fmt.Printf("    FAIL: city should be Busan, got %v\n", rec.Bins["city"])
			passed = false
		}
		if rec.Bins["name"] != "testuser" {
			fmt.Printf("    FAIL: name should still be testuser, got %v\n", rec.Bins["name"])
			passed = false
		}
	}

	if !skipDelete {
		// 6. Delete record
		fmt.Printf("[6] Delete record (key=%s)...\n", keyVal)
		deletePolicy := aero.NewWritePolicy(0, 0)
		deletePolicy.TotalTimeout = 5 * time.Second

		existed, err := client.Delete(deletePolicy, key)
		if err != nil {
			fmt.Printf("    FAIL: Cannot delete record: %v\n", err)
			passed = false
		} else if !existed {
			fmt.Printf("    WARN: Record did not exist before delete\n")
		} else {
			fmt.Printf("    PASS: Record deleted\n")
		}

		// 7. Verify deletion
		fmt.Printf("[7] Verify deletion (key=%s)...\n", keyVal)
		rec, err = client.Get(readPolicy, key)
		if err != nil {
			// KEY_NOT_FOUND_ERROR after delete is expected behavior
			ae := &aero.AerospikeError{}
			if errors.As(err, &ae) && ae.ResultCode == atypes.KEY_NOT_FOUND_ERROR {
				fmt.Printf("    PASS: Record correctly not found after deletion (KEY_NOT_FOUND_ERROR)\n")
			} else {
				fmt.Printf("    FAIL: Unexpected error reading after delete: %v\n", err)
				passed = false
			}
		} else if rec == nil {
			fmt.Printf("    PASS: Record correctly not found after deletion\n")
		} else {
			fmt.Printf("    FAIL: Record still exists after deletion\n")
			passed = false
		}
	} else {
		fmt.Printf("[6] Delete record... SKIPPED (SKIP_DELETE=1)\n")
		fmt.Printf("[7] Verify deletion... SKIPPED (SKIP_DELETE=1)\n")
	}

	// Summary
	if passed {
		fmt.Printf("\n--- %s: ALL TESTS PASSED ---\n", c.Name)
	} else {
		fmt.Printf("\n--- %s: SOME TESTS FAILED ---\n", c.Name)
	}

	return passed
}
