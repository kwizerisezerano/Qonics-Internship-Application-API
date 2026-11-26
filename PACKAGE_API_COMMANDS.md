# Package API Commands

## Prerequisites
- API should be running at `http://localhost:8080`
- You need a valid JWT token from the auth service
- Replace `YOUR_JWT_TOKEN` with your actual token

## Create Packages

### Package 1: Basic Training Package (150,000 RWF)
```bash
curl -X POST http://localhost:8080/packages \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Basic Training Package",
    "description": "Coding and Design (Figma)",
    "price": 150000,
    "skills": "Coding, Design (Figma)",
    "duration_months": 3,
    "currency": "RWF"
  }'
```

### Package 2: Standard Training Package (250,000 RWF)
```bash
curl -X POST http://localhost:8080/packages \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Standard Training Package",
    "description": "Coding, Design (Figma), 3D Printing, and CNC",
    "price": 250000,
    "skills": "Coding, Design (Figma), 3D Printing, CNC",
    "duration_months": 6,
    "currency": "RWF"
  }'
```

### Package 3: Premium Training Package (300,000 RWF)
```bash
curl -X POST http://localhost:8080/packages \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Premium Training Package",
    "description": "Coding, Design (Figma), 3D Printing, CNC, and IoT (Embedded System)",
    "price": 300000,
    "skills": "Coding, Design (Figma), 3D Printing, CNC, IoT (Embedded System)",
    "duration_months": 9,
    "currency": "RWF"
  }'
```

## Other Package Operations

### List All Packages (Public)
```bash
curl -X GET http://localhost:8080/packages
```

### Get Single Package by ID (Public)
```bash
curl -X GET http://localhost:8080/packages/1
```

### Update Package (Admin)
```bash
curl -X PUT http://localhost:8080/packages/1 \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Updated Package Name",
    "description": "Updated description",
    "price": 200000,
    "skills": "Updated skills",
    "duration_months": 4,
    "currency": "RWF"
  }'
```

### Delete Package (Admin)
```bash
curl -X DELETE http://localhost:8080/packages/1 \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"
```

## Quick Upload Script
Run the provided script to upload all packages at once:
```bash
# Edit the script to set your AUTH_TOKEN
nano upload_packages.sh

# Run the script
./upload_packages.sh
```
