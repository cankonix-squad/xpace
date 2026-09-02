"use client";
/* eslint-disable react-hooks/exhaustive-deps -- LiveKit track keys intentionally drive focus reconciliation. */

import { CspImage as Image } from "@/components/csp-image";
import * as React from "react";
import {
  AudioTrack,
  CarouselLayout,
  Chat,
  ConnectionQualityIndicator,
  ConnectionStateToast,
  ControlBar,
  FocusLayoutContainer,
  FocusToggle,
  GridLayout,
  LayoutContextProvider,
  LockLockedIcon,
  ParticipantContextIfNeeded,
  ParticipantName,
  ParticipantPlaceholder,
  RoomAudioRenderer,
  ScreenShareIcon,
  TrackMutedIndicator,
  TrackRefContextIfNeeded,
  VideoTrack,
  useCreateLayoutContext,
  useEnsureTrackRef,
  useFeatureContext,
  useIsEncrypted,
  useParticipantTile,
  usePinnedTracks,
  useTracks,
  type TrackReference,
  type TrackReferenceOrPlaceholder,
  type WidgetState,
} from "@livekit/components-react";
import { RoomEvent, Track } from "livekit-client";

type ParticipantMetadata = { avatarUrl?: string };

function realTrack(
  track: TrackReferenceOrPlaceholder,
): track is TrackReference {
  return "publication" in track && Boolean(track.publication);
}

function sameTrack(
  left: TrackReferenceOrPlaceholder | null,
  right: TrackReferenceOrPlaceholder | null,
) {
  if (!left || !right) return left === right;
  return (
    left.participant.identity === right.participant.identity &&
    left.source === right.source &&
    (!realTrack(left) ||
      !realTrack(right) ||
      left.publication.trackSid === right.publication.trackSid)
  );
}

function avatarFromMetadata(raw?: string) {
  if (!raw) return "";
  try {
    const metadata = JSON.parse(raw) as ParticipantMetadata;
    return typeof metadata.avatarUrl === "string" &&
      metadata.avatarUrl.startsWith("/api/")
      ? metadata.avatarUrl
      : "";
  } catch {
    return "";
  }
}

function ProfileParticipantTile({
  trackRef,
}: {
  trackRef?: TrackReferenceOrPlaceholder;
}) {
  const reference = useEnsureTrackRef(trackRef);
  const { elementProps } = useParticipantTile<HTMLDivElement>({
    trackRef: reference,
    htmlProps: {},
  });
  const encrypted = useIsEncrypted(reference.participant);
  const autoSubscription = useFeatureContext()?.autoSubscription;
  const avatarUrl = avatarFromMetadata(reference.participant.metadata);

  return (
    <div className="xspace-participant-tile" {...elementProps}>
      <TrackRefContextIfNeeded trackRef={reference}>
        <ParticipantContextIfNeeded participant={reference.participant}>
          {realTrack(reference) &&
          (reference.publication.kind === "video" ||
            reference.source === Track.Source.Camera ||
            reference.source === Track.Source.ScreenShare) ? (
            <VideoTrack
              trackRef={reference}
              manageSubscription={autoSubscription}
            />
          ) : (
            realTrack(reference) && <AudioTrack trackRef={reference} />
          )}
          <div
            className={`lk-participant-placeholder ${avatarUrl ? "xspace-profile-placeholder" : ""}`}
          >
            {avatarUrl ? (
              <span className="xspace-profile-thumbnail">
                <Image
                  src={avatarUrl}
                  alt={`${reference.participant.name || "Participant"} profile`}
                  fill
                  unoptimized
                  sizes="220px"
                />
              </span>
            ) : (
              <ParticipantPlaceholder />
            )}
          </div>
          <div className="lk-participant-metadata">
            <div className="lk-participant-metadata-item">
              {reference.source === Track.Source.Camera ? (
                <>
                  {encrypted && (
                    <LockLockedIcon className="xspace-icon-gap" />
                  )}
                  <TrackMutedIndicator
                    trackRef={{
                      participant: reference.participant,
                      source: Track.Source.Microphone,
                    }}
                    show="muted"
                  />
                  <ParticipantName />
                </>
              ) : (
                <>
                  <ScreenShareIcon className="xspace-icon-gap" />
                  <ParticipantName>&apos;s screen</ParticipantName>
                </>
              )}
            </div>
            <ConnectionQualityIndicator className="lk-participant-metadata-item" />
          </div>
          <FocusToggle trackRef={reference} />
        </ParticipantContextIfNeeded>
      </TrackRefContextIfNeeded>
    </div>
  );
}

export default function XspaceVideoConference() {
  const [widget, setWidget] = React.useState<WidgetState>({
    showChat: false,
    unreadMessages: 0,
    showSettings: false,
  });
  const lastAutoFocused = React.useRef<TrackReferenceOrPlaceholder | null>(
    null,
  );
  const tracks = useTracks(
    [
      { source: Track.Source.Camera, withPlaceholder: true },
      { source: Track.Source.ScreenShare, withPlaceholder: false },
    ],
    { updateOnlyOn: [RoomEvent.ActiveSpeakersChanged], onlySubscribed: false },
  );
  const layout = useCreateLayoutContext();
  const screenShares = tracks
    .filter(realTrack)
    .filter((track) => track.publication.source === Track.Source.ScreenShare);
  const focus = usePinnedTracks(layout)?.[0];
  const carousel = tracks.filter((track) => !sameTrack(track, focus ?? null));

  React.useEffect(() => {
    if (
      screenShares.some((track) => track.publication.isSubscribed) &&
      lastAutoFocused.current === null
    ) {
      layout.pin.dispatch?.({
        msg: "set_pin",
        trackReference: screenShares[0],
      });
      lastAutoFocused.current = screenShares[0];
    } else if (
      lastAutoFocused.current &&
      !screenShares.some(
        (track) =>
          realTrack(lastAutoFocused.current as TrackReferenceOrPlaceholder) &&
          track.publication.trackSid ===
            (lastAutoFocused.current as TrackReference).publication.trackSid,
      )
    ) {
      layout.pin.dispatch?.({ msg: "clear_pin" });
      lastAutoFocused.current = null;
    }
    if (focus && !realTrack(focus)) {
      const updated = tracks.find(
        (track) =>
          track.participant.identity === focus.participant.identity &&
          track.source === focus.source,
      );
      if (updated && updated !== focus && realTrack(updated))
        layout.pin.dispatch?.({ msg: "set_pin", trackReference: updated });
    }
  }, [
    screenShares
      .map(
        (track) =>
          `${track.publication.trackSid}_${track.publication.isSubscribed}`,
      )
      .join(),
    focus,
    tracks,
  ]);

  return (
    <div className="lk-video-conference">
      <LayoutContextProvider value={layout} onWidgetChange={setWidget}>
        <div className="lk-video-conference-inner">
          {!focus ? (
            <div className="lk-grid-layout-wrapper">
              <GridLayout tracks={tracks}>
                <ProfileParticipantTile />
              </GridLayout>
            </div>
          ) : (
            <div className="lk-focus-layout-wrapper">
              <FocusLayoutContainer>
                <CarouselLayout tracks={carousel}>
                  <ProfileParticipantTile />
                </CarouselLayout>
                <ProfileParticipantTile trackRef={focus} />
              </FocusLayoutContainer>
            </div>
          )}
          <ControlBar controls={{ chat: true, settings: false }} />
        </div>
        <Chat className={widget.showChat ? "" : "xspace-chat-hidden"} />
      </LayoutContextProvider>
      <RoomAudioRenderer />
      <ConnectionStateToast />
    </div>
  );
}
