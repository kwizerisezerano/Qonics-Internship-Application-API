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

// applicantContact resolves an applicant's decrypted email and full name from
// their user id. It is intentionally best-effort: any lookup or decryption error
// yields empty strings so callers can still notify over whichever channel is
// available without failing the surrounding request.
func (ac *ApplicationController) applicantContact(ctx context.Context, userID int) (email, name string) {
	var emailEncrypted, nameEncrypted []byte
	err := ac.DB.QueryRow(ctx, `
		SELECT email_encrypted, full_name_encrypted
		FROM applicant_profiles
		WHERE user_id = $1
	`, userID).Scan(&emailEncrypted, &nameEncrypted)
	if err != nil {
		fmt.Printf("WARNING: Failed to resolve applicant contact for user %d: %v\n", userID, err)
		return "", ""
	}

	if len(nameEncrypted) > 0 {
		if decrypted, decErr := utils.DecryptData(ctx, ac.DB, nameEncrypted); decErr == nil {
			name = decrypted
		}
	}
	if len(emailEncrypted) > 0 {
		if decrypted, decErr := utils.DecryptData(ctx, ac.DB, emailEncrypted); decErr == nil {
			email = decrypted
		}
	}
	return email, name
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

	// Build the filtered WHERE clause (structured filters + caller scope + search).
	filter, whereClause, params, err := ac.parseFilter(c, profileID)
	if err != nil {
		// Known error - invalid filter, no logging
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}

	// Summarise the caller's filtered applications by status for the dashboard.
	summary, err := ac.applicationSummary(ctx, whereClause, params)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to fetch applications", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}

	applications, err := ac.queryApplications(ctx, whereClause, params, filter.Limit, (filter.Page-1)*filter.Limit)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to fetch applications", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}
	if applications == nil {
		applications = []model.Application{}
	}

	return utils.JsonPaginatedSuccessResponse(c, "Applications retrieved", model.MineResponse{
		Applications: applications,
		Summary:      summary,
	}, utils.GeneratePaginationData(summary.Total, filter.Limit, filter.Page))
}

