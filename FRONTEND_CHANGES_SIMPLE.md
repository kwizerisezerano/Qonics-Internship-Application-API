# Frontend Changes - Simple Summary

## What Changed in the API

### 1. Application Form - Fields Changed

**REMOVED:**
- `department` dropdown
- `specialization` dropdown
- `duration` field
- `remote` option from work_mode
- `bachelor`, `master`, `phd`, `diploma`, `certificate` from education_level

**ADDED:**
- `package_id` (dropdown to select package)
- `duration_months` (input field for months)
- `agreed_to_terms` (checkbox, must be checked)

### 2. New Package Endpoints

**GET /packages** - List all packages (public)
```json
[
  {
    "id": 1,
    "name": "Basic Training Package",
    "description": "Coding and Design (Figma)",
    "price": 150000,
    "skills": ["coding", "design"],
    "period": "monthly",
    "currency": "RWF",
    "is_active": true,
    "minimum_duration_period": 1
  }
]
```

**GET /packages?id=1** - Get single package (public)
```json
{
  "id": 1,
  "name": "Basic Training Package",
  "description": "Coding and Design (Figma)",
  "price": 150000,
  "skills": ["coding", "design"],
  "period": "monthly",
  "currency": "RWF",
  "is_active": true,
  "minimum_duration_period": 1
}
```

### 3. Application Request - New Format

**POST /applications**
```json
{
  "package_id": 1,
  "duration_months": 3,
  "motivation": "I want to learn",
  "experience": "1 year",
  "education_level": "university",
  "session": "morning",
  "work_mode": "hybrid",
  "agreed_to_terms": true,
  "start_date": "2099-02-01T00:00:00Z"
}
```

**For guests (add these fields):**
```json
{
  "full_name": "Guest User",
  "email": "guest@example.com",
  "phone": "+250788123456"
}
```

### 4. Application Response - New Fields

**Response now includes:**
```json
{
  "package_id": 1,
  "package_name": "Basic Training Package",
  "duration_months": 3,
  "amount": 450000,
  "skills": ["coding", "design"]
}
```

**Amount is calculated automatically:** `package.price × duration_months`

### 5. Settings - New Fields

**GET /settings/public** (now includes seat capacity)
```json
{
  "applications_open": true,
  "morning_seats": 3,
  "afternoon_seats": 3
}
```

**PUT /settings** (admin - can update seat capacity)
```json
{
  "applications_open": true,
  "morning_seats": 5,
  "afternoon_seats": 4
}
```

## Validation Rules

- `education_level`: only "university", "high_school", "other"
- `work_mode`: only "onsite", "hybrid" (no remote)
- `agreed_to_terms`: must be true
- `duration_months`: must be >= package.minimum_duration_period

## Error Messages

**Duration too short:**
```
"Duration must be at least 6 months for this package"
```

**Session full:**
```
"That session has no free seats for your dates"
```

**Terms not agreed:**
```
"You must agree to the terms and conditions to submit an application"
```

## Admin Package Management (New)

**POST /packages** - Create package (admin)
**PUT /packages/:id** - Update package (admin)
**DELETE /packages/:id** - Delete package (admin, returns undo token)
**POST /packages/restore** - Restore package (admin, uses undo token)

## What You Need to Do

1. **Application Form:**
   - Replace department/specialization with package dropdown
   - Add duration_months input
   - Add agreed_to_terms checkbox
   - Remove "remote" from work_mode
   - Remove extra education levels
   - Display package details and calculate amount

2. **Admin Settings:**
   - Add morning_seats and afternoon_seats inputs

3. **Admin Packages:**
   - Build package CRUD pages
   - Display minimum_duration_period

4. **Public Settings:**
   - Display seat capacity (morning_seats, afternoon_seats)
