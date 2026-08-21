# CANKONIX Xpace — Build Task List

Status kerja proyek. Setiap item yang selesai wajib diubah menjadi `- [x]`.

## 0. Project Foundation

- [x] Memahami product blueprint, prototype, dan arah arsitektur Xpace.
- [x] Menetapkan workspace implementasi: `/Users/cankonix/Documents/Cankonix Labs/Xpace`.
- [x] Membuat task list proyek.
- [x] Menentukan struktur monorepo dan inisialisasi repository.
- [x] Menetapkan standar code quality, environment, dan dokumentasi lokal.
- [x] Menetapkan design tokens dari prototype sebagai theme bersama.

## 1. Technical PoC — Core Infrastructure

- [x] Menyiapkan frontend Next.js, React, TypeScript, dan Tailwind CSS.
- [x] Menyiapkan backend Go dengan struktur domain-oriented/clean architecture.
- [x] Menyiapkan PostgreSQL dan migration awal.
- [x] Menyiapkan Redis.
- [x] Menyiapkan LiveKit dan coturn untuk media realtime.
- [x] Menyiapkan MinIO/S3-compatible object storage untuk recording.
- [x] Menyiapkan Docker Compose untuk lingkungan lokal/PoC.
- [x] Menyiapkan konfigurasi environment yang aman (`.env.example`, tanpa secret di repo).
- [x] Menyiapkan reverse proxy/TLS-ready routing untuk deployment.
- [x] Menyiapkan health check dan observability dasar.

## 2. Shared Product Foundation

- [x] Implementasi design system: warna, typography, spacing, radius, icon, dan states.
- [x] Implementasi Xpace brand/logo dan app shell.
- [x] Implementasi sidebar, topbar, dan responsive navigation.
- [x] Implementasi autentikasi email/username + password.
- [x] Implementasi secure session, logout, dan audit login/logout.
- [x] Implementasi role dasar: super admin, tenant admin, host, co-host, member, guest.
- [x] Implementasi user profile dan tenant-aware data model.

## 3. Xpace Meet — MVP User Journey

- [x] Implementasi dashboard: New Meeting, Join, Schedule, upcoming, recent, recordings.
- [x] Implementasi instant meeting dan pembuatan meeting code/link.
- [x] Implementasi scheduled meeting.
- [x] Implementasi join by meeting code/link dan validasi akses.
- [x] Implementasi pre-join: permission, preview camera, mic/speaker/camera selection.
- [x] Implementasi waiting room dan admit/reject oleh host.
- [x] Integrasi token server-side dan koneksi frontend ke LiveKit.
- [x] Implementasi publish camera dan microphone.
- [x] Implementasi video grid, active speaker, dan state media peserta.
- [x] Implementasi device controls: mic, camera, speaker, dan leave meeting.
- [x] Implementasi screen sharing.
- [x] Implementasi panel participant dan kontrol host/co-host.
- [x] Implementasi meeting chat.
- [x] Implementasi meeting lock, mute/remove participant, dan promote co-host.
- [x] Implementasi reconnect, loading, permission-denied, dan meeting-ended states.
- [x] Implementasi meeting history.

## 4. Adaptive Media & Recording

- [x] Menampilkan network-quality indicator yang tidak hanya berbasis warna.
- [x] Menampilkan metrik demo: RTT, jitter, packet loss, bitrate, FPS, resolution, layer.
- [x] Mengimplementasikan low-bandwidth mode dan feedback UI yang jelas.
- [x] Memvalidasi simulcast/SVC dan adaptive subscribed layers pada LiveKit.
- [x] Memvalidasi audio-priority behavior saat bandwidth sangat rendah.
- [x] Mengimplementasikan recording dan metadata recording.
- [x] Menyimpan recording secara private di object storage.
- [x] Mengimplementasikan akses recording berbasis izin.

## 5. Xpace Admin — MVP

- [x] Implementasi admin dashboard: usage, meeting, participant, dan health summary.
- [x] Implementasi user management.
- [x] Implementasi group management dasar.
- [x] Implementasi meeting list dan meeting analytics.
- [x] Implementasi audit log tenant.
- [x] Implementasi meeting policy dasar (guest, waiting room, recording, screen share).
- [x] Implementasi system configuration dasar.

## 6. Security, Quality & Delivery

- [x] Menerapkan RBAC dan tenant isolation pada setiap API/data access.
- [x] Menerapkan rate limit, security headers, input validation, dan error handling aman.
- [x] Menerapkan token signing, secret management, dan private recording access.
- [x] Menerapkan audit event untuk tindakan keamanan dan moderasi penting.
- [x] Menulis unit test untuk domain/API utama.
- [x] Menulis integration test untuk authentication dan meeting lifecycle.
- [~] Menguji browser/device compatibility dan responsive UI.
- [~] Menguji accessibility: keyboard, ARIA, focus, contrast, dan non-color status.
- [x] Menjalankan demo adaptive-network end-to-end (normal → limited → poor → recovery).
- [x] Menulis README, local setup guide, PoC runbook, dan demo script.
- [x] Menyiapkan deployment PoC dan smoke test.

## 7. Post-MVP Roadmap (Tidak Dikerjakan Sebelum MVP Stabil)

- [ ] SSO, MFA, LDAP/AD, dan advanced RBAC.
- [ ] High availability, backup/DR, SIEM, dan advanced observability.
- [ ] Xpace Chat, Rooms, Drive, dan Calendar penuh.
- [ ] AI transcription, summary, translation, dan noise suppression.
- [ ] Multi-region, webinar/events, SDK, dan embedded communication.
