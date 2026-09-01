# Xpace MVP acceptance status

Last updated: 2026-08-23 (Asia/Jakarta)

## Release decision

The Xpace Meet/Admin MVP is deployed and suitable for controlled acceptance testing. It is not yet declared generally available because physical cross-device media acceptance and the remaining partial browser/accessibility checks still require manual evidence.

## Automated evidence completed

- Go unit tests pass for authentication, authorization, meeting APIs, user management, and platform helpers.
- The integration lifecycle passes for login, failed login, active-user creation, duplicate identity rejection, invitation activation, single-use token enforcement, meeting creation, waiting room, admission, LiveKit token issuance, leave, end-for-all, history, logout, and audit events.
- Frontend lint and production build pass, including `/accept-invite`, prejoin, room, and administration routes.
- Responsive source audit covers dashboard, login, invitation acceptance, prejoin, waiting room, mobile meeting header/actions, safe areas, 44 px touch targets, participant bottom sheet, diagnostics sheet, and mobile administration.
- Accessibility source audit covers keyboard focus, reduced motion, navigation/control names, modal semantics, live announcements, media controls, non-color network status, expanded states, and password autocomplete.
- Production smoke checks confirm web, API, PostgreSQL readiness, LiveKit signaling, security headers, camera/microphone policy, and private recording storage.
- Production invitation acceptance was tested with a temporary account: first activation returned HTTP 200, replay returned HTTP 410, subsequent login returned HTTP 200, and all temporary records were removed.

## Manual acceptance still required

- Create a `MEMBER` through Admin → Users on production and sign in from a second physical device.
- Run one meeting across two accounts, two devices, and preferably two different networks.
- Verify iPhone Safari, Android Chrome, and tablet portrait/landscape, including orientation changes.
- Confirm actual microphone, camera, speaker routing, screen sharing, chat, waiting-room admission, participant moderation, reconnect, recording, leave, and end-for-all.
- Record screenshots/device versions and any defects found during the session.

## Known limitations

- Device compatibility is currently supported by responsive implementation and source audits, but the physical device matrix is not yet signed off.
- Email delivery is not integrated; administrators must securely copy and send the one-time invitation link.
- Invitation links expire after 72 hours and are shown once. There is not yet a resend/revoke invitation control.
- Password reset and self-service password change are not yet implemented.
- The product is currently the Meet/Admin MVP. Full Chat, Rooms, Drive, Calendar, enterprise identity, AI collaboration, multi-region, webinar, SDK, and marketplace capabilities remain post-MVP roadmap work.

## Acceptance sign-off template

- Tester/date:
- Host device/browser/network:
- Member device/browser/network:
- Meeting code:
- Audio/video result:
- Waiting room/moderation result:
- Chat/screen share result:
- Recording result:
- Reconnect/orientation result:
- Defects/notes:
- Decision: pass / conditional pass / fail
