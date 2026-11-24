# Applying without logging in: frontend guide

Visitors can now submit an internship application **without an account**, and can attach documents. Their account is created automatically from the email, phone and name they give, and they get an email to set a password so they can open the portal later.

Everything below goes through the gateway (locally `http://localhost:8088`, **no `/api/v1` prefix**). All responses use the same envelope: `{ "status": 200, "message": "...", "data": ... }`. Errors are `{ "status": 4xx, "message": "..." }`.

Logged-in applicants are **unchanged**: same endpoints, same payloads, with their token.

---

## 1. The rule that matters most

**For a guest, send no `Authorization` header at all.**

- No header → treated as a guest.
- A header that is present but invalid or expired → `401`. It is never treated as a guest.

So if your API wrapper attaches a token from the store or a cookie, make sure it sends nothing for the guest calls below, or a stale token will turn every guest request into a 401.

---

## 2. Guest flow at a glance

```
1. (optional) upload documents       POST /files/guest/presigned-url     -> guest_token + file_id + presigned_url
                                      PUT  <presigned_url>                 (the file bytes, via your /api/r2 proxy)
                                      POST /files/guest/confirm-upload     { file_id, guest_token }
2. submit the application            POST /applications                   contact details + form + document_ids + guest_token
3. show "check your email"           the guest sets a password from the emailed link, then logs in
```

Documents are optional. With no documents, skip step 1 and send neither `document_ids` nor `guest_token`.

---

## 3. Uploading documents as a guest

### 3.1 Get an upload URL: `POST /files/guest/presigned-url`

Public (no token). Body:

```json
{
  "file_name": "cv.pdf",
  "file_type": "cv",
  "file_size": 123456,
  "guest_token": "optional on the first call, required on later calls"
}
```

- `file_type`: one of `cv`, `transcript`, `diploma`, `certificates`, `cover_letter`
- `file_name` must end in `.pdf`, `.doc` or `.docx`
- `file_size`: bytes, 1 to 5,242,880 (5 MB)
- `guest_token`: omit on the **first** upload. The response returns one. Send it on every later upload.

Response `data`:

```json
{
  "presigned_url": "https://...",
  "file_id": "uuid",
  "object_key": "uploads/....pdf",
  "expires_in": 900,
  "guest_token": "32 hex characters"
}
```

### 3.2 Upload the bytes

`PUT` the file to `presigned_url`. Keep using your existing same-origin `/api/r2?url=...` proxy exactly as you do for logged-in uploads. Nothing changes here.

### 3.3 Confirm: `POST /files/guest/confirm-upload`

Public (no token). Body:

```json
{ "file_id": "uuid", "guest_token": "the token from 3.1" }
```

Do this for every file after its PUT succeeds. An unconfirmed file cannot be attached to an application.

### 3.4 Important: do not upload the files in parallel at first

The **first** presigned-url call creates the guest token. If you fire all the uploads at once with `Promise.all`, each call creates its **own** token, and the application can only claim files that belong to one token.

Do this instead:

```ts
let token = ''
const fileIds: string[] = []

for (const [type, file] of files) {            // one at a time
  const slot = await api('/files/guest/presigned-url', {
    method: 'POST',
    body: { file_name: file.name, file_type: type, file_size: file.size, ...(token && { guest_token: token }) },
  })
  token ||= slot.guest_token                   // first response sets the token

  // PUT the bytes (same as today, through /api/r2)

  await api('/files/guest/confirm-upload', {
    method: 'POST',
    body: { file_id: slot.file_id, guest_token: token },
  })
  fileIds.push(slot.file_id)
}
```

(Once you have the token from the first file, the remaining files may be uploaded in parallel if you prefer.)

### 3.5 Limits and expiry

- **Maximum 5 documents** per guest token (`400` after that).
- Uploaded but unclaimed files are **deleted after 24 hours**.
- If the application submit fails (for example the validation error), the files are still valid. **Keep the same `guest_token` and `file_id`s and retry**. Do not re-upload.
- A file can be attached to **one** application only. Reusing it returns `400 "Invalid or unconfirmed document"`.

---

## 4. Submitting the application: `POST /applications`

Public (no token for guests). Same body as today, plus the guest fields:

```json
{
  "full_name": "Jane Doe",
  "email": "jane@example.com",
  "phone": "+250788123456",

  "department": "software",
  "specialization": "backend",
  "motivation": "...",
  "experience": "...",
  "start_date": "2026-12-15T00:00:00Z",
  "education_level": "university",
  "session": "morning",
  "duration_months": 3,
  "work_mode": "remote",

  "document_ids": ["file-id-1", "file-id-2"],
  "guest_token": "the token from section 3"
}
```

