package httpapi

import (
	"strings"
	"testing"
	"time"

	"github.com/livekit/protocol/livekit"
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

func TestCompletedRecordingMetrics(t *testing.T) {
	info := &livekit.EgressInfo{FileResults: []*livekit.FileInfo{{
		Filename: "tenants/t/meetings/m/video.mp4",
		Size:     4096,
		Duration: int64(17 * time.Second),
	}}}
	size, duration := completedRecordingMetrics(info, "tenants/t/meetings/m/video.mp4")
	if size != 4096 || duration != 17 {
		t.Fatalf("unexpected completed recording metrics: size=%d duration=%d", size, duration)
	}
	if size, duration = completedRecordingMetrics(info, "another.mp4"); size != 0 || duration != 0 {
		t.Fatalf("metrics from a different output must not be used: size=%d duration=%d", size, duration)
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
