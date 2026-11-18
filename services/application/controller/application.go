package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qonics/qonics-appy-api/libs/shared-package/config"
	"github.com/qonics/qonics-appy-api/libs/shared-package/utils"
	"github.com/qonics/qonics-appy-api/services/application/model"
	"github.com/redis/go-redis/v9"
)

type ApplicationController struct {
	DB     *pgxpool.Pool
	Config *config.Config
	Redis  *redis.Client
}

func NewApplicationController(db *pgxpool.Pool, cfg *config.Config, rdb *redis.Client) *ApplicationController {
	return &ApplicationController{
		DB:     db,
		Config: cfg,
		Redis:  rdb,
	}
}

// ListMine returns the current user's applications
func (ac *ApplicationController) ListMine(c *fiber.Ctx) error {
	userID := c.Locals("user_id")

	// Check if user is authenticated
	if userID == nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, "Unauthorized")
	}

	ctx := context.Background()

	// Get applicant profile ID
	var profileID string
	// Convert userID to int for database query
	userIDInt := int(userID.(float64))
	err := ac.DB.QueryRow(ctx, "SELECT id FROM applicant_profiles WHERE user_id = $1", userIDInt).Scan(&profileID)
	if err != nil {
		// Known error - profile not found, no logging
		return utils.JsonErrorResponse(c, fiber.StatusNotFound, "Applicant profile not found")
	}

	applications, err := ac.fetchApplications(ctx, profileID, "")
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to fetch applications", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}

	return utils.JsonSuccessResponse(c, "Applications retrieved", applications)
}

// ListAll returns all applications (admin only)
func (ac *ApplicationController) ListAll(c *fiber.Ctx) error {
	role := c.Locals("role")
	if role != "admin" {
		// Known error - forbidden, no logging
		return utils.JsonErrorResponse(c, fiber.StatusForbidden, "Admin access required")
	}

	ctx := context.Background()

	applications, err := ac.fetchApplications(ctx, "", "")
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to fetch applications", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}

	return utils.JsonSuccessResponse(c, "Applications retrieved", applications)
}

// Get returns a single application by ID
func (ac *ApplicationController) Get(c *fiber.Ctx) error {
	id := c.Params("id")
	ctx := context.Background()

	applications, err := ac.fetchApplications(ctx, "", id)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to fetch application", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}

	if len(applications) == 0 {
		// Known error - not found, no logging
		return utils.JsonErrorResponse(c, fiber.StatusNotFound, "Application not found")
	}

	return utils.JsonSuccessResponse(c, "Application retrieved", applications[0])
}

