# Frontend API Guide - Package-Based Pricing

## Overview

The internship application API has been updated to use a **package-based pricing model** instead of the old department/specialization/period pricing table.

### Key Changes

**Old Model:**
- Used `department`, `specialization`, and `duration` to look up prices from a pricing table
- Pricing was stored in a separate `prices` table

**New Model:**
- Applications select a `package_id` from a predefined list of training packages
- Packages contain: name, description, skills, price, currency, billing period
- Amount is calculated as: `package price × duration_months`
- Duration is tracked on the application, not the package
- Historical fields (`department`, `specialization`) are preserved but optional

---

## Package Endpoints

### GET /packages - List All Packages
**Authentication:** None (public)

**Query Parameters:**
- `id` (optional): Get single package by ID
- `page` (optional): Page number (default: 1)
- `limit` (optional): Items per page (default: 10, max: 100)

**Response (list):**
```json
{
  "status": 200,
  "message": "Packages retrieved",
  "data": [
    {
      "id": 1,
      "name": "Basic Training Package",
      "description": "Coding and Design (Figma)",
      "price": 150000,
      "skills": ["coding", "design"],
      "period": "monthly",
      "currency": "RWF",
      "is_active": true
    },
    {
      "id": 2,
      "name": "Standard Training Package",
      "description": "Coding, Design (Figma), 3D Printing, and CNC",
      "price": 250000,
      "skills": ["coding", "design", "3d-printing", "cnc"],
      "period": "monthly",
      "currency": "RWF",
      "is_active": true
    },
    {
      "id": 3,
      "name": "Premium Training Package",
      "description": "Coding, Design (Figma), 3D Printing, CNC, and IoT (Embedded System)",
      "price": 300000,
      "skills": ["coding", "design", "3d-printing", "cnc", "iot"],
      "period": "monthly",
      "currency": "RWF",
      "is_active": true
    }
  ],
  "meta": {
    "total": 3,
    "page": 1,
    "limit": 10,
    "total_pages": 1
  }
}
```

**Response (single package with ?id=):**
```json
{
  "status": 200,
  "message": "Package retrieved",
  "data": {
    "id": 1,
    "name": "Basic Training Package",
    "description": "Coding and Design (Figma)",
    "price": 150000,
    "skills": ["coding", "design"],
    "period": "monthly",
    "currency": "RWF",
    "is_active": true
  }
}
```

### POST /packages - Create Package (Admin Only)
**Authentication:** Required (JWT + admin role)

**Request Body:**
```json
{
  "name": "New Training Package",
  "description": "Package description",
  "price": 200000,
  "skills": ["coding", "design", "iot"],
  "period": "monthly",
  "currency": "RWF",
  "is_active": true
}
```

**Validation:**
- `name`: Required, max 100 chars, case-insensitive unique (active packages only)
- `price`: Required, must be >= 0
- `skills`: Required, array of strings
- `period`: Required, one of: monthly, yearly, quarterly
- `currency`: Optional, defaults to "RWF"
- `is_active`: Optional, defaults to true

**Response:**
```json
{
  "status": 200,
  "message": "Package created",
  "data": {
    "id": 4,
    "name": "New Training Package",
    "description": "Package description",
    "price": 200000,
    "skills": ["coding", "design", "iot"],
    "period": "monthly",
    "currency": "RWF",
    "is_active": true
  }
}
```

### PUT /packages/:id - Update Package (Admin Only)
**Authentication:** Required (JWT + admin role)

**Request Body:**
```json
{
  "name": "Updated Package Name",
  "description": "Updated description",
  "price": 250000,
  "skills": ["coding", "design", "iot", "3d-printing"],
  "period": "monthly",
  "currency": "RWF",
  "is_active": true
}
```

**Response:**
```json
{
  "status": 200,
  "message": "Package updated",
  "data": {
    "id": 1,
    "name": "Updated Package Name",
    "description": "Updated description",
    "price": 250000,
    "skills": ["coding", "design", "iot", "3d-printing"],
    "period": "monthly",
    "currency": "RWF",
    "is_active": true
  }
}
```

### DELETE /packages/:id - Delete Package (Admin Only)
**Authentication:** Required (JWT + admin role)

**Response:**
```json
{
  "status": 200,
  "message": "Package deleted",
  "data": {
    "undo_token": "abc123def456..."
  }
}
```

**Note:** This is a soft delete. The package is not permanently deleted. An `undo_token` is returned for restoration.

### POST /packages/restore - Restore Package (Admin Only)
**Authentication:** Required (JWT + admin role)

**Request Body:**
```json
{
  "undo_token": "abc123def456..."
}
```

