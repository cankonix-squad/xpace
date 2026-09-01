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
- [x] Menstandardkan seluruh branding yang terlihat pengguna menjadi `Xspace` pada UI, metadata, MFA, billing, serta email verifikasi/reset; menambahkan Open Graph/Twitter social card 1200×630 agar tautan WhatsApp menampilkan thumbnail logo.
- [x] Implementasi sidebar, topbar, dan responsive navigation.
- [x] Menjadikan sidebar workspace persisten sebagai layout bersama pada Meetings, Calendar, Chat, Rooms, Drive, People, Recordings, Admin, dan Settings.
- [x] Menambahkan theme switcher global di header untuk Dark Green dan Light, tersimpan per browser serta tersedia pada desktop dan mobile.
- [x] Menyempurnakan Light theme Overview dengan workspace background yang lembut, header/search putih, cards dan panels terang, kontras teks yang jelas, aksen hijau tenang, serta state hover/modal/toast yang konsisten; sidebar gelap dipertahankan sebagai brand anchor.
- [x] Menyamakan Light theme Calendar, Chat, Rooms, Drive, dan People dengan Overview, termasuk forms, side panels, conversation states, composer, file browser, profile drawer, dialog, hover, dan tampilan mobile.
- [x] Membatasi section MANAGE, Admin, Settings, Recordings, dan Plan & usage pada sidebar hanya untuk TENANT_ADMIN dan SUPER_ADMIN berdasarkan sesi aktif.
- [x] Mengaktifkan halaman Meetings, People, dan Recordings dengan data tenant nyata, pencarian directory, join/create meeting, serta secure recording download.
- [x] Implementasi autentikasi email/username + password.
- [x] Implementasi secure session, logout, dan audit login/logout.
- [x] Implementasi role dasar: super admin, tenant admin, host, co-host, member, guest.
- [x] Implementasi user profile dan tenant-aware data model.
- [x] Memindahkan profil pengguna ke header kanan, menambahkan dropdown account, editor profile, serta upload/remove foto privat JPEG/PNG/WebP maksimal 2 MB dengan audit event.

## 3. Xpace Meet — MVP User Journey

- [x] Implementasi dashboard: New Meeting, Join, Schedule, upcoming, recent, recordings.
- [x] Menghapus data dummy Overview dan menghubungkan next/upcoming meeting, live room, directory member, recording count/duration, serta recent recordings ke API tenant nyata.
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
- [x] Menampilkan foto profil peserta pada panel live meeting dengan fallback inisial, menambahkan alert admit yang selalu terlihat oleh host saat waiting-room request masuk, serta membuat join idempotent dengan unique active-participant guard agar refresh tidak menduplikasi nama peserta.
- [x] Implementasi meeting chat.
- [x] Implementasi meeting lock, mute/remove participant, dan promote co-host.
- [x] Implementasi reconnect, loading, permission-denied, dan meeting-ended states.
- [x] Implementasi meeting history.
- [x] Menambahkan server-side pagination pada halaman Meetings, pilihan jumlah baris, status/date/entry metadata, action kanan Open/Copy/Delete, konfirmasi delete, role guard host/admin, soft-delete, dan audit event; meeting ACTIVE memakai aksi aman End & delete yang memutus room untuk semua peserta terlebih dahulu.

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
- [x] Menambahkan permanent identity deletion pada user management: wajib deaktifkan akun aktif terlebih dahulu, konfirmasi email, cabut sesi/token, hapus profil/avatar, lepas email dan username agar dapat digunakan kembali, pertahankan riwayat bersama sebagai `Deleted user`, serta lindungi akun sendiri dan SUPER_ADMIN.
- [x] Implementasi group management dasar.
- [x] Memperbaiki production Group management `could not load groups` dengan JSON member aggregation yang kompatibel pgx, validasi hasil query, dan regression test untuk group kosong/beranggota.
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
- [x] Melakukan hard cutover production ke `xspace.cankonix.com` dan `livekit.xspace.cankonix.com`, memperbarui web/API/LiveKit/TURN serta URL kanonis, menerbitkan TLS tepercaya, menonaktifkan router domain lama, dan meluluskan production smoke test pada 27 Agustus 2026.

## 7. MVP Stabilization & Acceptance Gate (Aktif)

Tahap ini wajib selesai sebelum pengembangan modul post-MVP dimulai.

### 7.1 User onboarding

