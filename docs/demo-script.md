# Xpace PoC demo script

Target duration: 12–15 minutes. Use two accounts in separate browser profiles.

## 1. Secure workspace sign-in — 1 minute

Open `http://localhost:3300/login`. Explain workspace-scoped authentication,
HttpOnly signed sessions, role permissions, and tenant isolation. Sign in as a
host or tenant administrator.

## 2. Workspace and meeting creation — 2 minutes

Show the responsive workspace dashboard. Create a meeting with the waiting room
enabled and copy the generated join code. Point out that meeting and policy
operations are tenant-scoped and audited.

## 3. Prejoin and waiting room — 2 minutes

In the second profile, join using the code. Demonstrate camera, microphone, and
speaker selection. Request entry, then return to the host profile and admit the
participant from the participant panel.

## 4. Adaptive realtime media — 3 minutes

Show connection quality, RTT, jitter, loss, bitrate, resolution, codec, and
layer diagnostics. Run the deterministic network demo to show high → medium →
low → recovery publishing. Enable low-bandwidth mode and explain that camera is
paused while audio remains prioritized with DTX and RED.

## 5. Moderation and recording — 3 minutes

Demonstrate lock/unlock, mute, promote to co-host, and remove where appropriate.
Start a short recording, stop it, and explain private MinIO storage and
short-lived authorized download URLs. Never expose object keys or secrets.

## 6. Administration and audit — 2 minutes

Open `/admin` to show tenant health and usage, then `/admin/audit` to show login,
meeting, recording, and moderation events. Mention rate limits, security
headers, safe errors, and tenant constraints.

## 7. Close — 1 minute

End the meeting for everyone and show meeting history. Summarize the PoC scope:
secure multi-tenant collaboration, adaptive media, moderation, recording,
administration, and auditability. Clearly identify post-MVP features as roadmap
items rather than current functionality.

## Demo fallback

If camera access is unavailable, continue with avatar/audio-only mode. If an
external network blocks UDP, demonstrate TCP/TURN fallback. If Egress is slow,
show a previously generated recording entry while preserving the private bucket
policy. Do not weaken authentication or storage permissions during a demo.