// Create creates a new application
func (ac *ApplicationController) Create(c *fiber.Ctx) error {
	userID := c.Locals("user_id")

	// Check if user is authenticated
	if userID == nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, "Unauthorized")
	}

	ctx := context.Background()

	var req model.NewApplicationPayload
	var err error

	if err := c.BodyParser(&req); err != nil {
		// Known error - invalid request body, no logging
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Invalid request body")
	}

	// Get applicant profile ID
	var profileID string
	// Convert userID to int for database query
	userIDInt := int(userID.(float64))
	err = ac.DB.QueryRow(ctx, "SELECT id FROM applicant_profiles WHERE user_id = $1", userIDInt).Scan(&profileID)
	if err != nil {
		// Known error - profile not found, no logging
		return utils.JsonErrorResponse(c, fiber.StatusNotFound, "Applicant profile not found")
	}

	// Verify every referenced document belongs to the caller and is confirmed
	// before the application is created (leazi-style ownership guard).
	ownerIDStr := fmt.Sprintf("%v", userID)
	for _, docID := range req.DocumentIDs {
		var owned bool
		err = ac.DB.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM files
				WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL
			)
		`, docID, ownerIDStr).Scan(&owned)

		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to verify documents", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     err.Error(),
				ServiceName: ac.Config.ServiceName,
			})
		}
		if !owned {
			return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Invalid or unconfirmed document")
		}
	}

	// Generate reference
	reference := fmt.Sprintf("APP-%d", time.Now().Unix())
	appID := uuid.New().String()

	var startDate *time.Time
	if req.StartDate != "" {
		parsed, parseErr := time.Parse(time.RFC3339, req.StartDate)
		if parseErr != nil {
			// Known error - invalid date format, no logging
			return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Invalid date format")
		}
		startDate = &parsed
	}

	// Insert application
	_, err = ac.DB.Exec(ctx, `
		INSERT INTO applications (id, reference, applicant_id, department, specialization,
			motivation, experience, start_date, education_level, session,
			duration_months, work_mode, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`, appID, reference, profileID, req.Department, req.Specialization,
		req.Motivation, req.Experience, startDate, req.EducationLevel,
		req.Session, req.DurationMonths, req.WorkMode, "submitted")

	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to create application", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}

	// Link document records to application (generic source_table/source_table_id pair).
	// Ownership is re-asserted in the WHERE clause so a file can never be
	// attached by anyone other than its owner.
	for _, docID := range req.DocumentIDs {
		result, err := ac.DB.Exec(ctx, `
			UPDATE files
			SET source_table = 'applications', source_table_id = $1
			WHERE id = $2 AND owner_id = $3 AND deleted_at IS NULL
		`, appID, docID, ownerIDStr)

		if err != nil {
			fmt.Printf("WARNING: Failed to link document %s to application: %v\n", docID, err)
			continue
		}
		if result.RowsAffected() == 0 {
			fmt.Printf("WARNING: Document %s was not linked (not owned or not confirmed)\n", docID)
		}
	}

	// Add initial history entry
	_, err = ac.DB.Exec(ctx, `
		INSERT INTO application_history (application_id, status, note)
		VALUES ($1, 'submitted', 'Application received.')
	`, appID)

	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to create history", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}

	// Send in-app notification about application submission
	notificationTitle := "Application Submitted Successfully"
	notificationMessage := fmt.Sprintf("Your application for %s - %s has been submitted successfully. Reference: %s", req.Department, req.Specialization, reference)
	if err := utils.PublishInAppNotification(ctx, ac.Redis, userIDInt, notificationTitle, notificationMessage); err != nil {
		// Log warning but don't fail the creation
		fmt.Printf("WARNING: Failed to send application submission notification: %v\n", err)
	}

	// Fetch created application
	applications, err := ac.fetchApplications(ctx, "", appID)
	if err != nil {
		return err
	}

	return utils.JsonSuccessResponse(c, "Application created", applications[0])
}

// UpdateStatus updates the status of an application (admin only)
func (ac *ApplicationController) UpdateStatus(c *fiber.Ctx) error {
	role := c.Locals("role")
	if role != "admin" {
		// Known error - forbidden, no logging
		return utils.JsonErrorResponse(c, fiber.StatusForbidden, "Admin access required")
	}

	id := c.Params("id")
	var req model.StatusUpdatePayload

	if err := c.BodyParser(&req); err != nil {
		// Known error - invalid request body, no logging
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Invalid request body")
	}

	ctx := context.Background()

	// Get current status
	var currentStatus string
	var currentInterviewAt *time.Time
	err := ac.DB.QueryRow(ctx, "SELECT status, interview_at FROM applications WHERE id = $1", id).Scan(&currentStatus, &currentInterviewAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return utils.JsonErrorResponse(c, fiber.StatusNotFound, "Application not found")
		}
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Database error", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}

	var interviewAt *time.Time
	if req.InterviewAt != "" {
		parsed, err := time.Parse(time.RFC3339, req.InterviewAt)
		if err != nil {
			// Known error - invalid date format, no logging
			return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Invalid date format")
		}
		interviewAt = &parsed
	}

	// Check if status is actually changing
	if currentStatus == req.Status {
		// Compare interview timestamps
		if (currentInterviewAt == nil && interviewAt == nil) ||
			(currentInterviewAt != nil && interviewAt != nil && currentInterviewAt.Equal(*interviewAt)) {
			return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "Status is already set to this value")
		}
	}

	// Update application status
	_, err = ac.DB.Exec(ctx, `
		UPDATE applications
		SET status = $1, interview_at = $2, updated_at = NOW()
		WHERE id = $3
	`, req.Status, interviewAt, id)

	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to update status", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}

	// Add history entry
	_, err = ac.DB.Exec(ctx, `
		INSERT INTO application_history (application_id, status, note)
		VALUES ($1, $2, $3)
	`, id, req.Status, req.Note)

	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to create history", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}

	// Get user_id for notification
	var applicantUserID int
	err = ac.DB.QueryRow(ctx, `
		SELECT ap.user_id
		FROM applications a
		JOIN applicant_profiles ap ON a.applicant_id = ap.id
		WHERE a.id = $1
	`, id).Scan(&applicantUserID)

	if err == nil && applicantUserID > 0 {
		// Send in-app notification about status change
		notificationTitle := fmt.Sprintf("Application Status Updated: %s", req.Status)
		notificationMessage := fmt.Sprintf("Your application status has been updated to %s. %s", req.Status, req.Note)
		if err := utils.PublishInAppNotification(ctx, ac.Redis, applicantUserID, notificationTitle, notificationMessage); err != nil {
			// Log warning but don't fail the update
			fmt.Printf("WARNING: Failed to send status update notification: %v\n", err)
		}
	}

	// Fetch updated application
	applications, err := ac.fetchApplications(ctx, "", id)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to fetch application", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}

	return utils.JsonSuccessResponse(c, "Status updated", applications[0])
}

// AddNote adds a review note to an application
func (ac *ApplicationController) AddNote(c *fiber.Ctx) error {
	id := c.Params("id")
	var req model.AddNoteRequest
	author := c.Locals("email")

	if author == nil {
		// Known error - not authenticated, no logging
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, "Unauthorized")
	}

	if err := c.BodyParser(&req); err != nil {
		// Known error - invalid request body, no logging
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Invalid request body")
	}

	ctx := context.Background()

	// Convert author to string (from interface{})
	authorStr, ok := author.(string)
	if !ok {
		// Known error - invalid author type, no logging
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Invalid author information")
	}

	// Check for duplicate note (same author, same application, same text)
	var existingCount int
	err := ac.DB.QueryRow(ctx, `
		SELECT COUNT(id) FROM review_notes
		WHERE application_id = $1 AND author = $2 AND text = $3
	`, id, authorStr, req.Text).Scan(&existingCount)

	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to check for duplicate note", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}

	if existingCount > 0 {
		// Known error - duplicate note, no logging
		return utils.JsonErrorResponse(c, fiber.StatusConflict, "This note already exists")
	}

	noteID := uuid.New().String()
	_, err = ac.DB.Exec(ctx, `
		INSERT INTO review_notes (id, application_id, author, text)
		VALUES ($1, $2, $3, $4)
	`, noteID, id, authorStr, req.Text)

	if err != nil {
		// Check for foreign key error - known error, no logging
		if isFK, tableName := utils.IsForeignKeyErr(err); isFK {
			return utils.JsonErrorResponse(c, fiber.StatusBadRequest, utils.GetForeignKeyMessage(tableName))
		}
		// Unknown error - log as critical
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to add note", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}

	// Fetch updated application
	applications, err := ac.fetchApplications(ctx, "", id)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to fetch application", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}

	return utils.JsonSuccessResponse(c, "Note added", applications[0])
}

// Availability returns session availability information
func (ac *ApplicationController) Availability(c *fiber.Ctx) error {
	ctx := context.Background()

	rows, err := ac.DB.Query(ctx, `
		SELECT start_date, duration_months, session, COUNT(id)
		FROM applications
		WHERE work_mode IN ('onsite', 'hybrid') AND status IN ('submitted', 'under_review', 'shortlisted', 'interview', 'accepted')
		GROUP BY start_date, duration_months, session
		ORDER BY start_date
	`)

	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Database error", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}
	defer rows.Close()

	// SESSION_CAPACITY from frontend
	const SESSION_CAPACITY = 20

	var bookings []model.Booking
	for rows.Next() {
		var startDate time.Time
		var durationMonths int
		var session string
		var count int

		err := rows.Scan(&startDate, &durationMonths, &session, &count)
		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Error scanning row", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     err.Error(),
				ServiceName: ac.Config.ServiceName,
			})
		}

		endDate := startDate.AddDate(0, durationMonths, 0)
		availableSeats := SESSION_CAPACITY - count

		bookings = append(bookings, model.Booking{
			StartDate:      startDate.Format(time.RFC3339),
			EndDate:        endDate.Format(time.RFC3339),
			Session:        session,
			AvailableSeats: availableSeats,
		})
	}

	return utils.JsonSuccessResponse(c, "Availability retrieved", bookings)
}

// Helper function to fetch applications with related data
func (ac *ApplicationController) fetchApplications(ctx context.Context, applicantID string, applicationID string) ([]model.Application, error) {
	query := `
		SELECT a.id, a.reference, a.applicant_id, a.department, a.specialization,
		       a.motivation, a.experience, a.start_date, a.education_level,
		       a.session, a.duration_months, a.work_mode, a.status,
		       a.submitted_at, a.interview_at,
		       ap.full_name_encrypted, ap.email_encrypted, ap.phone_encrypted
		FROM applications a
		JOIN applicant_profiles ap ON a.applicant_id = ap.id
	`

	args := []interface{}{}
	argIndex := 1

	if applicantID != "" {
		query += fmt.Sprintf(" WHERE a.applicant_id = $%d", argIndex)
		args = append(args, applicantID)
		argIndex++
	} else if applicationID != "" {
		query += fmt.Sprintf(" WHERE a.id = $%d", argIndex)
		args = append(args, applicationID)
		argIndex++
	}

	query += " ORDER BY a.submitted_at DESC"

	rows, err := ac.DB.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var applications []model.Application
	for rows.Next() {
		var app model.Application
		var fullNameEncrypted, emailEncrypted, phoneEncrypted []byte
		var startDate *time.Time
		var submittedAt time.Time
		var interviewAt *time.Time

		err := rows.Scan(
			&app.ID, &app.Reference, &app.Applicant.ID, &app.Department, &app.Specialization,
			&app.Motivation, &app.Experience, &startDate, &app.EducationLevel,
			&app.Session, &app.DurationMonths, &app.WorkMode, &app.Status,
			&submittedAt, &interviewAt,
			&fullNameEncrypted, &emailEncrypted, &phoneEncrypted,
		)

		if err != nil {
			return nil, err
		}

		// Decrypt PII using pgcrypto
		app.Applicant.FullName, err = utils.DecryptData(ctx, ac.DB, fullNameEncrypted)
		if err != nil {
			return nil, err
		}

		app.Applicant.Email, err = utils.DecryptData(ctx, ac.DB, emailEncrypted)
		if err != nil {
			return nil, err
		}

		app.Applicant.Phone, err = utils.DecryptData(ctx, ac.DB, phoneEncrypted)
		if err != nil {
			return nil, err
		}

		if startDate != nil {
			dateStr := startDate.Format(time.RFC3339)
			app.StartDate = &dateStr
		}
		app.SubmittedAt = submittedAt.Format(time.RFC3339)
		if interviewAt != nil {
			interviewStr := interviewAt.Format(time.RFC3339)
			app.InterviewAt = &interviewStr
		}

		// Fetch documents
		app.Documents, _ = ac.fetchDocuments(ctx, app.ID)

		// Fetch history
		app.History, _ = ac.fetchHistory(ctx, app.ID)

		// Fetch notes
		app.Notes, _ = ac.fetchNotes(ctx, app.ID)

		applications = append(applications, app)
	}

	return applications, nil
}

func (ac *ApplicationController) fetchDocuments(ctx context.Context, applicationID string) ([]model.ApplicationDocument, error) {
	rows, err := ac.DB.Query(ctx, `
		SELECT id, file_type, name, file_size, COALESCE(uploaded_at, created_at)
		FROM files
		WHERE source_table = 'applications' AND source_table_id = $1
	`, applicationID)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var documents []model.ApplicationDocument
	for rows.Next() {
		var doc model.ApplicationDocument
		var uploadedAt time.Time

		err := rows.Scan(&doc.ID, &doc.Type, &doc.Name, &doc.Size, &uploadedAt)
		if err != nil {
			return nil, err
		}

		doc.UploadedAt = uploadedAt.Format(time.RFC3339)
		documents = append(documents, doc)
	}

	return documents, nil
}

func (ac *ApplicationController) fetchHistory(ctx context.Context, applicationID string) ([]model.StatusEvent, error) {
	rows, err := ac.DB.Query(ctx, `
		SELECT status, at, note FROM application_history
		WHERE application_id = $1 ORDER BY at ASC
	`, applicationID)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []model.StatusEvent
	for rows.Next() {
		var event model.StatusEvent
		var at time.Time

		err := rows.Scan(&event.Status, &at, &event.Note)
		if err != nil {
			return nil, err
		}

		event.At = at.Format(time.RFC3339)
		history = append(history, event)
	}

	return history, nil
}

func (ac *ApplicationController) fetchNotes(ctx context.Context, applicationID string) ([]model.ReviewNote, error) {
	rows, err := ac.DB.Query(ctx, `
		SELECT id, author, text, at FROM review_notes
		WHERE application_id = $1 ORDER BY at DESC
	`, applicationID)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notes []model.ReviewNote
	for rows.Next() {
		var note model.ReviewNote
		var at time.Time

		err := rows.Scan(&note.ID, &note.Author, &note.Text, &at)
		if err != nil {
			return nil, err
		}

		note.At = at.Format(time.RFC3339)
		notes = append(notes, note)
	}

	return notes, nil
}