- [x] Memisahkan alur `Create active user` dan `Invite user` pada Admin → Users.
- [x] Merapikan layout Admin → Users di dalam sidebar persisten serta menyediakan lifecycle action Deactivate/Cancel invitation/Reactivate dengan session revocation dan audit-history retention.
- [x] Menambahkan field password dan retype password untuk pembuatan user aktif.
- [x] Menambahkan show/hide password, validasi minimal 8 karakter, dan validasi kedua password harus sama.
- [x] Menambahkan tooltip dan petunjuk format pada signup untuk Workspace URL, Username, dan password; validasi username tanpa spasi/`@`; serta ikon mata independen pada password dan retype password.
- [x] Memastikan password hanya diproses sebagai secret dan disimpan sebagai hash Argon2id, tidak pernah dicatat atau ditampilkan kembali.
- [x] Menambahkan alur invitation acceptance dan set-password sebelum akun `INVITED` dapat diaktifkan.
- [x] Menulis unit/integration test untuk create user, duplicate identity, password mismatch, activation, dan login user baru.
- [x] Menguji pembuatan akun `MEMBER` dari production admin hingga berhasil login dari perangkat kedua.

### 7.2 Mobile-responsive meeting experience

- [x] Mendesain ulang live meeting untuk mobile setara pola penggunaan Zoom/Google Meet tanpa menyalin tampilan produk lain.
- [x] Membuat header mobile ringkas dan memindahkan meeting code serta secondary actions ke menu `More`.
- [x] Membuat control bar bawah yang sticky, mendukung safe area iPhone, dan memiliki touch target minimal 44×44 px.
- [x] Memastikan tombol mic, camera, speaker, screen share, participants, more, dan leave tidak terpotong pada layar kecil.
- [x] Membuat video grid adaptif untuk single/multiple participant, portrait, landscape, dan rotasi layar.
- [x] Mengubah participant panel menjadi bottom sheet atau full-screen drawer pada mobile.
- [x] Memindahkan network metrics dan adaptive-network demo ke panel diagnostics yang dapat dibuka-tutup pada mobile.
- [x] Menyempurnakan prejoin dan waiting room agar preview, device selector, permission state, dan tombol join tidak overflow.
- [x] Memperbaiki live meeting iPhone: meeting code di header dapat ditekan untuk menyalin kode penuh, menu More menyediakan aksi `Copy meeting code`, popup menutup saat tap di area luar atau menekan Escape, dan seluruh target sentuh tetap minimal 44 px; lint, TypeScript, build, accessibility, serta responsive source audit lulus 30 Agustus 2026.
- [ ] Menguji iPhone Safari, Android Chrome, tablet portrait/landscape, reconnect, dan perubahan orientasi.

### 7.3 MVP end-to-end acceptance

- [x] Menjalankan uji meeting production dengan dua akun dan dua perangkat/jaringan berbeda.
- [x] Memvalidasi lifecycle meeting production secara lengkap.
  - [x] Create/join meeting.
  - [x] Waiting room serta admit/reject.
  - [x] Audio/video.
  - [x] Participant notification.
  - [x] Chat meeting/workspace memakai bubble kanan-kiri bergaya WhatsApp Desktop; composer workspace memiliki emoji picker di samping attachment.
  - [x] Screen sharing.
  - [x] Moderation host/co-host: integration acceptance lock/unlock, promote co-host, remove participant, dan penolakan token ulang setelah remove lulus.
  - [x] Recording worker dan upload production lulus; file MP4 production terverifikasi ada di object storage. Library memakai endpoint recording mandiri, merekonsiliasi status/ukuran dari egress serta object storage (termasuk egress lama), menyediakan player video kecil, Download MP4, dan Delete. Polling status tombol Record sudah memakai properti JSON yang benar. Automated production acceptance create → record → finalize MP4 → signed HTTPS download → delete object lulus 28 Agustus 2026; acceptance pengguna untuk Play, Download MP4, dan Delete dikonfirmasi lulus 28 Agustus 2026.
  - [x] Leave dan end-for-all.
- [x] Menginvestigasi recording production: egress selesai tanpa error; akar masalah library adalah ketergantungan pada 10 meeting pertama dan status egress yang dapat tertinggal saat room tutup. Endpoint library mandiri dan rekonsiliasi status diterapkan sebelum list/start.
- [ ] Menyelesaikan browser/device compatibility dan responsive UI yang masih berstatus parsial pada bagian 6.
- [ ] Menyelesaikan accessibility keyboard, ARIA, focus, contrast, touch target, dan non-color status yang masih berstatus parsial pada bagian 6.
- [x] Mendokumentasikan hasil acceptance, known limitations, dan keputusan MVP release-ready.

## 8. Post-MVP Phase 1 — Collaboration Suite