| Field | Guest | Notes |
|---|---|---|
| `full_name` | **required** | Letters, spaces, hyphens and apostrophes only. **Digits are rejected.** Max 150 characters. |
| `email` | **required** | Valid email. Lowercased server-side. |
| `phone` | **required** | Same rule as registration, for example `+250788123456`. Max 30 characters. |
| `document_ids` | optional | Only the confirmed ids from section 3. |
| `guest_token` | required **if** `document_ids` is sent | Must be the token those files were uploaded with. |

If a logged-in user sends these contact fields they are **ignored**. The signed-in account is used.

Success (`200`): `data` is the created application, with `data.documents` listing the attached files.

---

## 5. Error handling

| Status | When | What to show |
|---|---|---|
| `400` | Validation (name, email, phone, enums), missing contact details, bad/missing `guest_token`, bad or reused document, over 5 files, wrong file extension | Show `message`. Field problems name the field, for example `full_name contains unsupported characters`. |
| `401` | A token was sent but is invalid or expired | Send **no** `Authorization` header for guests (section 1). |
| `403` | Applications are closed | Show `message` and switch to your existing "closed" state. |
| `409` | See below | See below |
| `429` | Rate limit | Ask them to wait a minute. |

**`409` has three meanings. Tell them apart by `message`:**

- `"An account with this email already exists. Please log in to apply."` → show it with a **link to the login page**. A guest cannot apply under someone else's account.
- `"You already have an application for ... (APP-...)"` → duplicate application for the same track.
- `"That session has no free seats ..."` → seat limit. Ask them to pick another start date or session, or go remote.

**Rate limits (per IP):** guest upload endpoints 15 per minute, `POST /applications` 10 per minute.

---

## 6. After a successful guest submit

- **No token is returned and the user is not logged in.** Do not try to log them in or call logged-in endpoints.
- `/applicant/applications/:id` needs a login, so don't link to it for guests.
- Show something like: *"Application submitted. Reference APP-.... Check your email for a link to set your password so you can follow your application."*
- The email links to your existing `/auth/reset-password?token=...` page. **No change needed there.** The link is valid for **7 days**.
- If the link expires, they can use the normal "Forgot password" flow with the same email.

---

## 7. Other 401s you may be seeing when logged out

These are not part of the guest flow itself, but they cause errors on the apply page when nobody is logged in:

- **`GET /applications/mine`** requires a login. Don't call it for guests. The current `availability()` uses it to build seat windows. For guests use the public **`GET /sessions/bookings`** instead (`?page=1&limit=100`). It returns bookings grouped by `start_date` / `end_date` / `session` with `available_seats` (capacity 6). The server enforces the real capacity on submit either way.
- **`GET /settings`** requires a login. Use **`GET /settings/public`** when logged out. (`stores/settings.ts` already does. If you still see a 401 on `/settings`, another page or the layout is calling it.)
- Don't fetch the applicant profile or notifications for guests.

---

## 8. Known limitation

A guest account is created with **only name, email and phone**. The profile fields reviewers see (institution, field of study, skills and so on) are empty until the person logs in and completes their profile in the portal. The "complete your profile first" gate on the apply page must be skipped for guests. If we want those fields collected on the guest form, tell the backend and they can be added to the same request.

---

## 9. Full curl reference

Replace `BASE` with the gateway (`http://localhost:8088` locally). Every call below is public: **send no `Authorization` header**. In Apidog use Import → cURL, and put `BASE`, `GUEST_TOKEN` and `FILE_ID` in environment variables.

### 9.1 Before the form (public data)

```bash
# Is intake open?  -> data.applications_open, data.close_on, data.closure_message
curl 'BASE/settings/public'

# Seat availability (replaces GET /applications/mine for guests)
curl 'BASE/sessions/bookings?page=1&limit=100'
```

### 9.2 Upload a document

```bash
# a) first file: no guest_token. Response data: presigned_url, file_id, guest_token
curl -X POST 'BASE/files/guest/presigned-url' \
  -H 'Content-Type: application/json' \
  -d '{"file_name":"cv.pdf","file_type":"cv","file_size":123456}'

# b) send the bytes to the presigned_url from (a)
curl -X PUT 'PRESIGNED_URL' -H 'Content-Type: application/pdf' --data-binary '@cv.pdf'

# c) confirm (required, or the file can't be attached)
curl -X POST 'BASE/files/guest/confirm-upload' \
  -H 'Content-Type: application/json' \
  -d '{"file_id":"FILE_ID","guest_token":"GUEST_TOKEN"}'

# d) every further file: same steps, but send the SAME guest_token
curl -X POST 'BASE/files/guest/presigned-url' \
  -H 'Content-Type: application/json' \
  -d '{"file_name":"transcript.pdf","file_type":"transcript","file_size":98765,"guest_token":"GUEST_TOKEN"}'
```

