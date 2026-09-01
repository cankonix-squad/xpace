# Xspace legal readiness review

Status: **implementation draft — business/legal approval required before final publication**
Draft version: `2026-08-29`

## Implemented

- Public Legal Center: Terms of Service, Privacy Policy, Data Processing Addendum, Cookie Notice, and Meeting Recording Notice.
- Signup links directly to the versioned Terms and Privacy Policy; acceptance continues to record version, timestamp, IP address, and user agent.
- Login and pricing footers link to legal documents.
- Prejoin requires an explicit recording-notice acknowledgement. The API rejects missing or stale versions and records the current version in the host-workspace audit event.
- Terms, Privacy, and recording-consent versions have one deployable configuration source.

## Indonesian regulatory basis reviewed

- [Law No. 27 of 2022 on Personal Data Protection](https://peraturan.bpk.go.id/Details/229798/uu-no-27-tahun-2022) — the official JDIH BPK summary covers data-subject rights, personal-data processing, controller and processor duties, transfers, administrative sanctions, disputes, and offences; status shown as in force.
- [Government Regulation No. 71 of 2019 on Electronic Systems and Transactions](https://peraturan.bpk.go.id/Details/122030/pp-no-71-tahun-2019) — official JDIH BPK record; status shown as in force.

These sources establish the initial Indonesian baseline. Qualified counsel must assess the complete facts, customer market, employment/recording context, cross-border processing, sector rules, and any later implementing regulation before sign-off.

## Required owner/legal decisions

1. Confirm the exact contracting legal entity, registration details, registered address, and authorized signatory. “Cankonix Technology” is currently a product/business name in the draft.
2. Approve governing law, court/arbitration venue, warranty language, indemnities, liability cap, tax treatment, renewal/refund rules, and enterprise order precedence.
3. Publish the named subprocessor list, processing purpose, data category, hosting/processing location, and advance-change process.
4. Confirm hosting/data-residency claims, backup deletion window, default retention periods, breach-notification channel, and contractual notification target.
5. Confirm whether Xspace will sell only business plans or also to consumers; consumer protection wording may differ.
6. Approve the procedure for privacy rights, identity verification, complaints, regulator contact, and DPO/contact designation if required.
7. Confirm employee, minor, health, biometric, confidential, and cross-border meeting/recording rules for target customers.
8. Record approval date, approver, approved version, and publication date in `tasklist.md`; remove every visible “DRAFT” notice only after approval.

## Sign-off record

| Role | Name | Decision | Date | Version |
| --- | --- | --- | --- | --- |
| Business owner | Pending | Pending | — | 2026-08-29 |
| Legal/privacy reviewer | Pending | Pending | — | 2026-08-29 |
| Security/operations owner | Pending | Pending | — | 2026-08-29 |