- [x] Mengembangkan Xpace Chat penuh: direct message, group/channel, thread, attachment, search, presence, dan notification.
- [x] Mengembangkan Xpace Rooms sebagai ruang kerja persisten untuk tim, member, meeting, chat, dan file.
- [x] Mengembangkan Xpace Drive: upload, folder, preview, sharing, permission, versioning, quota, dan audit.
- [x] Mengembangkan Xpace Calendar: event, recurring meeting, invitation, reminder, timezone, dan integrasi meeting.
- [x] Menyatukan pencarian, notification, activity, dan permission lintas Meet, Chat, Rooms, Drive, dan Calendar.
  - [x] Global Search tenant-scoped di header untuk Meetings, People, Chat, Rooms, Drive, Calendar, dan Recordings, dengan permission filtering per pengguna, debounce, keyboard `Ctrl/⌘ K`, navigasi panah/Enter/Escape, click-outside, grouped results, responsive mobile overlay, dan deep-link ke modul tujuan. Unit backend dan production build lulus 29 Agustus 2026.
  - [x] Unified Activity Feed tenant-scoped di Overview untuk Meeting, Chat, Rooms, Drive, Calendar, dan Recordings, dengan resource-level permission filtering, cursor pagination, filter kategori, relative time, responsive light/dark UI, dan deep-link ke resource asal. Unit endpoint, production build/deployment, health check, log audit, dan query permission terhadap data production lulus 29 Agustus 2026.

## 9. Post-MVP Phase 2 — Enterprise Identity, Security & Operations

- [ ] Mengimplementasikan SSO (OIDC/SAML), MFA, LDAP/AD, SCIM/provisioning, dan advanced RBAC.
- [~] Mengimplementasikan retention policy dan legal hold; data export, compliance control lanjutan, dan approval workflow masih terbuka.
- [~] Mengintegrasikan SIEM, advanced audit, alerting, security analytics, dan incident workflow.
  - [x] Prometheus mengirim alert firing/resolved ke Alertmanager 0.28.1 private; grouping, deduplication, repeat interval, SMTP production, recipient operasional, runtime secret `0600`, health, koneksi upstream, dan monitor dry-run acceptance tervalidasi 29 Agustus 2026.
  - [x] Incident workflow tenant-scoped menyediakan case P1–P4, source, assignee, acknowledgement, investigation, resolution/close/reopen, note timeline, RBAC `incident.manage`, dan immutable admin audit; halaman `/admin/incidents` responsif aktif di production, migration berhasil, serta rollback-based lifecycle acceptance (`ACKNOWLEDGED|P3|1`) lulus 29 Agustus 2026 tanpa meninggalkan data dummy.
  - [x] Alertmanager mengirim webhook bearer-authenticated ke API internal dan secara idempotent membuat/update incident berdasarkan fingerprint; severity Prometheus dipetakan ke P1–P4, firing yang berulang tidak menduplikasi case, dan resolved memperbarui status/timeline. Secret independen telah dirotasi, token API/Alertmanager cocok, unauthenticated request ditolak `401`, acceptance pascarotasi menghasilkan `RESOLVED|P1|2`, monitor lulus, dan seluruh data uji dibersihkan pada 30 Agustus 2026.
  - [x] Automated acknowledgement escalation aktif dengan SLA P1 5 menit, P2 15 menit, P3 1 jam, dan P4 4 jam; worker memakai advisory lock dan transaksi atomik per insiden, lalu membuat timeline, immutable audit, notifikasi internal admin, dan antrean email terdeduplikasi. Production worker aktif dan rollback acceptance (`1|1|3|3`) lulus 30 Agustus 2026 tanpa mengirim email atau menyimpan data uji.
  - [ ] Menjalankan controlled end-to-end notification drill setelah memperoleh persetujuan penerima email operasional.
  - [ ] Mengintegrasikan event keamanan dan insiden ke SIEM eksternal setelah vendor/endpoint, format, autentikasi, dan kebijakan retensi ditentukan.
- [~] Menyiapkan backup/restore teruji dan runbook DR; offsite scheduling, continuous recovery, dan high availability masih terbuka.
- [x] Menambahkan advanced observability untuk application, database, media, storage, dan tenant usage.
  - [x] Admin Dashboard menampilkan telemetry tenant nyata untuk database pool/wait, active media rooms, joined/waiting participant, failed recording, client error 24 jam, Drive/chat/recording storage, Rooms, Calendar, dan chat activity 24 jam; unit test, production build, deployment, health check, serta production data-query acceptance lulus 29 Agustus 2026.
  - [x] Endpoint Prometheus mengekspor database pool/wait, active meeting, waiting/joined participant, failed recording, Drive/chat/recording storage, chat activity, client error, dan status kegagalan scrape; regression test, production deployment, serta scrape acceptance (`scrape_error=0`) lulus 29 Agustus 2026.
  - [x] Prometheus production menyimpan time-series selama 30 hari/10 GB, Grafana 12.4.3 memprovision datasource dan dashboard operasi 12-panel secara otomatis, serta 9 alert rule lintas application/database/media/recording/storage tervalidasi sehat pada 29 Agustus 2026. UI observability hanya terikat ke loopback server dan diakses melalui SSH tunnel.
  - [x] Monitor systemd per menit ikut memvalidasi Prometheus server, status target API, alert rules, dan health database Grafana; production dry-run acceptance lulus 29 Agustus 2026.

## 10. Post-MVP Phase 3 — AI & Intelligent Collaboration

