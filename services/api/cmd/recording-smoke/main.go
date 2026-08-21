package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/livekit/protocol/livekit"
	lksdk "github.com/livekit/server-sdk-go/v2"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

func main() {
	key, secret := required("LIVEKIT_API_KEY"), required("LIVEKIT_API_SECRET")
	url := envOr("LIVEKIT_API_URL", "http://127.0.0.1:7880")
	roomName := fmt.Sprintf("xpace-recording-smoke-%d", time.Now().Unix())
	objectKey := fmt.Sprintf("smoke-tests/%s.mp4", roomName)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	roomClient := lksdk.NewRoomServiceClient(url, key, secret)
	if _, err := roomClient.CreateRoom(ctx, &livekit.CreateRoomRequest{Name: roomName, EmptyTimeout: 60}); err != nil {
		panic(fmt.Errorf("create room: %w", err))
	}
	defer roomClient.DeleteRoom(context.Background(), &livekit.DeleteRoomRequest{Room: roomName})
	participant, err := lksdk.ConnectToRoom(url, lksdk.ConnectInfo{
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

	egress := lksdk.NewEgressClient(url, key, secret)
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
	time.Sleep(8 * time.Second)
	if _, err = egress.StopEgress(ctx, &livekit.StopEgressRequest{EgressId: info.EgressId}); err != nil {
		panic(fmt.Errorf("stop egress: %w", err))
	}
	time.Sleep(5 * time.Second)
	fmt.Printf("recording smoke completed: %s\n", objectKey)
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
