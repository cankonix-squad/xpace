package httpapi

import (
	"strings"
	"testing"
)

func TestCanManageRecordingAccess(t *testing.T) {
	meeting := meetingResponse{HostID: "host"}
	if !canManageRecordingAccess(meeting, currentUser{ID: "host", Role: roleMember}) {
		t.Fatal("meeting host must manage recording access")
	}
	if !canManageRecordingAccess(meeting, currentUser{ID: "admin", Role: roleTenantAdmin}) {
		t.Fatal("tenant admin must manage recording access")
	}
	if canManageRecordingAccess(meeting, currentUser{ID: "member", Role: roleMember}) {
		t.Fatal("ordinary member must not manage recording access")
	}
}

func TestRecordingObjectClientConfiguration(t *testing.T) {
	t.Setenv("RECORDING_S3_PUBLIC_ENDPOINT", "https://recordings.example.com")
	t.Setenv("MINIO_ROOT_USER", "access-key")
	t.Setenv("MINIO_ROOT_PASSWORD", strings.Repeat("s", 16))
	client, bucket, err := recordingObjectClient()
	if err != nil || client == nil || bucket != "xpace-recordings" {
		t.Fatalf("valid object client configuration rejected: %v", err)
	}
	t.Setenv("RECORDING_S3_PUBLIC_ENDPOINT", "file:///private")
	if _, _, err = recordingObjectClient(); err == nil {
		t.Fatal("non-HTTP public endpoint must be rejected")
	}
}