- [ ] Mengimplementasikan live transcription dan speaker attribution.
- [ ] Mengimplementasikan meeting summary, action items, chapter, dan searchable transcript.
- [ ] Mengimplementasikan realtime caption translation dan post-meeting translation.
- [ ] Mengimplementasikan AI noise suppression dan audio enhancement dengan kontrol privasi yang jelas.
- [ ] Menambahkan governance untuk consent, retention, tenant policy, model usage, dan akses hasil AI.

## 11. Post-MVP Phase 4 — Scale, Events & Platform

- [ ] Menyiapkan multi-region deployment, regional routing, failover, dan data-residency policy.
- [ ] Mengembangkan webinar/events dengan registration, backstage, broadcast role, moderation, dan analytics.
- [ ] Mengembangkan public API, webhook, SDK, dan embedded communication.
- [ ] Menambahkan integration marketplace dan lifecycle aplikasi/integrasi tenant.
- [ ] Melakukan capacity planning, load test, chaos/failover test, dan production scale certification.

## 12. Next Task List — Post-MVP Execution Backlog

MVP dinyatakan cukup untuk tahap ini. Pengerjaan berikutnya mengikuti urutan prioritas di bawah.

Progress 2026-08-25: production DNS/TLS readiness, responsive route audit, dan accessibility source audit lulus. Acceptance dua perangkat masih menunggu environment/device test yang aktif.

### 12.1 Priority P0 — MVP follow-up dan product hardening

- [x] Menstabilkan accumulated post-MVP worktree: ESLint, TypeScript serial, production build 40 route, Go unit/race test, meeting lifecycle dan tenant-isolation integration, accessibility/responsive source audit, dependency scan, Compose validation, deployment preflight, serta `git diff --check` lulus 1 September 2026. Regresi participant-list cross-workspace `404` sebelum authorization diperbaiki menjadi penolakan `403`; bukti tersedia di `docs/worktree-stabilization-2026-09-01.md`.
- [x] Menutup acceptance test dua akun, dua perangkat, dan dua jaringan berbeda.
- [ ] Menyelesaikan matriks kompatibilitas iPhone Safari, Android Chrome, tablet, orientasi, dan reconnect.
- [~] Audit accessibility otomatis (focus source, ARIA, status, reduced motion, font, dan touch target) lulus dan masuk CI; audit diperluas ke workspace shell, global search, notification center, dan account menu. Font Platform 11 px serta cascading-render React pada Incidents, Drive, dan Global Search diperbaiki; accessibility/responsive source audit, ESLint, TypeScript, production build, seluruh public route, dan media permission header lulus 30 Agustus 2026. Keyboard, contrast, screen-reader, orientasi, dan reconnect manual lintas perangkat masih menunggu browser/device aktif.
- [x] Menambahkan automated integration smoke test CI untuk alur create → join → admit → meeting/token → leave → end.
- [x] Menambahkan tenant-scoped UI error tracking, recovery boundary, release identifier, dan error count 24 jam pada production Admin Dashboard.

### 12.2 Priority P1 — Xpace Chat

- [x] Mendesain model conversation, direct message, channel, thread, dan message permission.
- [x] Mengimplementasikan fondasi API conversation, membership tenant-scoped, daftar pesan, dan kirim pesan.
- [x] Membuat UI Chat dasar: daftar conversation, pembuatan channel, histori pesan, dan composer.
- [x] Menambahkan `New message` dengan pemilih user aktif workspace, direct-message reuse, nama lawan bicara, serta pembuatan channel dengan pemilihan banyak anggota dan validasi tenant/status.
- [x] Meredesain People sebagai professional workspace directory dengan card/list view, foto profil aman tenant-scoped, search/role filter, panel detail akun, contact/workspace metadata, responsive light/dark theme, dan action direct message.
- [x] Mengimplementasikan realtime messaging dengan unread count dan presence berbasis SSE/read-state/heartbeat.
- [x] Menambahkan reply/thread, reaction, edit, delete, mention context, dan pin message.
- [x] Attachment Chat: private object storage, tenant-scoped metadata, protected presigned download, upload API, dan UI composer sudah tersedia.
- [x] Menambahkan pencarian pesan tenant-scoped dan hasil navigasi kembali ke thread.
- [x] Menambahkan notification center untuk mention, reply, reaction, dan unread activity.
- [x] Mengaktifkan Global Notification Center pada header seluruh workspace dengan polling/focus refresh, badge unread, deep-link ke Chat, read individual, Mark all read, serta close-on-outside-click dan tombol Escape.
- [x] Menambahkan dropdown aksi pada setiap conversation card: Clear chat mengosongkan riwayat hanya untuk akun aktif, Delete conversation menyembunyikan card dan riwayat hanya untuk akun aktif, pesan baru dapat memunculkannya kembali, data anggota lain tidak terhapus, seluruh aksi diaudit; migration, unit test, deployment, schema check, dan production health lulus 29 Agustus 2026.

### 12.3 Priority P1 — Xpace Calendar dan Rooms

