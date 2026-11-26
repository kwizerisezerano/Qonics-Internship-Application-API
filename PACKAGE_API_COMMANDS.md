# Package API Commands

## Prerequisites
- API should be running at `http://localhost:8080`
- You need a valid JWT token from the auth service for admin operations
- Replace `YOUR_JWT_TOKEN` with your actual token

## Create Packages (Admin Only)

### Package 1: Basic Training Package (150,000 RWF/month)
```bash
curl -X POST http://localhost:8080/packages \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Basic Training Package",
    "description": "Coding and Design (Figma)",
    "price": 150000,
    "skills": ["coding", "design"],
    "period": "monthly",
    "currency": "RWF",
    "is_active": true
  }'
```

### Package 2: Standard Training Package (250,000 RWF/month)
```bash
curl -X POST http://localhost:8080/packages \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Standard Training Package",
    "description": "Coding, Design (Figma), 3D Printing, and CNC",
    "price": 250000,
    "skills": ["coding", "design", "3d-printing", "cnc"],
    "period": "monthly",
    "currency": "RWF",
    "is_active": true
  }'
```

### Package 3: Premium Training Package (300,000 RWF/month)
```bash
curl -X POST http://localhost:8080/packages \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Premium Training Package",
    "description": "Coding, Design (Figma), 3D Printing, CNC, and IoT (Embedded System)",
    "price": 300000,
    "skills": ["coding", "design", "3d-printing", "cnc", "iot"],
    "period": "monthly",
    "currency": "RWF",
    "is_active": true
  }'
```

## Other Package Operations

### List All Packages (Public)
```bash
curl -X GET http://localhost:8080/packages
```

### Get Single Package by ID (Public)
```bash
curl -X GET "http://localhost:8080/packages?id=1"
```

### Update Package (Admin)
```bash
curl -X PUT "http://localhost:8080/packages?id=1" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Updated Package Name",
    "description": "Updated description",
    "price": 200000,
    "skills": ["coding", "design"],
    "period": "monthly",
    "currency": "RWF",
    "is_active": true
  }'
```

### Delete Package (Admin - Soft Delete)
```bash
curl -X DELETE "http://localhost:8080/packages?id=1" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"
```

### Restore Deleted Package (Admin)
```bash
curl -X POST "http://localhost:8080/packages/restore?id=1" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"
```

## Notes
- Package IDs are passed as query parameters (`?id=1`), not path parameters
- `skills` is an array of strings, not a comma-separated string
- `period` indicates billing frequency (monthly/yearly/quarterly), not duration
- Duration is tracked on the application table, not the package table
- Delete operations are soft deletes - packages can be restored by admins
- Only admin users can create, update, delete, or restore packages
- Public users can list and view active packages
