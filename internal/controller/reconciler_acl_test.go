package controller

import (
	"errors"
	"strings"
	"testing"

	aero "github.com/aerospike/aerospike-client-go/v8"

	ackov1alpha1 "github.com/ksr/aerospike-ce-kubernetes-operator/api/v1alpha1"
)

const (
	testNamespace = "myns"
	testSetName   = "myset"
)

// --- parsePrivilege split tests (now uses strings.SplitN internally) ---

func TestParsePrivilegeSplit_SinglePart(t *testing.T) {
	parts := strings.SplitN("read", ".", 3)
	if len(parts) != 1 || parts[0] != "read" {
		t.Errorf("SplitN(\"read\") = %v, want [\"read\"]", parts)
	}
}

func TestParsePrivilegeSplit_TwoParts(t *testing.T) {
	parts := strings.SplitN("read-write.testNamespace", ".", 3)
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(parts))
	}
	if parts[0] != "read-write" || parts[1] != "testNamespace" {
		t.Errorf("SplitN = %v, want [\"read-write\", \"testNamespace\"]", parts)
	}
}

func TestParsePrivilegeSplit_ThreeParts(t *testing.T) {
	parts := strings.SplitN("write."+testNamespace+"."+testSetName, ".", 3)
	if len(parts) != 3 {
		t.Fatalf("expected 3 parts, got %d", len(parts))
	}
	if parts[0] != "write" || parts[1] != testNamespace || parts[2] != testSetName {
		t.Errorf("SplitN = %v, want [\"write\", %q, %q]", parts, testNamespace, testSetName)
	}
}

func TestParsePrivilegeSplit_EmptyString(t *testing.T) {
	parts := strings.SplitN("", ".", 3)
	if len(parts) != 1 || parts[0] != "" {
		t.Errorf("SplitN(\"\") = %v, want [\"\"]", parts)
	}
}

// --- privilegeFromCodeString tests ---

func TestPrivilegeFromCodeString_AllKnownCodes(t *testing.T) {
	// Compare by creating expected Privilege structs with exported constants
	tests := []struct {
		code         string
		expectedPriv aero.Privilege
	}{
		{"read", aero.Privilege{Code: aero.Read}},
		{"write", aero.Privilege{Code: aero.Write}},
		{"read-write", aero.Privilege{Code: aero.ReadWrite}},
		{"read-write-udf", aero.Privilege{Code: aero.ReadWriteUDF}},
		{"sys-admin", aero.Privilege{Code: aero.SysAdmin}},
		{"user-admin", aero.Privilege{Code: aero.UserAdmin}},
		{"data-admin", aero.Privilege{Code: aero.DataAdmin}},
		{"truncate", aero.Privilege{Code: aero.Truncate}},
	}

	for _, tc := range tests {
		priv, err := privilegeFromCodeString(tc.code)
		if err != nil {
			t.Errorf("privilegeFromCodeString(%q) unexpected error: %v", tc.code, err)
			continue
		}
		if priv.Code != tc.expectedPriv.Code {
			t.Errorf("privilegeFromCodeString(%q).Code = %v, want %v", tc.code, priv.Code, tc.expectedPriv.Code)
		}
	}
}

func TestPrivilegeFromCodeString_UnknownReturnsError(t *testing.T) {
	_, err := privilegeFromCodeString("unknown-code")
	if err == nil {
		t.Error("expected error for unknown privilege code")
	}
	if !strings.Contains(err.Error(), "unknown privilege code") {
		t.Errorf("error should mention 'unknown privilege code', got: %v", err)
	}
}

// --- parsePrivilege tests ---

func TestParsePrivilege_CodeOnly(t *testing.T) {
	priv, err := parsePrivilege("read")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if priv.Code != aero.Read {
		t.Errorf("Code = %v, want Read", priv.Code)
	}
	if priv.Namespace != "" {
		t.Errorf("Namespace should be empty, got %q", priv.Namespace)
	}
	if priv.SetName != "" {
		t.Errorf("SetName should be empty, got %q", priv.SetName)
	}
}

func TestParsePrivilege_WithNamespace(t *testing.T) {
	priv, err := parsePrivilege("read-write." + testNamespace)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if priv.Code != aero.ReadWrite {
		t.Errorf("Code = %v, want ReadWrite", priv.Code)
	}
	if priv.Namespace != testNamespace {
		t.Errorf("Namespace = %q, want %q", priv.Namespace, testNamespace)
	}
	if priv.SetName != "" {
		t.Errorf("SetName should be empty, got %q", priv.SetName)
	}
}