- [x] Mengimplementasikan calendar event, recurring event, timezone, reminder, dan invitation.
- [x] Menghubungkan calendar event dengan meeting code, waiting room, dan participant count/list context.
- [x] Persistent Xpace Rooms untuk tim dengan linked Chat, meeting creation, room-scoped Drive files, UI, dan activity timeline.
- [x] Menambahkan room membership, OWNER/ADMIN/MEMBER/GUEST role, private/tenant sharing model, dan activity timeline.

### 12.4 Priority P1 — Xpace Drive

- [x] Mengimplementasikan upload/download file dengan private signed URL pada Xpace Drive.
- [x] Folder, browser preview/download, rename, move, delete, sharing, VIEW/EDIT permission, dan UI management tersedia.
- [x] Version history/restore, quota 10 GB, retention 30 hari dengan cleanup enforcement, dan audit event tersedia.
- [x] Mendesain ulang Drive seperti Finder/Google Drive dengan sidebar file, breadcrumb, pencarian, sortir, tampilan list/grid, drag-and-drop, metadata yang jelas, action menu, dialog sharing/version history, dan layout mobile responsif.

### 12.5 Priority P2 — Enterprise identity dan governance

- [x] Menyeragamkan alignment halaman Security, Roles, SCIM, dan Governance di dalam workspace shell: konten tidak lagi terpusat sempit/terdorong ke kanan, memakai area kerja maksimal 1500 px dan padding responsif 16–36 px.
- [x] Mengimplementasikan MFA untuk admin dan akun berisiko tinggi.
  - [x] Enrollment TOTP, secret terenkripsi AES-GCM, verifikasi RFC 6238, dan delapan recovery code sekali pakai.
  - [x] Login challenge, setup/disable mandiri dengan verifikasi faktor kedua, audit event, dan halaman Security responsif.
  - [x] Unit test kriptografi/TOTP/recovery serta deployment produksi dan smoke test endpoint selesai 25 Agustus 2026.
- [~] Mengimplementasikan SSO OIDC terlebih dahulu, lalu SAML.
  - [x] OIDC authorization-code flow dengan PKCE S256, state sekali pakai, client secret terenkripsi, UserInfo identity mapping, JIT provisioning terbatas, audit, konfigurasi admin, dan tombol login sudah deployed.
  - [x] Endpoint HTTPS-only, redirect URI produksi, proteksi admin, unit test konfigurasi, dan smoke test kondisi SSO belum aktif selesai 25 Agustus 2026.
  - [ ] Menjalankan acceptance end-to-end dengan tenant/provider OIDC perusahaan nyata lalu mengimplementasikan SAML.
- [~] Menambahkan SCIM provisioning, advanced RBAC, dan session/device management.
  - [x] Session/device management: metadata IP/browser/platform, last-seen throttling, daftar sesi aktif, revoke satu sesi, sign out perangkat lain, audit event, dan UI Security responsif.
  - [x] Unit test device detection, migrasi produksi, protected-endpoint smoke test, serta API/web healthy selesai 25 Agustus 2026.
  - [x] Advanced RBAC dengan tenant custom roles, permission catalog, user assignment, effective permission resolution, privilege-escalation guard, audit, dan enforcement pada admin/meeting endpoints.
  - [x] Admin Roles UI responsif, unit test base/custom permissions, migrasi produksi, serta protected-endpoint smoke test selesai 25 Agustus 2026.
  - [~] Mengimplementasikan SCIM 2.0 provisioning untuk Users dan Groups.
    - [x] Tenant bearer token sekali tampil/rotate/disable, ServiceProviderConfig, SCIM error media type, tenant isolation, dan Admin SCIM UI.
    - [x] Users/Groups list, filter/pagination, create, get, replace, deactivate/delete, session revocation, group membership, dan audit event deployed 25 Agustus 2026.
    - [x] PATCH Users untuk active/userName/displayName/externalId serta PATCH Groups untuk displayName dan add/remove/replace members, termasuk filtered member removal.
    - [x] Unit test PATCH parsing, ServiceProviderConfig patch support, deployment API, health check, dan protected PATCH smoke test selesai 25 Agustus 2026.
    - [ ] Menjalankan acceptance end-to-end dengan tenant Microsoft Entra ID atau Okta nyata.