`file_type`: `cv`, `transcript`, `diploma`, `certificates`, `cover_letter`. Files: pdf/doc/docx, max 5 MB, max 5 files per token.

### 9.3 Submit the application

```bash
# with documents
curl -X POST 'BASE/applications' \
  -H 'Content-Type: application/json' \
  -d '{
    "full_name":"Jane Doe",
    "email":"jane@example.com",
    "phone":"+250788123456",
    "department":"software",
    "specialization":"backend",
    "motivation":"I want to learn",
    "experience":"2 years",
    "start_date":"2026-12-15T00:00:00Z",
    "education_level":"university",
    "session":"morning",
    "duration_months":3,
    "work_mode":"remote",
    "document_ids":["FILE_ID","FILE_ID_2"],
    "guest_token":"GUEST_TOKEN"
  }'
```

Without documents: the same body with no `document_ids` and no `guest_token`.
Expected: `200 "Application created"`. `data.documents` lists the attached files.

### 9.4 After submitting: the guest sets a password

The guest gets an email with a link to `/auth/reset-password?token=...`. That page calls:

```bash
curl -X POST 'BASE/auth/reset-password' \
  -H 'Content-Type: application/json' \
  -d '{"token":"TOKEN_FROM_EMAIL","password":"Str0ng!Passw0rd"}'
```

Then they log in:

```bash
curl -X POST 'BASE/auth/login' \
  -H 'Content-Type: application/json' \
  -d '{"email":"jane@example.com","password":"Str0ng!Passw0rd"}'
```

If the link expired (7 days):

```bash
curl -X POST 'BASE/auth/forgot-password' \
  -H 'Content-Type: application/json' \
  -d '{"email":"jane@example.com"}'
```

### 9.5 Error cases to test (use a new email each time)

| Test | How | Expected |
|---|---|---|
| Email already registered | Submit 9.3 twice with the same email | `409` "An account with this email already exists. Please log in to apply." |
| Missing contact details | 9.3 without `full_name`, `email` and `phone` | `400` |
| Name with digits | `"full_name":"Jane 2"` | `400` "full_name contains unsupported characters" |
| Bad phone | `"phone":"abc"` | `400` |
| Documents without token | 9.3 with `document_ids` but no `guest_token` | `400` "guest_token is required to attach documents" |
| Someone else's / reused file | Submit 9.3 twice with the same `FILE_ID` and different emails | `400` "Invalid or unconfirmed document" |
| Unconfirmed file | Skip step 9.2(c), then submit | `400` "Invalid or unconfirmed document" |
| More than 5 files | Request a 6th presigned URL with the same token | `400` "You can upload at most 5 documents..." |
| Wrong file type | `"file_name":"virus.exe"` | `400` "Only PDF, DOC and DOCX files can be uploaded" |
| File too large | `"file_size":6000000` | `400` |
| Bad token header | `-H 'Authorization: Bearer abc'` on 9.3 | `401` (never a guest) |
| Applications closed | Admin closes intake, then submit | `403` with the admin's message |
| Rate limit | More than 10 submits or 15 uploads in a minute from one IP | `429` |

Interactive API docs for `POST /applications` are in `openapi.yaml`.

## 10. Prices by course and period

Prices belong to a course (specialization) and a period (`duration_months`). The price is the total for that period. A combination an admin has not priced costs 200000 RWF x months.

- `GET /prices/quote?specialization=backend&duration_months=3` (public): `{"price":600000,"currency":"RWF","is_default":true}`. Use it to show the price on the form.
- `GET /prices?page=1&limit=10&department=software` (public): the admin-set prices, paginated.
- `PUT /prices` (admin): `{"department":"software","specialization":"backend","duration_months":3,"price":450000,"currency":"RWF"}`. Sending the same course and period again replaces the price.
- `DELETE /prices/{id}` (admin): that course and period go back to the default.

`POST /applications` does not take an amount. The backend calculates it from the same price and returns it as `amount`.

Suggested text for the on-site morning session:
"On-site, morning session (09:00 - 11:00): this is a paid session. The fee includes a coffee break."