func TestParsePrivilege_WithNamespaceAndSet(t *testing.T) {
	priv, err := parsePrivilege("write." + testNamespace + "." + testSetName)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if priv.Code != aero.Write {
		t.Errorf("Code = %v, want Write", priv.Code)
	}
	if priv.Namespace != testNamespace {
		t.Errorf("Namespace = %q, want %q", priv.Namespace, testNamespace)
	}
	if priv.SetName != testSetName {
		t.Errorf("SetName = %q, want %q", priv.SetName, testSetName)
	}
}

func TestParsePrivilege_UnknownCode(t *testing.T) {
	_, err := parsePrivilege("invalid-code.ns1")
	if err == nil {
		t.Error("expected error for unknown privilege code")
	}
}

// --- privilegeKey tests ---

func TestPrivilegeKey_GlobalPrivilege(t *testing.T) {
	key := privilegeKey(aero.Privilege{Code: aero.Read})
	expected := "read::"
	if key != expected {
		t.Errorf("privilegeKey = %q, want %q", key, expected)
	}
}

func TestPrivilegeKey_NamespaceScopedPrivilege(t *testing.T) {
	key := privilegeKey(aero.Privilege{Code: aero.Write, Namespace: testNamespace})
	expected := "write:" + testNamespace + ":"
	if key != expected {
		t.Errorf("privilegeKey = %q, want %q", key, expected)
	}
}

func TestPrivilegeKey_FullyScopedPrivilege(t *testing.T) {
	priv := aero.Privilege{Code: aero.ReadWrite, Namespace: testNamespace, SetName: testSetName}
	key := privilegeKey(priv)
	// Verify it contains namespace and set info; the Code portion depends on Stringer
	expectedSuffix := testNamespace + ":" + testSetName
	if len(key) == 0 {
		t.Fatal("privilegeKey returned empty string")
	}
	// Check the format is "code:namespace:set"
	if key[len(key)-len(expectedSuffix):] != expectedSuffix {
		t.Errorf("privilegeKey = %q, want suffix %q", key, expectedSuffix)
	}
}

// --- privilegeSet tests ---

func TestPrivilegeSet_EmptySlice(t *testing.T) {
	set := privilegeSet(nil)
	if len(set) != 0 {
		t.Errorf("expected empty set, got %d entries", len(set))
	}
}

func TestPrivilegeSet_MultiplePrivileges(t *testing.T) {
	privs := []aero.Privilege{
		{Code: aero.Read},
		{Code: aero.Write, Namespace: "ns1"},
		{Code: aero.ReadWrite, Namespace: "ns1", SetName: "set1"},
	}
	set := privilegeSet(privs)
	if len(set) != 3 {
		t.Errorf("expected 3 entries, got %d", len(set))
	}
}

func TestPrivilegeSet_DeduplicatesIdenticalPrivileges(t *testing.T) {
	privs := []aero.Privilege{
		{Code: aero.Read},
		{Code: aero.Read},
	}
	set := privilegeSet(privs)
	if len(set) != 1 {
		t.Errorf("expected 1 entry after dedup, got %d", len(set))
	}
}

// --- sliceToSet tests ---

func TestSliceToSet_EmptySlice(t *testing.T) {
	set := sliceToSet(nil)
	if len(set) != 0 {
		t.Errorf("expected empty set, got %d entries", len(set))
	}
}

func TestSliceToSet_NoDuplicates(t *testing.T) {
	set := sliceToSet([]string{"a", "b", "c"})
	if len(set) != 3 {
		t.Errorf("expected 3 entries, got %d", len(set))
	}
	if !set["a"] || !set["b"] || !set["c"] {
		t.Errorf("expected all keys to be true: %v", set)
	}
}

func TestSliceToSet_WithDuplicates(t *testing.T) {
	set := sliceToSet([]string{"a", "b", "a"})
	if len(set) != 2 {
		t.Errorf("expected 2 entries after dedup, got %d", len(set))
	}
}

// --- builtinRoles tests ---

func TestBuiltinRoles_KnownRolesAreProtected(t *testing.T) {
	expected := []string{
		"user-admin", "sys-admin", "data-admin",
		"read", "write", "read-write", "read-write-udf", "truncate",
	}
	for _, role := range expected {
		if !builtinRoles[role] {
			t.Errorf("expected builtin role %q to be protected", role)
		}
	}
}