- [~] Menambahkan retention policy, legal hold, export data, dan approval workflow.
  - [x] Tenant retention policy untuk recording, Drive trash, chat, dan audit dengan validasi 1–3650 hari serta minimum audit 30 hari.
  - [x] Legal hold tenant-scoped untuk recording, Drive file, dan chat conversation; add/remove resource, release hold, audit event, dan proteksi retention.
  - [x] Eksekusi retention manual: redaksi chat kedaluwarsa, menyembunyikan recording kedaluwarsa, purge metadata/object Drive trash yang eligible, dan purge audit event sesuai policy.
  - [x] Admin Governance UI responsif, permission `governance.manage`, unit test validasi, migration production, serta protected-endpoint smoke test selesai 25 Agustus 2026.
  - [x] Scheduled retention worker 24 jam dengan initial delay terkonfigurasi, PostgreSQL advisory lock untuk multi-instance, isolation per tenant, dan audit event terjadwal.
  - [x] Physical object purge untuk recording dan chat attachment dengan recheck legal hold, transaction-level resource lock, metadata lifecycle recording, batch limit, dan deployment production selesai 25 Agustus 2026.
  - [x] Data export FULL/AUDIT/DIRECTORY dengan arsip JSONL gzip private, secret-field exclusion, SHA-256, signed URL lima menit, expiry tujuh hari, dan automatic object cleanup.
  - [x] Four-eyes approval workflow: requester tidak dapat self-approve, approve/reject membutuhkan admin berbeda, rejection reason, queued worker multi-instance, status lifecycle, audit event, dan UI responsif.
  - [x] Production acceptance worker: FULL archive 11.554 byte berhasil dibuat, checksum database/object cocok, password hash/object key tidak ikut export, expiry dan audit tervalidasi; artefak uji dibersihkan 25 Agustus 2026.
  - [ ] Menjalankan acceptance kebijakan akhir bersama tim legal/compliance dan mendokumentasikan sign-off organisasi.
- [~] Menambahkan backup/restore teruji, disaster recovery, RPO/RTO, dan high availability.
  - [x] Encrypted backup PostgreSQL custom dump + private MinIO bucket, per-file manifest, outer SHA-256, GPG AES-256, dan zero-downtime object copy dengan `--no-deps`.
  - [x] Non-destructive restore test ke database disposable, production restore dengan explicit confirmation, serta cleanup aman tersedia sebagai script.
  - [x] Production acceptance backup 39 MB: archive/object checksum valid, restore menghasilkan 1 tenant, 6 user, 42 tabel, database disposable terhapus, dan API tetap healthy pada 25 Agustus 2026.
  - [x] Runbook recovery dengan target paid-beta RPO 24 jam, RTO 2 jam, retention recommendation, verification, incident steps, dan post-restore smoke test.
  - [ ] Menjadwalkan encrypted backup harian ke storage offsite, alert freshness 26 jam, WAL/PITR, replicated database/object storage, dan failover/HA test.

### 12.6 Priority P2 — AI collaboration

- [ ] Mengimplementasikan consent-aware live transcription dan speaker attribution.
- [ ] Mengimplementasikan meeting summary, action items, chapter, dan searchable transcript.
- [ ] Menambahkan caption translation serta governance untuk retention dan akses hasil AI.

### 12.7 Priority P3 — Scale dan platform

- [x] Menambahkan advanced observability untuk API, database, media, storage, dan tenant usage; dashboard admin, collector Prometheus, retensi 30 hari, dashboard Grafana, dan alert threshold sudah aktif di production.
- [ ] Melakukan capacity planning, load test, chaos test, dan failover certification.
- [ ] Menyiapkan multi-region, regional routing, failover, dan data residency.
- [ ] Mengembangkan webinar/events, public API, webhook, SDK, embedded communication, dan integration marketplace.

## 13. SaaS Commercial Launch Gate

Sepuluh kelompok berikut adalah blocker menuju paid beta. Fitur AI, webinar, SDK, marketplace, dan multi-region bukan syarat penjualan pertama.

- [~] Acceptance meeting dua akun/perangkat/jaringan selesai; matriks browser mobile utama masih terbuka.
  - [x] Cross-workspace meeting dan external guest access: global join-code resolver, host-tenant policy/waiting room, meeting-scoped external identity, tenant-safe moderation, avatar, audit metadata, dan UI penanda workspace host; production acceptance dua workspace (login → preview → join/refresh → waiting room → admit → LiveKit token → leave) lulus 28 Agustus 2026.
  - [x] Live meeting camera-off fallback menampilkan profile picture asli sebagai thumbnail bulat di tengah tile (gaya Zoom) melalui metadata LiveKit dan endpoint avatar meeting-scoped; peserta tanpa profile picture tetap menggunakan siluet aman bawaan.
  - [x] Participant reconnect/leave hardening: identitas LiveKit stabil per user, rekonsiliasi peserta online terhadap status database, cleanup `JOINED` stale, leave waiting-room, compatibility identitas lama, dan deduplikasi defensif pada panel host.
- [x] Menyediakan self-service signup, pembuatan workspace/tenant, verifikasi email, password reset, dan onboarding owner.
  - [x] Signup membuat tenant dan owner `TENANT_ADMIN` berstatus `INVITED`; login diblokir sampai email terverifikasi.
  - [x] Token verifikasi/reset bersifat single-use, disimpan sebagai hash, memiliki expiry, dan payload outbox dienkripsi.
  - [x] Forgot/reset password tidak membocorkan keberadaan akun dan reset mencabut seluruh sesi lama.
  - [x] Persetujuan Terms/Privacy dicatat dengan versi, waktu, IP, dan user-agent.
  - [x] Acceptance lokal signup → verify → login → forgot → reset → session revocation lulus pada 25 Agustus 2026; data uji dibersihkan.
  - [x] SMTP Hostinger production aktif; delivery ke `info@cankonix.com`, verifikasi melalui inbox nyata, aktivasi akun, dan login production lulus pada 25 Agustus 2026; tenant uji dibersihkan.