// ListAll returns all applications (admin only)
func (ac *ApplicationController) ListAll(c *fiber.Ctx) error {
	role := c.Locals("role")
	if role != "admin" {
		// Known error - forbidden, no logging
		return utils.JsonErrorResponse(c, fiber.StatusForbidden, "Admin access required")
	}

	ctx := context.Background()

	filter, whereClause, params, err := ac.parseFilter(c, "")
	if err != nil {
		// Known error - invalid filter, no logging
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}

	// Dashboard aggregates (over the filtered set, not just the current page).
	summary, err := ac.applicationSummary(ctx, whereClause, params)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to fetch applications", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}

	applications, err := ac.queryApplications(ctx, whereClause, params, filter.Limit, (filter.Page-1)*filter.Limit)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to fetch applications", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}
	if applications == nil {
		applications = []model.Application{}
	}

	byDepartment, err := ac.applicationsByDepartment(ctx, whereClause, params)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to fetch applications", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}

	trends, err := ac.applicationTrends(ctx, whereClause, params, 12)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to fetch applications", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}

	bySpecialization, err := ac.applicationsBySpecialization(ctx, whereClause, params)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to fetch applications", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}

	return utils.JsonPaginatedSuccessResponse(c, "Applications retrieved", model.ListAllResponse{
		Applications:     applications,
		Summary:          summary,
		ByDepartment:     byDepartment,
		BySpecialization: bySpecialization,
		Trends:           trends,
	}, utils.GeneratePaginationData(summary.Total, filter.Limit, filter.Page))
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

	// Validate the submitted values (department, specialization and the other
	// enumerated fields) before touching the database.
	if err := model.Validate.Struct(&req); err != nil {
		// Known error - invalid payload, no logging
		return utils.JsonValidationErrorResponse(c, err)
	}

	// Enforce the admin-controlled intake window (settings row id = 1) before any
	// work is done: submissions are rejected when applications are switched off
	// or when the closing date has arrived. A missing settings row means open.
	var applicationsOpen bool
	var closeOn *time.Time
	var closureMessage string
	err = ac.DB.QueryRow(ctx, `
		SELECT applications_open, close_on, closure_message
		FROM settings WHERE id = 1
	`).Scan(&applicationsOpen, &closeOn, &closureMessage)
	if err != nil && err != pgx.ErrNoRows {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to load application settings", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}
	if err == nil && (!applicationsOpen || (closeOn != nil && !closeOn.After(time.Now()))) {
		message := closureMessage
		if message == "" {
			message = "Applications are currently closed."
		}
		// Known error - intake window closed, no logging
		return utils.JsonErrorResponse(c, fiber.StatusForbidden, message)
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

	// Generate reference (nanosecond precision keeps it unique even when two
	// applications are submitted within the same second).
	reference := fmt.Sprintf("APP-%d", time.Now().UnixNano())
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

	// Notify the applicant over both channels (in-app + email) and fan out to
	// every admin. Notification failures must never fail the submission, so they
	// are logged only. Emails are rendered from HTML templates (leazi pattern).
	applicantEmail, applicantName := ac.applicantContact(ctx, userIDInt)
	if applicantName == "" {
		applicantName = "An applicant"
	}

	applicantSubject := "Application Submitted Successfully"
	applicantMessage := fmt.Sprintf("Your application for %s - %s has been submitted successfully and is awaiting review. You will be notified once it has been reviewed. Reference: %s", req.Department, req.Specialization, reference)
	if err := utils.PublishInAppNotification(ctx, ac.Redis, userIDInt, applicantSubject, applicantMessage); err != nil {
		fmt.Printf("WARNING: Failed to send application submission notification: %v\n", err)
	}
	if applicantEmail != "" {
		if err := utils.PublishTemplatedEmailNotification(ctx, ac.Redis, applicantEmail, applicantSubject, "application_submitted", map[string]interface{}{
			"name":           applicantName,
			"reference":      reference,
			"department":     req.Department,
			"specialization": req.Specialization,
		}); err != nil {
			fmt.Printf("WARNING: Failed to send application submission email: %v\n", err)
		}
	}

	adminSubject := "New Application Submitted"
	adminMessage := fmt.Sprintf("%s submitted an application for %s - %s. Reference: %s", applicantName, req.Department, req.Specialization, reference)
	if err := utils.NotifyAdmins(ctx, ac.DB, ac.Redis, adminSubject, adminMessage, "admin_new_application", map[string]interface{}{
		"applicant_name": applicantName,
		"reference":      reference,
		"department":     req.Department,
		"specialization": req.Specialization,
		"login_url":      ac.Config.WebURL,
	}); err != nil {
		fmt.Printf("WARNING: Failed to notify admins of new application: %v\n", err)
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

	if err := model.Validate.Struct(&req); err != nil {
		// Known error - invalid payload, no logging
		return utils.JsonValidationErrorResponse(c, err)
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
	var reference string
	err = ac.DB.QueryRow(ctx, `
		SELECT ap.user_id, a.reference
		FROM applications a
		JOIN applicant_profiles ap ON a.applicant_id = ap.id
		WHERE a.id = $1
	`, id).Scan(&applicantUserID, &reference)

	if err == nil && applicantUserID > 0 {
		// Notify the applicant over both channels (in-app + email). The student
		// must learn about a status change regardless of which channel they use.
		// The email is rendered from an HTML template (leazi pattern).
		notificationTitle := fmt.Sprintf("Application Status Updated: %s", req.Status)
		notificationMessage := fmt.Sprintf("Your application status has been updated to %s. %s", req.Status, req.Note)
		if err := utils.PublishInAppNotification(ctx, ac.Redis, applicantUserID, notificationTitle, notificationMessage); err != nil {
			// Log warning but don't fail the update
			fmt.Printf("WARNING: Failed to send status update notification: %v\n", err)
		}
		if applicantEmail, applicantName := ac.applicantContact(ctx, applicantUserID); applicantEmail != "" {
			if err := utils.PublishTemplatedEmailNotification(ctx, ac.Redis, applicantEmail, notificationTitle, "application_status_updated", map[string]interface{}{
				"name":      applicantName,
				"reference": reference,
				"status":    req.Status,
				"note":      req.Note,
			}); err != nil {
				fmt.Printf("WARNING: Failed to send status update email: %v\n", err)
			}
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

	if err := model.Validate.Struct(&req); err != nil {
		// Known error - invalid payload, no logging
		return utils.JsonValidationErrorResponse(c, err)
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

// parseFilter reads the query string into an ApplicationFilter, applies the
// leazi pagination defaults and builds the shared WHERE clause: the structured
// filters via utils.BuildWhereClause, an optional caller scope and the global
// search (plain columns with ILIKE, encrypted PII via the hash columns).
func (ac *ApplicationController) parseFilter(c *fiber.Ctx, applicantID string) (model.ApplicationFilter, string, []any, error) {
	filter := model.ApplicationFilter{}
	if err := c.QueryParser(&filter); err != nil {
		return filter, "", nil, fmt.Errorf("invalid query parameters")
	}

	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.Limit < 1 {
		filter.Limit = 10
	}
	if filter.Limit > 100 {
		filter.Limit = 100
	}

	from, err := parseFilterTime(c.Query("submitted_from"))
	if err != nil {
		return filter, "", nil, fmt.Errorf("invalid submitted_from")
	}
	filter.SubmittedFrom = from

	to, err := parseFilterTime(c.Query("submitted_to"))
	if err != nil {
		return filter, "", nil, fmt.Errorf("invalid submitted_to")
	}
	filter.SubmittedTo = to

	whereClause, _, params, err := utils.BuildWhereClause(&filter)
	if err != nil {
		return filter, "", nil, err
	}

	if applicantID != "" {
		params = append(params, applicantID)
		whereClause = utils.AppendCondition(whereClause, fmt.Sprintf("a.applicant_id = $%d", len(params)))
	}

	whereClause, params = utils.AppendSearchClause(whereClause, params,
		[]string{"a.reference", "a.department", "a.specialization"},
		[]string{"ap.full_name_hash", "ap.email_hash"},
		filter.Search,
	)

	return filter, whereClause, params, nil
}

// parseFilterTime parses an optional date query parameter, accepting RFC3339 or
// a plain YYYY-MM-DD date.
func parseFilterTime(value string) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02", "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, value); err == nil {
			return &t, nil
		}
	}
	return nil, fmt.Errorf("invalid date %q", value)
}

// applicationSummary counts the filtered applications by status. The COUNT(*)
// also serves as the total used for pagination.
func (ac *ApplicationController) applicationSummary(ctx context.Context, whereClause string, params []any) (model.ApplicationSummary, error) {
	var s model.ApplicationSummary
	err := ac.DB.QueryRow(ctx, `
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE a.status = 'submitted'),
		       COUNT(*) FILTER (WHERE a.status = 'under_review'),
		       COUNT(*) FILTER (WHERE a.status = 'shortlisted'),
		       COUNT(*) FILTER (WHERE a.status = 'interview'),
		       COUNT(*) FILTER (WHERE a.status = 'accepted'),
		       COUNT(*) FILTER (WHERE a.status = 'rejected')
		FROM applications a
		JOIN applicant_profiles ap ON a.applicant_id = ap.id
	`+whereClause, params...).Scan(
		&s.Total, &s.Submitted, &s.UnderReview, &s.Shortlisted,
		&s.Interview, &s.Accepted, &s.Rejected,
	)
	return s, err
}

// applicationsByDepartment aggregates the filtered applications by department,
// most applications first.
func (ac *ApplicationController) applicationsByDepartment(ctx context.Context, whereClause string, params []any) ([]model.DepartmentCount, error) {
	rows, err := ac.DB.Query(ctx, `
		SELECT a.department, COUNT(*)
		FROM applications a
		JOIN applicant_profiles ap ON a.applicant_id = ap.id
	`+whereClause+`
		GROUP BY a.department
		ORDER BY COUNT(*) DESC, a.department ASC
	`, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := []model.DepartmentCount{}
	for rows.Next() {
		var dc model.DepartmentCount
		if err := rows.Scan(&dc.Department, &dc.Count); err != nil {
			return nil, err
		}
		counts = append(counts, dc)
	}
	return counts, nil
}

// applicationsBySpecialization aggregates the filtered applications by
// department and specialization, most applications first within each
// department.
func (ac *ApplicationController) applicationsBySpecialization(ctx context.Context, whereClause string, params []any) ([]model.SpecializationCount, error) {
	rows, err := ac.DB.Query(ctx, `
		SELECT a.department, a.specialization, COUNT(*)
		FROM applications a
		JOIN applicant_profiles ap ON a.applicant_id = ap.id
	`+whereClause+`
		GROUP BY a.department, a.specialization
		ORDER BY a.department ASC, COUNT(*) DESC, a.specialization ASC
	`, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := []model.SpecializationCount{}
	for rows.Next() {
		var sc model.SpecializationCount
		if err := rows.Scan(&sc.Department, &sc.Specialization, &sc.Count); err != nil {
			return nil, err
		}
		counts = append(counts, sc)
	}
	return counts, nil
}

// applicationTrends returns the number of filtered applications submitted in
// each of the last `months` months, oldest first.
func (ac *ApplicationController) applicationTrends(ctx context.Context, whereClause string, params []any, months int) ([]model.TrendPoint, error) {
	rows, err := ac.DB.Query(ctx, `
		SELECT to_char(date_trunc('month', a.submitted_at), 'YYYY-MM'), COUNT(*)
		FROM applications a
		JOIN applicant_profiles ap ON a.applicant_id = ap.id
	`+whereClause+`
		GROUP BY 1
	`, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := map[string]int{}
	for rows.Next() {
		var month string
		var count int
		if err := rows.Scan(&month, &count); err != nil {
			return nil, err
		}
		counts[month] = count
	}

	now := time.Now()
	points := make([]model.TrendPoint, months)
	for i := 0; i < months; i++ {
		d := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).AddDate(0, -(months - 1 - i), 0)
		points[i] = model.TrendPoint{Month: d.Format("Jan"), Count: counts[d.Format("2006-01")]}
	}
	return points, nil
}

// applicationSelect is the shared projection for application rows.
const applicationSelect = `
	SELECT a.id, a.reference, a.applicant_id, a.department, a.specialization,
	       a.motivation, a.experience, a.start_date, a.education_level,
	       a.session, a.duration_months, a.work_mode, a.status,
	       a.submitted_at, a.interview_at,
	       ap.full_name_encrypted, ap.email_encrypted, ap.phone_encrypted
	FROM applications a
	JOIN applicant_profiles ap ON a.applicant_id = ap.id
`

// queryApplications runs the shared application query with an optional
// LIMIT/OFFSET. A non-positive limit fetches every matching row.
func (ac *ApplicationController) queryApplications(ctx context.Context, whereClause string, params []any, limit int, offset int) ([]model.Application, error) {
	query := applicationSelect + whereClause + " ORDER BY a.submitted_at DESC"

	args := params
	if limit > 0 {
		args = append(append([]any{}, params...), limit, offset)
		query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(params)+1, len(params)+2)
	}

	rows, err := ac.DB.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return ac.scanApplications(ctx, rows)
}

// fetchApplications fetches applications scoped by applicant or application id.
func (ac *ApplicationController) fetchApplications(ctx context.Context, applicantID string, applicationID string) ([]model.Application, error) {
	whereClause := ""
	params := []any{}

	if applicantID != "" {
		whereClause = " WHERE a.applicant_id = $1"
		params = append(params, applicantID)
	} else if applicationID != "" {
		whereClause = " WHERE a.id = $1"
		params = append(params, applicationID)
	}

	return ac.queryApplications(ctx, whereClause, params, 0, 0)
}

func (ac *ApplicationController) scanApplications(ctx context.Context, rows pgx.Rows) ([]model.Application, error) {
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
		WHERE application_id = $1 ORDER BY at DESC
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
