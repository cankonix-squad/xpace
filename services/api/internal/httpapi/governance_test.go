package httpapi

import "testing"

func TestGovernancePolicyValidation(t *testing.T) {
	valid := governancePolicy{RecordingRetentionDays: 90, DriveTrashRetentionDays: 30, ChatRetentionDays: 365, AuditRetentionDays: 365}
	if !valid.validate() {
		t.Fatal("expected default governance policy to be valid")
	}
	invalid := []governancePolicy{
		{RecordingRetentionDays: 0, DriveTrashRetentionDays: 30, ChatRetentionDays: 365, AuditRetentionDays: 365},
		{RecordingRetentionDays: 90, DriveTrashRetentionDays: 3651, ChatRetentionDays: 365, AuditRetentionDays: 365},
		{RecordingRetentionDays: 90, DriveTrashRetentionDays: 30, ChatRetentionDays: 365, AuditRetentionDays: 29},
	}
	for _, policy := range invalid {
		if policy.validate() {
			t.Fatalf("expected policy to be invalid: %+v", policy)
		}
	}
}

func TestLegalHoldResourceTypes(t *testing.T) {
	for _, value := range []string{"RECORDING", "drive_file", " Chat_Conversation "} {
		if !validLegalHoldResourceType(value) {
			t.Fatalf("expected %q to be valid", value)
		}
	}
	for _, value := range []string{"", "AUDIT_EVENT", "recordings"} {
		if validLegalHoldResourceType(value) {
			t.Fatalf("expected %q to be invalid", value)
		}
	}
}

func TestGovernanceResourceLockKeyIsResourceScoped(t *testing.T) {
	first := governanceResourceLockKey("tenant-a", "RECORDING", "resource-a")
	if first == governanceResourceLockKey("tenant-b", "RECORDING", "resource-a") || first == governanceResourceLockKey("tenant-a", "DRIVE_FILE", "resource-a") || first == governanceResourceLockKey("tenant-a", "RECORDING", "resource-b") {
		t.Fatal("governance lock key must include tenant, resource type, and resource id")
	}
}