- [~] Menambahkan plan catalog, entitlement, quota, trial, upgrade/downgrade, dan enforcement per tenant.
  - [x] Katalog `STARTER`, `BUSINESS`, dan `ENTERPRISE` beserta harga, trial, feature flags, dan limit tersimpan di database.
  - [x] Workspace baru otomatis memperoleh trial `STARTER` 14 hari; tenant existing dibackfill aman ke `BUSINESS/ACTIVE`.
  - [x] Usage real-time menghitung user, meeting bulanan, recording bulanan, serta gabungan storage Drive/chat/recording.
  - [x] Override entitlement per tenant mendukung feature flag dan limit khusus tanpa mengubah katalog global.
  - [x] Enforcement berlaku pada user admin, SCIM/OIDC auto-provision, meeting langsung/Calendar/Rooms, recording, Drive, dan attachment chat.
  - [x] Halaman admin Plan & usage serta public plan API tersedia; unit dan local acceptance quota lulus 26 Agustus 2026.
  - [ ] Upgrade/downgrade otomatis dan perubahan periode subscription menunggu webhook billing provider.
  - [x] Auto-end meeting berdasarkan effective `maxMeetingDurationMinutes` plan/override/kebijakan workspace: worker 30 detik memakai advisory lock multi-instance, menutup LiveKit room, mengakhiri participant, mencatat audit, dan retry closure yang idempotent. Unit test, migration `0040`, deployment, serta production acceptance berhasil menutup 3 meeting Starter lama pada batas 60 menit dengan `pending_room_close=0` tanggal 29 Agustus 2026.
- [~] Mengintegrasikan billing provider, checkout, subscription lifecycle, invoice, webhook idempotent, dunning, dan cancellation.
  - [x] Menstandarkan CTA Contact sales, enterprise inquiry, dan plan change ke `info@cankonix.com`.
  - [x] Skema checkout session, invoice, event webhook idempotent, dan riwayat transisi subscription tersedia pada migrasi `0033`.
  - [x] Endpoint webhook provider-neutral memverifikasi signature HMAC, menolak event ID yang dipakai ulang dengan payload berbeda, dan menerapkan subscription/invoice secara atomic.
  - [x] Lifecycle `ACTIVE`, `PAST_DUE`, `CANCELED`, dan `SUSPENDED` serta event invoice pending/paid/failed/void/refund telah dipetakan ke entitlement tenant.
  - [x] Admin dapat melihat invoice serta menjadwalkan atau membatalkan cancellation dari halaman Plan & usage.
  - [x] Unit test signature, validasi event, dan transaksi invoice-paid lulus 26 Agustus 2026.
  - [x] Migrasi `0033`, secret webhook, API/web, health check, dan proteksi invalid-signature telah diverifikasi di production pada 26 Agustus 2026.
  - [~] Adapter Xendit Payment Sessions (`SUBSCRIPTION`, hosted payment link, IDR, retry schedule) dan mapping webhook native session/plan/cycle telah diimplementasikan; aktivasi production menunggu Secret API Key dan Webhook Verification Token dari dashboard Xendit.
  - [x] Cancellation provider-managed menghentikan siklus berikutnya melalui endpoint deactivate Xendit, mempertahankan akses sampai akhir periode terbayar, dan tidak menawarkan resume palsu untuk plan yang sudah inactive.
  - [x] Adapter Xendit telah dideploy ke production dalam mode terkunci; health/log bersih dan endpoint native mengembalikan `503` sampai credential resmi dikonfigurasi pada 26 Agustus 2026.
  - [ ] Dunning otomatis, retry pembayaran, email billing, credit note, pajak, dan rekonsiliasi payout masih terbuka.
- [x] Membuat SaaS super-admin untuk tenant lifecycle, suspension, support access terkontrol, dan usage overview lintas tenant.
  - [x] Dashboard `/platform` khusus `SUPER_ADMIN`: overview workspace/user/meeting/storage, pencarian, status filter, pagination, serta detail usage dan subscription tenant.
  - [x] Status operasional tenant dipisahkan dari billing; suspend mencabut sesi tenant tanpa mengubah invoice/subscription, reactivate tidak memulihkan sesi lama, home tenant platform dilindungi, dan login tenant suspended diblokir.
  - [x] Support access read-only memakai alasan wajib, expiry 15–120 menit, revoke manual, directory view terbatas, serta audit untuk start/view/revoke.
  - [x] Migrasi `0037`, role isolation, unit test, route production, dan API/web health lulus 28 Agustus 2026; dua workspace existing tetap `ACTIVE` setelah migrasi.
