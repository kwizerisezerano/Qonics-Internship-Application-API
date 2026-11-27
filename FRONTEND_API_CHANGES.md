# Frontend API Changes - Package-Based Pricing Model

This document describes all API changes for the new package-based pricing model. Frontend developers should update their applications to use these new endpoints and request/response formats.

---

## Table of Contents
1. [Package Endpoints](#package-endpoints)
2. [Settings Endpoints](#settings-endpoints)
3. [Application Endpoints](#application-endpoints)
4. [Data Model Changes](#data-model-changes)

---

## Package Endpoints

### 1. GET /packages (List All Packages)
**Authentication:** None (public)

**Request:**
```bash
GET /packages
```

**Response:**
```json
{
  "data": [
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
    },
    {
      "id": 2,
      "name": "Standard Training Package",
      "description": "Coding, Design (Figma), 3D Printing, and CNC",
      "price": 250000,
      "skills": ["coding", "design", "3d-printing", "cnc"],
      "period": "monthly",
      "currency": "RWF",
      "is_active": true,
      "minimum_duration_period": 1
    },
    {
      "id": 3,
      "name": "Premium Training Package",
      "description": "Coding, Design (Figma), 3D Printing, CNC, and IoT (Embedded System)",
      "price": 300000,
      "skills": ["coding", "design", "3d-printing", "cnc", "iot"],
      "period": "monthly",
      "currency": "RWF",
      "is_active": true,
      "minimum_duration_period": 6
    }
  ],
  "message": "Packages retrieved",
  "meta": {
    "current": 1,
    "next": null,
    "pages": 1,
    "per_page": 10,
    "prev": null,
    "total": 3
  },
  "status": 200
}
```

---

### 2. GET /packages?id=1 (Get Single Package)
**Authentication:** None (public)

**Request:**
```bash
GET /packages?id=1
```

**Response:**
```json
{
  "data": {
    "id": 1,
    "name": "Basic Training Package",
    "description": "Coding and Design (Figma)",
    "price": 150000,
    "skills": ["coding", "design"],
    "period": "monthly",
    "currency": "RWF",
    "is_active": true,
    "minimum_duration_period": 1
  },
  "message": "Package retrieved",
  "status": 200
}
```

---

### 3. POST /packages (Create Package - Admin Only)
**Authentication:** Required (Admin role)

**Request:**
```bash
POST /packages
Authorization: Bearer <admin_token>
Content-Type: application/json

{
  "name": "Custom Package",
  "description": "Custom training package description",
  "price": 200000,
  "skills": ["coding", "design", "3d-printing"],
  "period": "monthly",
  "currency": "RWF",
  "minimum_duration_period": 3
}
```

**Request Fields:**
- `name` (string, required): Package name
- `description` (string, required): Package description
- `price` (number, required): Price per period (must be >= 0)
- `skills` (array of strings, required): List of skills included
- `period` (string, required): Billing period - "monthly", "yearly", or "quarterly"
- `currency` (string, optional): Currency code (default: "RWF")
- `minimum_duration_period` (integer, optional): Minimum months required (default: 1)

**Response:**
```json
{
  "data": {
    "id": 4,
    "name": "Custom Package",
    "description": "Custom training package description",
    "price": 200000,
    "skills": ["coding", "design", "3d-printing"],
    "period": "monthly",
    "currency": "RWF",
    "is_active": true,
    "minimum_duration_period": 3
  },
  "message": "Package created",
  "status": 200
}
```

---

### 4. PUT /packages/:id (Update Package - Admin Only)
**Authentication:** Required (Admin role)

**Request:**
```bash
PUT /packages/4
Authorization: Bearer <admin_token>
Content-Type: application/json

{
  "name": "Updated Package Name",
  "description": "Updated description",
  "price": 250000,
  "skills": ["coding", "design", "3d-printing", "cnc"],
  "period": "monthly",
  "currency": "RWF",
  "minimum_duration_period": 2
}
```

**Response:**
```json
{
  "data": {
    "id": 4,
    "name": "Updated Package Name",
    "description": "Updated description",
    "price": 250000,
    "skills": ["coding", "design", "3d-printing", "cnc"],
    "period": "monthly",
    "currency": "RWF",
    "is_active": true,
    "minimum_duration_period": 2
  },
  "message": "Package updated",
  "status": 200
}
```

---

### 5. DELETE /packages/:id (Delete Package - Admin Only)
**Authentication:** Required (Admin role)

**Request:**
```bash
DELETE /packages/4
Authorization: Bearer <admin_token>
```

**Response:**
```json
{
  "data": {
    "undo_token": "a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6"
  },
  "message": "Package deleted",
  "status": 200
}
```

**Note:** Save the `undo_token` to restore the package.

---

### 6. POST /packages/restore (Restore Package - Admin Only)
**Authentication:** Required (Admin role)

**Request:**
```bash
POST /packages/restore
Authorization: Bearer <admin_token>
Content-Type: application/json

{
  "undo_token": "a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6"
}
```

**Response:**
```json
{
  "data": {
    "id": 4,
    "name": "Updated Package Name",
    "description": "Updated description",
    "price": 250000,
    "skills": ["coding", "design", "3d-printing", "cnc"],
    "period": "monthly",
    "currency": "RWF",
    "is_active": true,
    "minimum_duration_period": 2
  },
  "message": "Package restored",
  "status": 200
}
```

---

## Settings Endpoints

### 7. GET /settings/public (Public Settings)
**Authentication:** None (public)

**Request:**
```bash
GET /settings/public
```

**Response:**
```json
{
  "data": {
    "applications_open": true,
    "close_on": "",
    "closure_message": "",
    "morning_seats": 3,
    "afternoon_seats": 3
  },
  "message": "Settings retrieved",
  "status": 200
}
```

**Response Fields:**
- `applications_open` (boolean): Whether applications are currently accepting submissions
- `close_on` (string): Date when applications will close (empty if not set)
- `closure_message` (string): Message shown when applications are closed
- `morning_seats` (integer): Available seats for morning session
- `afternoon_seats` (integer): Available seats for afternoon session

---

### 8. PUT /settings (Update Settings - Admin Only)
**Authentication:** Required (Admin role)

**Request:**
```bash
PUT /settings
Authorization: Bearer <admin_token>
Content-Type: application/json

{
  "applications_open": true,
  "close_on": "",
  "closure_message": "",
  "morning_seats": 5,
  "afternoon_seats": 4
}
```

**Request Fields:**
- `applications_open` (boolean): Enable/disable applications
- `close_on` (string, optional): Close date in ISO 8601 format (e.g., "2026-12-31T23:59:59Z")
- `closure_message` (string, optional): Custom closure message
- `morning_seats` (integer): Number of seats for morning session (default: 3)
- `afternoon_seats` (integer): Number of seats for afternoon session (default: 3)

**Response:**
```json
{
  "data": {
    "applications_open": true,
    "close_on": "",
    "closure_message": "",
    "morning_seats": 5,
    "afternoon_seats": 4
  },
  "message": "Settings updated",
  "status": 200
}
```

---

## Application Endpoints

### 9. POST /applications (Create Application)
**Authentication:** Optional (Guest or Authenticated)

#### Authenticated Request:
```bash
POST /applications
Authorization: Bearer <applicant_token>
Content-Type: application/json

{
  "package_id": 1,
  "duration_months": 3,
  "motivation": "I want to learn coding and design",
  "experience": "1 year of self-study",
  "education_level": "university",
  "session": "morning",
  "work_mode": "hybrid",
  "agreed_to_terms": true,
  "start_date": "2099-02-01T00:00:00Z"
}
```

#### Guest Request:
```bash
POST /applications
Content-Type: application/json

{
  "package_id": 1,
  "duration_months": 3,
  "motivation": "I want to learn",
  "experience": "1 year",
  "education_level": "high_school",
  "session": "afternoon",
  "work_mode": "hybrid",
  "agreed_to_terms": true,
  "start_date": "2099-03-01T00:00:00Z",
  "full_name": "Guest User",
  "email": "guest@example.com",
  "phone": "+250788123456"
}
```

**Request Fields:**
- `package_id` (integer, required): Selected package ID
- `duration_months` (integer, required): Duration in months (must be >= package.minimum_duration_period)
- `motivation` (string, required): Motivation for applying
- `experience` (string, required): Relevant experience
- `education_level` (string, required): Education level - "university", "high_school", or "other"
- `session` (string, required): Session - "morning" or "afternoon"
- `work_mode` (string, required): Work mode - "onsite" or "hybrid" (remote removed)
- `agreed_to_terms` (boolean, required): Must be true to submit
- `start_date` (string, required): Start date in ISO 8601 format
- `full_name` (string, required for guests): Full name
- `email` (string, required for guests): Email address
- `phone` (string, required for guests): Phone number
- `document_ids` (array of strings, optional): IDs of uploaded documents
- `guest_token` (string, optional): Token for guest-uploaded documents

**Response:**
```json
{
  "data": {
    "id": "24774358-f9e6-46a1-9b9d-0fd0bf54f66a",
    "reference": "APP-1795775904006337882",
    "applicant": {
      "id": "2e340311-2054-4d96-ac39-2cff8c79fb2f",
      "full_name": "Test User",
      "email": "testuser@example.com",
      "phone": "+250788999999",
      "country": "Rwanda",
      "city": "Kigali",
      "institution": "University of Rwanda",
      "field_of_study": "Computer Science",
      "qualification": "Bachelor",
      "academic_status": "final year",
      "graduation_year": 2026,
      "skills": [],
      "bio": "",
      "github_url": "",
      "linkedin_url": "",
      "portfolio_url": ""
    },
    "package_id": 1,
    "package_name": "Basic Training Package",
    "department": "General",
    "specialization": "General",
    "motivation": "I want to learn coding and design",
    "experience": "1 year of self-study",
    "start_date": "2099-02-01T00:00:00Z",
    "education_level": "university",
    "session": "morning",
    "duration_months": 3,
    "work_mode": "hybrid",
    "amount": 450000,
    "skills": ["coding", "design"],
    "agreed_to_terms": false,
    "status": "submitted",
    "submitted_at": "2026-11-27T10:38:24Z",
    "documents": [],
    "history": [
      {
        "status": "submitted",
        "at": "2026-11-27T10:38:24Z",
        "note": "Application received."
      }
    ],
    "notes": []
  },
  "message": "Application created",
  "status": 200
}
```

**New Response Fields:**
- `package_id` (integer): Selected package ID
- `package_name` (string): Name of the selected package
- `amount` (number): Calculated amount (package.price × duration_months)
- `skills` (array): Skills from the selected package
- `duration_months` (integer): Duration in months

**Removed Fields:**
- `department` (replaced by package)
- `specialization` (replaced by package)
- `duration` (replaced by duration_months)

---

## Data Model Changes

### Removed Fields:
- `department` (from application request)
- `specialization` (from application request)
- `duration` (from application request)
- `remote` (from work_mode options)
- `bachelor`, `master`, `phd`, `diploma`, `certificate` (from education_level options)

### Added Fields:
- `package_id` (to application request)
- `duration_months` (to application request)
- `agreed_to_terms` (to application request)
- `minimum_duration_period` (to package model)
- `morning_seats` (to settings)
- `afternoon_seats` (to settings)

### Validation Rules:
- `education_level`: Only accepts "university", "high_school", "other"
- `work_mode`: Only accepts "onsite", "hybrid" (remote removed)
- `agreed_to_terms`: Must be true to submit application
- `duration_months`: Must be >= package.minimum_duration_period
- Seat capacity: On-site/hybrid interns limited by session seat count

---

## Error Responses

### Duration Below Minimum:
```json
{
  "message": "Duration must be at least 6 months for this package",
  "status": 400
}
```

### Session Full (Seat Capacity):
```json
{
  "message": "That session has no free seats for your dates. Choose another start date or session, or apply in hybrid mode.",
  "status": 409
}
```

### Terms Not Agreed:
```json
{
  "message": "You must agree to the terms and conditions to submit an application",
  "status": 400
}
```

### Invalid Education Level:
```json
{
  "message": "education_level must be one of university high_school other",
  "status": 400
}
```

### Invalid Work Mode:
```json
{
  "message": "work_mode must be one of onsite hybrid",
  "status": 400
}
```

---

## Implementation Checklist

### Application Form:
- [ ] Replace department/specialization dropdowns with package selection
- [ ] Add duration_months input field
- [ ] Add agreed_to_terms checkbox
- [ ] Update education_level options (remove bachelor, master, phd, diploma, certificate)
- [ ] Update work_mode options (remove remote)
- [ ] Display package details (name, description, price, skills, minimum duration)
- [ ] Calculate and display amount (price × duration)
- [ ] Validate duration >= package minimum_duration_period
- [ ] Update application response handling to show new fields

### Admin Settings Page:
- [ ] Add morning_seats input field
- [ ] Add afternoon_seats input field
- [ ] Update PUT /settings request body to include seat fields

### Admin Packages Page:
- [ ] Create package list page (GET /packages)
- [ ] Create package creation form (POST /packages)
- [ ] Create package update form (PUT /packages/:id)
- [ ] Add delete functionality with undo token (DELETE /packages/:id)
- [ ] Add restore functionality (POST /packages/restore)
- [ ] Display minimum_duration_period in all package views

### Public Settings Display:
- [ ] Display morning_seats and afternoon_seats from GET /settings/public
- [ ] Show session availability to applicants

---

## Example Integration Code

### Fetch Packages:
```javascript
const fetchPackages = async () => {
  const response = await fetch('/packages');
  const data = await response.json();
  return data.data; // Array of packages
};
```

### Calculate Amount:
```javascript
const calculateAmount = (package, durationMonths) => {
  return package.price * durationMonths;
};
```

### Submit Application:
```javascript
const submitApplication = async (formData) => {
  const response = await fetch('/applications', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token}`
    },
    body: JSON.stringify({
      package_id: formData.packageId,
      duration_months: formData.durationMonths,
      motivation: formData.motivation,
      experience: formData.experience,
      education_level: formData.educationLevel,
      session: formData.session,
      work_mode: formData.workMode,
      agreed_to_terms: formData.agreedToTerms,
      start_date: formData.startDate
    })
  });
  return response.json();
};
```

### Update Settings:
```javascript
const updateSettings = async (settings) => {
  const response = await fetch('/settings', {
    method: 'PUT',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${adminToken}`
    },
    body: JSON.stringify({
      applications_open: settings.applicationsOpen,
      close_on: settings.closeOn,
      closure_message: settings.closureMessage,
      morning_seats: settings.morningSeats,
      afternoon_seats: settings.afternoonSeats
    })
  });
  return response.json();
};
```
