package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/livekit/protocol/livekit"
	lksdk "github.com/livekit/server-sdk-go/v2"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

func main() {
	key, secret := required("LIVEKIT_API_KEY"), required("LIVEKIT_API_SECRET")
	liveKitURL := envOr("LIVEKIT_API_URL", "http://127.0.0.1:7880")
	roomName := fmt.Sprintf("xpace-recording-smoke-%d", time.Now().Unix())
	objectKey := fmt.Sprintf("smoke-tests/%s.mp4", roomName)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	roomClient := lksdk.NewRoomServiceClient(liveKitURL, key, secret)
	if _, err := roomClient.CreateRoom(ctx, &livekit.CreateRoomRequest{Name: roomName, EmptyTimeout: 60}); err != nil {
		panic(fmt.Errorf("create room: %w", err))
	}
	defer roomClient.DeleteRoom(context.Background(), &livekit.DeleteRoomRequest{Room: roomName})
	participant, err := lksdk.ConnectToRoom(liveKitURL, lksdk.ConnectInfo{
		APIKey: key, APISecret: secret, RoomName: roomName, ParticipantIdentity: "recording-smoke-host",
	}, nil)
	if err != nil {
		panic(fmt.Errorf("join smoke participant: %w", err))
	}
	defer participant.Disconnect()
	track, err := lksdk.NewLocalSampleTrack(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2})
	if err != nil {
		panic(fmt.Errorf("create smoke audio track: %w", err))
	}
	if _, err = participant.LocalParticipant.PublishTrack(track, &lksdk.TrackPublicationOptions{Name: "smoke-audio"}); err != nil {
		panic(fmt.Errorf("publish smoke audio track: %w", err))
	}
	stopAudio := make(chan struct{})
	defer close(stopAudio)
	go func() {
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_ = track.WriteSample(media.Sample{Data: []byte{0xf8, 0xff, 0xfe}, Duration: 20 * time.Millisecond}, nil)
			case <-stopAudio:
				return
			}
		}
	}()

	egress := lksdk.NewEgressClient(liveKitURL, key, secret)
	info, err := egress.StartRoomCompositeEgress(ctx, &livekit.RoomCompositeEgressRequest{
		RoomName: roomName,
		Layout:   "grid",
		FileOutputs: []*livekit.EncodedFileOutput{{
			FileType: livekit.EncodedFileType_MP4,
			Filepath: objectKey,
			Output: &livekit.EncodedFileOutput_S3{S3: &livekit.S3Upload{
				AccessKey:      envOr("MINIO_ROOT_USER", "xpace-local"),
				Secret:         required("MINIO_ROOT_PASSWORD"),
				Endpoint:       envOr("RECORDING_S3_ENDPOINT", "http://minio:9000"),
				Bucket:         envOr("RECORDING_S3_BUCKET", "xpace-recordings"),
				ForcePathStyle: true,
			}},
		}},
	})
	if err != nil {
		panic(fmt.Errorf("start egress: %w", err))
	}
	if err = waitForEgressActive(ctx, egress, info.EgressId); err != nil {
		panic(err)
	}
	time.Sleep(8 * time.Second)
	if _, err = egress.StopEgress(ctx, &livekit.StopEgressRequest{EgressId: info.EgressId}); err != nil {
		panic(fmt.Errorf("stop egress: %w", err))
	}
	verifyRecordingObject(ctx, objectKey)
	fmt.Printf("recording smoke completed: create, signed download, and delete passed for %s\n", objectKey)
}

func waitForEgressActive(ctx context.Context, client *lksdk.EgressClient, egressID string) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		response, err := client.ListEgress(ctx, &livekit.ListEgressRequest{EgressId: egressID})
		if err != nil {
			return fmt.Errorf("inspect egress startup: %w", err)
		}
		if len(response.Items) != 0 {
			info := response.Items[0]
			switch info.Status {
			case livekit.EgressStatus_EGRESS_ACTIVE:
				return nil
			case livekit.EgressStatus_EGRESS_FAILED,
				livekit.EgressStatus_EGRESS_ABORTED,
				livekit.EgressStatus_EGRESS_LIMIT_REACHED:
				return fmt.Errorf("egress startup failed: status=%s error=%s", info.Status, info.Error)
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for egress active: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func verifyRecordingObject(ctx context.Context, objectKey string) {
	accessKey, secret := envOr("MINIO_ROOT_USER", "xpace-local"), required("MINIO_ROOT_PASSWORD")
	bucket := envOr("RECORDING_S3_BUCKET", "xpace-recordings")
	internal := objectClient(envOr("RECORDING_S3_ENDPOINT", "http://minio:9000"), accessKey, secret)
	var object minio.ObjectInfo
	var err error
	for deadline := time.Now().Add(35 * time.Second); time.Now().Before(deadline); {
		object, err = internal.StatObject(ctx, bucket, objectKey, minio.StatObjectOptions{})
		if err == nil && object.Size > 0 {
			break
		}
		time.Sleep(time.Second)
	}
	if err != nil || object.Size == 0 {
		panic(fmt.Errorf("recorded object was not finalized: %w", err))
	}

	signedURL, err := internal.PresignedGetObject(ctx, bucket, objectKey, 5*time.Minute, nil)
	if err != nil {
		panic(fmt.Errorf("issue signed recording URL: %w", err))
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, signedURL.String(), nil)
	if err != nil {
		panic(fmt.Errorf("create signed recording request: %w", err))
	}
	request.Header.Set("Range", "bytes=0-1023")
	response, err := (&http.Client{Timeout: 20 * time.Second}).Do(request)
	if err != nil {
		panic(fmt.Errorf("download signed recording URL: %w", err))
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusPartialContent {
		panic(fmt.Errorf("signed recording URL returned %s", response.Status))
	}
	if bytesRead, readErr := io.Copy(io.Discard, io.LimitReader(response.Body, 1024)); readErr != nil || bytesRead == 0 {
		panic(fmt.Errorf("read signed recording response: bytes=%d err=%w", bytesRead, readErr))
	}
	if err = internal.RemoveObject(ctx, bucket, objectKey, minio.RemoveObjectOptions{}); err != nil {
		panic(fmt.Errorf("delete smoke recording: %w", err))
	}
	if _, err = internal.StatObject(ctx, bucket, objectKey, minio.StatObjectOptions{}); err == nil {
		panic("deleted smoke recording is still available")
	}
}

func objectClient(endpoint, accessKey, secret string) *minio.Client {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || parsed.Path != "" && parsed.Path != "/" {
		panic(fmt.Errorf("invalid recording endpoint %q", endpoint))
	}
	client, err := minio.New(parsed.Host, &minio.Options{
		Creds: credentials.NewStaticV4(accessKey, secret, ""), Secure: parsed.Scheme == "https",
	})
	if err != nil {
		panic(fmt.Errorf("create recording object client: %w", err))
	}
	return client
}

func required(name string) string {
	value := os.Getenv(name)
	if value == "" {
		panic(name + " is required")
	}
	return value
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