func TestBuiltinRoles_CustomRolesAreNotProtected(t *testing.T) {
	customRoles := []string{"custom-role", "my-admin", "app-reader"}
	for _, role := range customRoles {
		if builtinRoles[role] {
			t.Errorf("custom role %q should not be in builtinRoles", role)
		}
	}
}

// --- roleParsedPrivileges tests ---

func TestRoleParsedPrivileges_MultiplePrivileges(t *testing.T) {
	roleSpec := ackov1alpha1AerospikeRoleSpec("test-role",
		"read", "write.ns1", "read-write.ns1.set1")

	privs, err := roleParsedPrivileges(roleSpec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(privs) != 3 {
		t.Fatalf("expected 3 privileges, got %d", len(privs))
	}

	if privs[0].Code != aero.Read {
		t.Errorf("privs[0].Code = %v, want Read", privs[0].Code)
	}
	if privs[1].Code != aero.Write || privs[1].Namespace != "ns1" {
		t.Errorf("privs[1] = {Code: %v, NS: %q}, want {Write, ns1}", privs[1].Code, privs[1].Namespace)
	}
	if privs[2].Code != aero.ReadWrite || privs[2].Namespace != "ns1" || privs[2].SetName != "set1" {
		t.Errorf("privs[2] = {Code: %v, NS: %q, Set: %q}, want {ReadWrite, ns1, set1}",
			privs[2].Code, privs[2].Namespace, privs[2].SetName)
	}
}

func TestRoleParsedPrivileges_EmptyPrivileges(t *testing.T) {
	roleSpec := ackov1alpha1AerospikeRoleSpec("empty-role")
	privs, err := roleParsedPrivileges(roleSpec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(privs) != 0 {
		t.Errorf("expected 0 privileges, got %d", len(privs))
	}
}

func TestRoleParsedPrivileges_UnknownCodeReturnsError(t *testing.T) {
	roleSpec := ackov1alpha1AerospikeRoleSpec("bad-role", "read", "unknown-priv")
	_, err := roleParsedPrivileges(roleSpec)
	if err == nil {
		t.Error("expected error for role with unknown privilege code")
	}
	if !strings.Contains(err.Error(), "bad-role") {
		t.Errorf("error should mention role name, got: %v", err)
	}
}

// --- isTransientAeroError tests ---

func TestIsTransientAeroError_NilError(t *testing.T) {
	if isTransientAeroError(nil) {
		t.Error("expected false for nil error")
	}
}

func TestIsTransientAeroError_TransientErrors(t *testing.T) {
	transientMessages := []string{
		"connection reset by peer",
		"dial tcp 10.0.0.1:3000: connection refused",
		"i/o timeout",
		"read tcp 10.0.0.1:3000: timeout",
		"querying roles: connection reset",
		// Failure modes seen during rolling restarts that the original
		// substring list missed.
		"write tcp 10.0.0.1:3000->10.0.0.2:3000: write: broken pipe",
		"unexpected EOF",
		"dial tcp 10.0.0.1:3000: connect: network is unreachable",
		"no available connections to the node",
		"connection error: node not connected",
		// Case-insensitive matching: wrapped errors may uppercase the message.
		"Connection Reset By Peer",
		"QUERYING USERS: TIMEOUT",
	}
	for _, msg := range transientMessages {
		err := errors.New(msg)
		if !isTransientAeroError(err) {
			t.Errorf("expected true for transient error %q", msg)
		}
	}
}

func TestIsTransientAeroError_PermanentErrors(t *testing.T) {
	permanentMessages := []string{
		"role already exists",
		"user does not exist",
		"not authenticated",
		"invalid privilege",
		"unknown privilege code \"bad\"",
	}
	for _, msg := range permanentMessages {
		err := errors.New(msg)
		if isTransientAeroError(err) {
			t.Errorf("expected false for permanent error %q", msg)
		}
	}
}

// Helper to create an AerospikeRoleSpec without importing types directly
// (since it's in the same binary via ackov1alpha1 import in reconciler_acl.go)
func ackov1alpha1AerospikeRoleSpec(name string, privileges ...string) ackov1alpha1.AerospikeRoleSpec {
	return ackov1alpha1.AerospikeRoleSpec{
		Name:       name,
		Privileges: privileges,
	}
}