**Response:**
```json
{
  "status": 200,
  "message": "Package restored",
  "data": {
    "id": 1,
    "name": "Updated Package Name",
    "description": "Updated description",
    "price": 250000,
    "skills": ["coding", "design", "iot", "3d-printing"],
    "period": "monthly",
    "currency": "RWF",
    "is_active": true
  }
}
```

---

## Application Endpoints

### POST /applications - Create Application
**Authentication:** Optional (guests can apply without auth)

**Request Body (Guest - no auth):**
```json
{
  "full_name": "Guest Applicant",
  "email": "guest@example.com",
  "phone": "+250788777666",
  "package_id": 1,
  "duration_months": 3,
  "department": "software",
  "specialization": "fullstack",
  "motivation": "I want to learn software development",
  "experience": "I have some programming experience",
  "start_date": "2025-01-15",
  "education_level": "university",
  "session": "morning",
  "work_mode": "hybrid",
  "document_ids": ["uuid1", "uuid2"],
  "guest_token": "optional_token_for_documents"
}
```

**Request Body (Authenticated user):**
```json
{
  "package_id": 1,
  "duration_months": 3,
  "department": "software",
  "specialization": "fullstack",
  "motivation": "I want to learn software development",
  "experience": "I have some programming experience",
  "start_date": "2025-01-15",
  "education_level": "university",
  "session": "morning",
  "work_mode": "hybrid",
  "document_ids": ["uuid1", "uuid2"]
}
```

**Required Fields:**
- `package_id`: Integer, min 1
- `duration_months`: Integer, 1-24
- `motivation`: String, max 2000 chars
- `experience`: String, max 2000 chars
- `education_level`: "university" or "high_school"
- `session`: "morning" or "afternoon"
- `work_mode`: "onsite", "remote", or "hybrid"

**Guest-Only Fields:**
- `full_name`: Required when not logged in
- `email`: Required when not logged in
- `phone`: Required when not logged in
- `guest_token`: Required if submitting documents as guest

**Optional Historical Fields:**
- `department`: Optional, defaults to "General"
- `specialization`: Optional, defaults to "General"

**Response:**
```json
{
  "status": 200,
  "message": "Application created",
  "data": {
    "id": "c52915ca-55dd-43b9-a55d-9d87ef1a12c4",
    "reference": "APP-1795713331102122045",
    "applicant": {
      "id": "b686f1a2-f0e7-4a13-b3a7-dcedc9b25962",
      "full_name": "Test Applicant",
      "email": "testapplicant@example.com",
      "phone": "+250788999888",
      "country": "",
      "city": "",
      "institution": "",
      "field_of_study": "",
      "qualification": "",
      "academic_status": "",
      "graduation_year": null,
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
    "motivation": "I want to learn software development",
    "experience": "I have some programming experience",
    "education_level": "university",
    "session": "morning",
    "duration_months": 3,
    "work_mode": "hybrid",
    "amount": 450000,
    "skills": ["coding", "design"],
    "status": "submitted",
    "submitted_at": "2026-11-26T17:15:31Z",
    "documents": [],
    "history": [
      {
        "status": "submitted",
        "at": "2026-11-26T17:15:31Z",
        "note": "Application received."
      }
    ],
    "notes": []
  }
}
```

**Amount Calculation:**
- `amount = package.price × duration_months`
- Example: 150,000 RWF/month × 3 months = 450,000 RWF

### GET /applications/mine - Get My Applications
**Authentication:** Required (JWT)

**Response:**
```json
{
  "status": 200,
  "message": "Applications retrieved",
  "data": [
    {
      "id": "c52915ca-55dd-43b9-a55d-9d87ef1a12c4",
      "reference": "APP-1795713331102122045",
      "package_id": 1,
      "package_name": "Basic Training Package",
      "amount": 450000,
      "skills": ["coding", "design"],
      "status": "submitted",
      ...
    }
  ],
  "meta": {
    "total": 1,
    "page": 1,
    "limit": 10,
    "total_pages": 1
  }
}
```

### GET /applications/:id - Get Single Application
**Authentication:** Required (JWT)

**Response:** Same as POST response above

### GET /applications - Get All Applications (Admin)
**Authentication:** Required (JWT + admin role)

**Query Parameters:**
- `page`: Page number (default: 1)
- `limit`: Items per page (default: 10)
- `status`: Filter by status (submitted, under_review, shortlisted, interview, accepted, rejected)
- `package_id`: Filter by package ID
- `search`: Search by reference or package name
- `submitted_from`: Date range start
- `submitted_to`: Date range end

