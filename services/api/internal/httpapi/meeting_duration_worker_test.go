package httpapi

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestMeetingDurationWorkerConfig(t *testing.T) {
	t.Setenv("MEETING_DURATION_WORKER_ENABLED", "true")
	t.Setenv("MEETING_DURATION_INITIAL_DELAY", "12s")
	t.Setenv("MEETING_DURATION_INTERVAL", "45s")
	config := loadMeetingDurationWorkerConfig()
	if !config.Enabled || config.InitialDelay != 12*time.Second || config.Interval != 45*time.Second {
		t.Fatalf("unexpected meeting duration config: %+v", config)
	}
}

func TestEnforceMeetingDurationEndsParticipantsAndClosesRoom(t *testing.T) {
	api, mock := mockAPI(t)
	item := expiredMeeting{ID: "meeting-1", TenantID: "tenant-1", RoomName: "room-1", Status: "ACTIVE", LimitMinutes: 60}
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE meetings SET status='ENDED'").WithArgs(item.ID, item.TenantID, item.LimitMinutes).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE meeting_participants SET status='LEFT'").WithArgs(item.ID).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("INSERT INTO audit_events").WithArgs(item.TenantID, item.ID, item.LimitMinutes).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	closed := ""
	mock.ExpectExec("UPDATE meetings SET room_closed_at").WithArgs(item.ID, item.TenantID).WillReturnResult(sqlmock.NewResult(0, 1))

	err := enforceMeetingDuration(context.Background(), api.database, item, func(_ context.Context, room string) error {
		closed = room
		return nil
	})
	if err != nil || closed != item.RoomName {
		t.Fatalf("duration enforcement failed: room=%q err=%v", closed, err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestEnforceMeetingDurationRetriesRoomClosure(t *testing.T) {
	api, mock := mockAPI(t)
	item := expiredMeeting{ID: "meeting-1", TenantID: "tenant-1", RoomName: "room-1", Status: "ENDED", LimitMinutes: 60}
	err := enforceMeetingDuration(context.Background(), api.database, item, func(context.Context, string) error {
		return errors.New("temporary LiveKit failure")
	})
	if err == nil {
		t.Fatal("expected room closure error")
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestEnforceMeetingDurationAcceptsAlreadyClosedRoom(t *testing.T) {
	api, mock := mockAPI(t)
	item := expiredMeeting{ID: "meeting-1", TenantID: "tenant-1", RoomName: "room-1", Status: "ENDED", LimitMinutes: 60}
	mock.ExpectExec("UPDATE meetings SET room_closed_at").WithArgs(item.ID, item.TenantID).WillReturnResult(sqlmock.NewResult(0, 1))
	err := enforceMeetingDuration(context.Background(), api.database, item, func(context.Context, string) error {
		return errors.New("twirp error not_found: requested room does not exist")
	})
	if err != nil {
		t.Fatalf("already closed room should be accepted: %v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