- [x] Menyiapkan pricing/landing/signup publik serta transactional email untuk onboarding, invitation, billing, dan security notice.
  - [x] Landing/pricing publik `/pricing` responsif dengan metadata SEO/Open Graph, manfaat platform/security, katalog paket real-time dari public plan API, CTA trial ke signup, CTA Enterprise ke `info@cankonix.com`, serta navigasi dari login.
  - [x] Signup publik, verifikasi email, dan reset password menggunakan SMTP production dan transactional outbox.
  - [x] Mengirim invitation langsung melalui transactional outbox (dengan link manual sebagai fallback), billing notice yang idempotent per webhook, serta security notice untuk reset password dan perubahan MFA; semua template memiliki versi teks + HTML bermerek dan retry melalui SMTP worker.
- [~] Menyelesaikan backup/restore production teruji, encrypted offsite backup, runbook DR, serta target RPO/RTO; fondasi dan restore acceptance selesai, offsite schedule/PITR masih terbuka.
- [x] Menutup security/operations launch gate: monitoring-alerting, vulnerability/dependency review, load test dasar, dan incident/support runbook.
  - [x] Monitor independen systemd berjalan setiap menit untuk public web, API/PostgreSQL readiness, API metrics, Prometheus health/target/rules, Alertmanager readiness, Grafana health, dan LiveKit TLS; alert SMTP memakai tiga kegagalan beruntun, cooldown 30 menit, serta recovery notice. Timer production aktif dan dry-run acceptance terbaru lulus 29 Agustus 2026.
  - [x] Dependency gate mencakup Dependabot, npm audit, govulncheck, Trivy image scan, dan PR dependency review. Audit aktual: npm 0 vulnerability, Go 0 reachable vulnerability; `cel-go` diperbarui ke versi perbaikan dan residual `openpgp` yang tidak reachable dicatat di `docs/security-review-2026-08-29.md`.
  - [x] Pentest hardening 30 Agustus 2026: source/attack-surface review, negative HTTP/session/CORS/method tests, TLS/listener/container review, race test, dependency audit, dan OWASP ZAP passive baseline selesai. Potensi SSRF OIDC (High), rate-limit berbasis proxy (Medium), dan disclosure `X-Powered-By` (Low) telah diperbaiki/deploy; scan ulang menghasilkan 61 PASS, 0 FAIL, 6 warning rendah/informasional. Laporan tersedia di `docs/pentest-2026-08-30.md`.
  - [ ] Menjalankan pentest authenticated multi-tenant BOLA/IDOR dengan akun attacker/victim terpisah untuk Tenant Admin, Member, Guest, external meeting, dan Platform Admin; kemudian memperoleh pentest independen sebelum enterprise general availability.
  - [ ] Menghapus CSP `unsafe-inline` memakai nonce/hash yang kompatibel dengan Next.js dan menguji hardening container `no-new-privileges`, capability drop, serta read-only root filesystem tanpa mengganggu meeting/recording.
  - [x] Basic production load test read-only lulus 200 request, concurrency 10, 0% failure, p95 13 ms dengan threshold maksimal 1% failure dan p95 1000 ms.
  - [x] Severity, response target, ownership, recovery, security escalation, customer support, dan post-incident review tersedia di `docs/incident-response.md`.
- [~] Menerbitkan Terms of Service, Privacy Policy, DPA/cookie/recording consent yang sesuai pasar penjualan dan memperoleh sign-off pemilik bisnis.
  - [x] Legal Center publik `/legal` beserta Terms, Privacy, DPA, Cookie Notice, dan Meeting Recording Notice versi draft `2026-08-29` responsif, memiliki metadata/canonical, serta ditautkan dari signup, login, dan pricing.
  - [x] Signup tetap mencatat versi Terms/Privacy, waktu, IP, dan user-agent; versi konfigurasi diselaraskan ke `2026-08-29`.
  - [x] Prejoin meminta acknowledgement recording notice; join API menolak acknowledgement kosong/versi lama dan mencatat versi yang diterima pada audit event workspace host.
  - [x] Baseline regulasi Indonesia dan keputusan legal yang masih wajib dilengkapi didokumentasikan di `docs/legal-readiness.md`.
  - [x] Deployment production dan acceptance selesai 29 Agustus 2026: enam route Legal Center HTTP 200, signup menautkan Terms/Privacy, API dan web healthy, monitor satu menit lulus, serta audit responsif/header production lulus.
  - [ ] Konfirmasi identitas/badan hukum dan alamat Cankonix, subprocessor/lokasi data, venue/liability/commercial terms, prosedur hak privasi, lalu isi sign-off pemilik bisnis dan reviewer legal sebelum label DRAFT dilepas.
- [ ] Menjalankan paid-beta release rehearsal: signup → checkout → tenant aktif → meeting → usage → invoice → cancellation/restore support.