**Response:**
```json
{
  "status": 200,
  "message": "Applications retrieved",
  "data": [...],
  "summary": {
    "total": 100,
    "submitted": 50,
    "under_review": 20,
    "shortlisted": 10,
    "interview": 5,
    "accepted": 10,
    "rejected": 5
  },
  "by_package": [
    {
      "package_id": 1,
      "package_name": "Basic Training Package",
      "count": 40
    }
  ],
  "trends": [
    {
      "month": "2026-01",
      "count": 25
    }
  ]
}
```

### PATCH /applications/:id/status - Update Status (Admin)
**Authentication:** Required (JWT + admin role)

**Request Body:**
```json
{
  "status": "under_review",
  "note": "Application looks promising",
  "interview_at": "2026-05-01T09:00:00Z"
}
```

**Valid Statuses:**
- `submitted`
- `under_review`
- `shortlisted`
- `interview`
- `accepted`
- `rejected`

**Response:**
```json
{
  "status": 200,
  "message": "Status updated",
  "data": {
    "id": "c52915ca-55dd-43b9-a55d-9d87ef1a12c4",
    "status": "under_review",
    ...
  }
}
```

### POST /applications/:id/notes - Add Review Note (Admin)
**Authentication:** Required (JWT + admin role)

**Request Body:**
```json
{
  "text": "This applicant has strong technical skills"
}
```

**Response:**
```json
{
  "status": 200,
  "message": "Note added",
  "data": {
    "id": "note-uuid",
    "author": "Admin User",
    "text": "This applicant has strong technical skills",
    "at": "2026-11-26T17:20:00Z"
  }
}
```

---

## Frontend Implementation Guide

### 1. Package Selection Flow

**Step 1: Fetch available packages**
```javascript
const response = await fetch('http://localhost:8080/packages');
const { data: packages } = await response.json();
```

**Step 2: Display packages to user**
- Show package name, description, price, skills
- Calculate total cost: `package.price × userSelectedDuration`

**Step 3: User selects package and duration**
```javascript
const selectedPackage = packages.find(p => p.id === selectedPackageId);
const totalAmount = selectedPackage.price * durationMonths;
```

**Step 4: Submit application with package_id**
```javascript
const response = await fetch('http://localhost:8080/applications', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    package_id: selectedPackageId,
    duration_months: durationMonths,
    // ... other fields
  })
});
```

### 2. Displaying Application Details

When displaying an application, show:
- Package name
- Package skills
- Duration (months)
- Total amount (calculated)
- Historical fields (department, specialization) if available

### 3. Pricing Display

Show pricing to users:
- Per-month price: `package.price RWF/month`
- Total cost: `package.price × duration_months RWF`

Example:
- Basic Package: 150,000 RWF/month
- For 3 months: 450,000 RWF total

### 4. Form Validation

**Required fields:**
- `package_id` must be selected
- `duration_months` must be 1-24
- `motivation` and `experience` required
- `education_level`, `session`, `work_mode` required

**Guest form:**
- Add `full_name`, `email`, `phone` when not authenticated

### 5. Error Handling

**Common errors:**
- `409 Conflict`: Duplicate package name (admin create)
- `404 Not Found`: Package not found / application not found
- `400 Bad Request`: Validation errors
- `401 Unauthorized`: Missing or invalid token
- `403 Forbidden`: Not admin for admin-only routes

---

## Migration Notes

### What Changed

**Before:**
- Used `department`, `specialization`, `duration` for pricing
- Looked up price from `prices` table
- Period was stored on pricing record

**After:**
- Uses `package_id` to select training package
- Price comes from `packages` table
- Duration is on application (not package)
- Period is on package (billing frequency)
- Historical fields preserved for backward compatibility

### Data Preservation

- All existing application data is preserved
- Historical `department` and `specialization` fields remain
- Can be sent optionally in requests
- Default to "General" if not provided

---

## Example cURL Commands

### List packages
```bash
curl http://localhost:8080/packages
```

### Get single package
```bash
curl "http://localhost:8080/packages?id=1"
```

### Create application (guest)
```bash
curl -X POST http://localhost:8080/applications \
  -H "Content-Type: application/json" \
  -d '{
    "full_name": "Guest Applicant",
    "email": "guest@example.com",
    "phone": "+250788777666",
    "package_id": 1,
    "duration_months": 3,
    "motivation": "I want to learn",
    "experience": "My experience",
    "education_level": "university",
    "session": "morning",
    "work_mode": "hybrid"
  }'
```

### Create application (authenticated)
```bash
curl -X POST http://localhost:8080/applications \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "package_id": 1,
    "duration_months": 3,
    "motivation": "I want to learn",
    "experience": "My experience",
    "education_level": "university",
    "session": "morning",
    "work_mode": "hybrid"
  }'
```

---

## Support

For questions or issues, contact the backend team.
