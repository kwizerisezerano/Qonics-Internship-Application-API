package controller

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qonics/qonics-appy-api/libs/shared-package/config"
	"github.com/qonics/qonics-appy-api/libs/shared-package/utils"
	"github.com/qonics/qonics-appy-api/services/application/model"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
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
	user, ok := utils.GetUserPayload(c)
	if !ok {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, "Unauthorized")
	}

	ctx := context.Background()

	applicant, err := ac.applicantByUser(ctx, user.ID)
	if err != nil {
		if err == pgx.ErrNoRows {
			// Known error - profile not found, no logging
			return utils.JsonErrorResponse(c, fiber.StatusNotFound, "Applicant profile not found")
		}
		return ac.internalError(c, "Failed to fetch applications", err)
	}
	profileID := applicant.ProfileID

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
	// Internal review notes are for admins only.
	stripNotes(applications)

	return utils.JsonPaginatedSuccessResponse(c, "Applications retrieved", model.MineResponse{
		Applications: applications,
		Summary:      summary,
	}, utils.GeneratePaginationData(summary.Total, filter.Limit, filter.Page))
}

// ListAll returns all applications (admin only)
func (ac *ApplicationController) ListAll(c *fiber.Ctx) error {
	if user, _ := utils.GetUserPayload(c); !user.IsAdmin() {
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

	byPackage, err := ac.applicationsByPackage(ctx, whereClause, params)
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

	return utils.JsonPaginatedSuccessResponse(c, "Applications retrieved", model.ListAllResponse{
		Applications: applications,
		Summary:      summary,
		ByPackage:    byPackage,
		Trends:       trends,
	}, utils.GeneratePaginationData(summary.Total, filter.Limit, filter.Page))
}

// Get returns a single application by ID. Admins can read any application;
// applicants can only read their own, and never see internal review notes.
// Someone else's application answers 404 so its existence is not revealed.
func (ac *ApplicationController) Get(c *fiber.Ctx) error {
	user, ok := utils.GetUserPayload(c)
	if !ok {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, "Unauthorized")
	}
	id := c.Params("id")
	ctx := context.Background()

	whereClause := " WHERE a.id = $1"
	params := []any{id}
	if !user.IsAdmin() {
		whereClause += " AND ap.user_id = $2"
		params = append(params, user.ID)
	}

	applications, err := ac.queryApplications(ctx, whereClause, params, 0, 0)
	if err != nil {
		return ac.internalError(c, "Failed to fetch application", err)
	}

	if len(applications) == 0 {
		// Known error - not found, no logging
		return utils.JsonErrorResponse(c, fiber.StatusNotFound, "Application not found")
	}

	if !user.IsAdmin() {
		stripNotes(applications)
	}

	return utils.JsonSuccessResponse(c, "Application retrieved", applications[0])
}

// Create creates a new application
//
// Logged-in applicants apply under their own account. Guests (no token) must
// send full_name, email and phone: their account and profile are created in the
// same transaction as the application, and an email with a link to set a
// password is sent so they can open the portal later.
func (ac *ApplicationController) Create(c *fiber.Ctx) error {
	user, loggedIn := utils.GetUserPayload(c)
	if !loggedIn && c.Get("Authorization") != "" {
		// A token was sent but identifies nobody: not a guest, so reject it.
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, "Unauthorized")
	}
	guest := !loggedIn

	ctx := context.Background()

	var req model.NewApplicationPayload
	if err := c.BodyParser(&req); err != nil {
		// Known error - invalid request body, no logging
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Invalid request body")
	}
	req.Email = utils.NormalizeEmail(req.Email)

	// Validate the submitted values (department, specialization and the other
	// enumerated fields) before touching the database.
	if err := model.Validate.Struct(&req); err != nil {
		// Known error - invalid payload, no logging
		return utils.JsonValidationErrorResponse(c, err)
	}

	if guest {
		if req.FullName == "" || req.Email == "" || req.Phone == "" {
			// Known error - missing contact details, no logging
			return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Full name, email and phone are required to apply")
		}
		if len(req.DocumentIDs) > 0 && req.GuestToken == "" {
			// Known error - a guest's documents are tied to their upload token.
			return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "guest_token is required to attach documents")
		}
	}

	// Enforce the admin-controlled intake window before any work is done.
	intake, err := utils.GetIntakeStatus(ctx, ac.DB)
	if err != nil {
		return ac.internalError(c, "Failed to load application settings", err)
	}
	if !intake.Open {
		// Known error - intake window closed, no logging
		return utils.JsonErrorResponse(c, fiber.StatusForbidden, intake.Message)
	}

	// The caller's profile id, name and email in one query (used for the
	// application row and the notifications).
	var applicant applicantRef
	if guest {
		// The profile is created inside the transaction below.
		applicant = applicantRef{ProfileID: uuid.New().String(), Name: req.FullName, Email: req.Email}
	} else {
		applicant, err = ac.applicantByUser(ctx, user.ID)
		if err != nil {
			if err == pgx.ErrNoRows {
				// Known error - profile not found, no logging
				return utils.JsonErrorResponse(c, fiber.StatusNotFound, "Applicant profile not found")
			}
			return ac.internalError(c, "Failed to create application", err)
		}
	}

	var startDate *time.Time
	if req.StartDate != "" {
		parsed, parseErr := time.Parse(time.RFC3339, req.StartDate)
		if parseErr != nil {
			// Known error - invalid date format, no logging
			return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Invalid date format")
		}
		startDate = &parsed
	}

	// Every referenced document must belong to the caller and be confirmed:
	// checked for all documents in one query.
	ownerID := fmt.Sprintf("%d", user.ID)
	documentIDs := uniqueStrings(req.DocumentIDs)

	// A logged-in applicant's documents are confirmed files they own. A guest's
	// are files uploaded with their guest token: confirmed (uploaded_at) but
	// still carrying the pending expiry until this application claims them.
	docOwner, docState := ownerID, "deleted_at IS NULL"
	if guest {
		docOwner = utils.GuestUploadOwner(req.GuestToken)
		docState = "uploaded_at IS NOT NULL AND deleted_at > NOW()"
	}
	if len(documentIDs) > 0 {
		var owned int
		err = ac.DB.QueryRow(ctx, `
			SELECT COUNT(id) FROM files
			WHERE id = ANY($1) AND owner_id = $2 AND `+docState+`
		`, documentIDs, docOwner).Scan(&owned)
		if err != nil {
			return ac.internalError(c, "Failed to verify documents", err)
		}
		if owned != len(documentIDs) {
			return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Invalid or unconfirmed document")
		}
	}

	// Generate reference (nanosecond precision keeps it unique even when two
	// applications are submitted within the same second).
	reference := fmt.Sprintf("APP-%d", time.Now().UnixNano())
	appID := uuid.New().String()

	// The application row, its document links and the first history entry are
	// written in one transaction so a failure never leaves a partial submission.
	tx, err := ac.DB.Begin(ctx)
	if err != nil {
		return ac.internalError(c, "Failed to create application", err)
	}
	defer tx.Rollback(ctx)

	// Guests get their account and profile first; if anything below fails (full
	// session, bad data) the rollback removes them again.
	if guest {
		userID, ok := ac.createGuestAccount(c, tx, applicant.ProfileID, req)
		if !ok {
			return nil // the error response is already written
		}
		user = utils.UserPayload{ID: userID, Email: req.Email, Role: "applicant"}
		ownerID = fmt.Sprintf("%d", user.ID)
	}

	// One applicant can't hold two live applications for the same package. The
	// per-applicant lock makes a double-click submit wait for the first one.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('applicant:' || $1))`, applicant.ProfileID); err != nil {
		return ac.internalError(c, "Failed to create application", err)
	}
	var existingRef string
	err = tx.QueryRow(ctx, `
		SELECT reference FROM applications
		WHERE applicant_id = $1 AND package_id = $2 AND status <> 'rejected'
		LIMIT 1
	`, applicant.ProfileID, req.PackageID).Scan(&existingRef)
	if err == nil {
		// Known error - duplicate application, no logging
		return utils.JsonErrorResponse(c, fiber.StatusConflict, fmt.Sprintf(
			"You already have an application for this package (%s).", existingRef))
	}
	if err != pgx.ErrNoRows {
		return ac.internalError(c, "Failed to create application", err)
	}

	// Lookup the package to get price, duration, and skills
	pkg, err := utils.LookupPackage(ctx, tx, req.PackageID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return utils.JsonErrorResponse(c, fiber.StatusNotFound, "Package not found")
		}
		return ac.internalError(c, "Failed to lookup package", err)
	}

	// On-site and hybrid interns need a seat in their daily session for the
	// whole internship. Count live applications whose period overlaps this one;
	// the per-session lock stops two submissions taking the last seat at once.
	if model.UsesSeat(req.WorkMode) && startDate != nil {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('session:' || $1))`, req.Session); err != nil {
			return ac.internalError(c, "Failed to create application", err)
		}
		endDate := startDate.AddDate(0, pkg.DurationMonths, 0)
		var taken int
		err = tx.QueryRow(ctx, `
			SELECT COUNT(id) FROM applications
			WHERE session = $1 AND `+seatHoldingFilter+`
			  AND start_date < $3
			  AND start_date + make_interval(months => duration_months) > $2
		`, req.Session, *startDate, endDate).Scan(&taken)
		if err != nil {
			return ac.internalError(c, "Failed to check seat availability", err)
		}
		if taken >= model.SessionCapacity {
			// Known error - session full, no logging
			return utils.JsonErrorResponse(c, fiber.StatusConflict,
				"That session has no free seats for your dates. Choose another start date or session, or apply remotely.")
		}
	}

	// The amount is set by the package pricing, never by the client.
	amount := pkg.Price

	// Insert application with package details
	_, err = tx.Exec(ctx, `
		INSERT INTO applications (id, reference, applicant_id, package_id, skills,
			motivation, experience, start_date, education_level, session,
			duration_months, work_mode, amount, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`, appID, reference, applicant.ProfileID, req.PackageID, pkg.Skills,
		req.Motivation, req.Experience, startDate, req.EducationLevel,
		req.Session, pkg.DurationMonths, req.WorkMode, amount, "submitted")
	if err != nil {
		return ac.internalError(c, "Failed to create application", err)
	}

	// Link all documents in one statement (generic source_table/source_table_id
	// pair). Ownership is re-asserted in the WHERE clause.
	if len(documentIDs) > 0 {
		result, err := tx.Exec(ctx, `
			UPDATE files
			SET source_table = 'applications', source_table_id = $1,
			    owner_id = $4, deleted_at = NULL
			WHERE id = ANY($2) AND owner_id = $3 AND `+docState+`
		`, appID, documentIDs, docOwner, ownerID)
		if err != nil {
			return ac.internalError(c, "Failed to attach documents", err)
		}
		if result.RowsAffected() != int64(len(documentIDs)) {
			// Ownership was verified above, so this only happens if a file was
			// removed in between: reject rather than submit without it.
			return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Invalid or unconfirmed document")
		}
	}

	// Add initial history entry
	_, err = tx.Exec(ctx, `
		INSERT INTO application_history (application_id, status, note)
		VALUES ($1, 'submitted', 'Application received.')
	`, appID)
	if err != nil {
		return ac.internalError(c, "Failed to create history", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return ac.internalError(c, "Failed to create application", err)
	}

	if guest {
		utils.RecordSuccessActivityLog(c, ac.DB, user.ID, "USER_REGISTER", fmt.Sprintf("Account created while applying (user_id=%d, applicant_profile_id=%s)", user.ID, applicant.ProfileID), nil)
	}

	// Record the submission in the audit trail (leazi-style activity log).
	utils.RecordSuccessActivityLog(c, ac.DB, user.ID, "APPLICATION_SUBMIT",
		fmt.Sprintf("Submitted application (application_id=%s, user_id=%d, reference=%s) for package %s", appID, user.ID, reference, pkg.Name),
		map[string]any{
			"application_id": appID,
			"reference":      reference,
			"package_id":     req.PackageID,
			"package_name":   pkg.Name,
		})

	// Notifications go out in the background: Redis being slow or down must never
	// hold up (or fail) a submission that is already saved. The gateway times out
	// at 3s, so waiting here made the applicant see a 500 for a saved application.
	go ac.notifySubmission(user.ID, applicant, req, reference, guest)

	// Fetch created application
	applications, err := ac.fetchApplications(ctx, "", appID)
	if err != nil {
		return ac.internalError(c, "Failed to fetch application", err)
	}

	return utils.JsonSuccessResponse(c, "Application created", applications[0])
}

// guestAccessTTL is how long the set-password link emailed to a guest
// applicant stays valid (longer than a normal reset: they may not open it today).
const guestAccessTTL = 7 * 24 * time.Hour

// createGuestAccount inserts the user and applicant profile for someone
// applying without an account, inside the caller's transaction. The password is
// random and never disclosed: the applicant sets their own through the emailed
// link. When ok is false the error response has already been written.
func (ac *ApplicationController) createGuestAccount(c *fiber.Ctx, tx pgx.Tx, profileID string, req model.NewApplicationPayload) (userID int, ok bool) {
	ctx := context.Background()

	secret, err := utils.SecureRandString(32)
	if err != nil {
		_ = ac.internalError(c, "Failed to create application", err)
		return 0, false
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		_ = ac.internalError(c, "Failed to create application", err)
		return 0, false
	}

	nameEnc, nameHash, err := utils.EncryptAndHash(ctx, ac.DB, req.FullName)
	if err != nil {
		_ = ac.internalError(c, "Failed to encrypt data", err)
		return 0, false
	}
	emailEnc, emailHash, err := utils.EncryptAndHash(ctx, ac.DB, req.Email)
	if err != nil {
		_ = ac.internalError(c, "Failed to encrypt data", err)
		return 0, false
	}
	phoneEnc, phoneHash, err := utils.EncryptAndHash(ctx, ac.DB, req.Phone)
	if err != nil {
		_ = ac.internalError(c, "Failed to encrypt data", err)
		return 0, false
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO users (full_name_encrypted, full_name_hash, email_encrypted, email_hash, phone_encrypted, phone_hash, password_hash, role)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'applicant')
		RETURNING id
	`, nameEnc, nameHash, emailEnc, emailHash, phoneEnc, phoneHash, string(passwordHash)).Scan(&userID)
	if err != nil {
		if isDup, _ := utils.IsErrDuplicate(err); isDup {
			// Known error - email already registered, no logging. Applying under
			// someone else's account without logging in is not allowed.
			_ = utils.JsonErrorResponse(c, fiber.StatusConflict,
				"An account with this email already exists. Please log in to apply.")
			return 0, false
		}
		_ = ac.internalError(c, "Failed to create application", err)
		return 0, false
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO applicant_profiles (id, user_id, full_name_encrypted, full_name_hash, email_encrypted, email_hash, phone_encrypted, phone_hash)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, profileID, userID, nameEnc, nameHash, emailEnc, emailHash, phoneEnc, phoneHash)
	if err != nil {
		_ = ac.internalError(c, "Failed to create application", err)
		return 0, false
	}
	return userID, true
}

// sendGuestAccessEmail emails a new guest applicant a link to set their
// password (the same single-use token the auth service's reset-password accepts)
// so they can open the portal later. Failures are logged, never returned: the
// application is already saved and they can always use "Forgot password".
func (ac *ApplicationController) sendGuestAccessEmail(ctx context.Context, userID int, name, email string) {
	token, err := utils.SecureRandString(32)
	if err != nil {
		fmt.Printf("WARNING: Failed to generate guest access token: %v\n", err)
		return
	}
	if err := ac.Redis.Set(ctx, utils.PasswordResetKeyPrefix+token, strconv.Itoa(userID), guestAccessTTL).Err(); err != nil {
		fmt.Printf("WARNING: Failed to store guest access token: %v\n", err)
		return
	}
	if err := utils.PublishTemplatedEmailNotification(ctx, ac.Redis, email, "Set your password to track your application", "password_reset", map[string]any{
		"name":       name,
		"email":      email,
		"reset_url":  utils.FrontendResetURL(ac.Config.WebURL, token),
		"expires_in": "7 days",
	}); err != nil {
		fmt.Printf("WARNING: Failed to send guest access email: %v\n", err)
	}
}

// UpdateStatus updates the status of an application (admin only)
func (ac *ApplicationController) UpdateStatus(c *fiber.Ctx) error {
	user, _ := utils.GetUserPayload(c)
	if !user.IsAdmin() {
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

	// Current status plus everything the notifications need (applicant user id,
	// reference, decrypted name and email) in one joined query.
	var currentStatus, reference, applicantEmail, applicantName string
	var currentInterviewAt *time.Time
	var applicantUserID int
	err := ac.DB.QueryRow(ctx, `
		SELECT a.status, a.interview_at, a.reference, ap.user_id,
		       `+utils.DecryptSQL("ap.email_encrypted")+`,
		       `+utils.DecryptSQL("ap.full_name_encrypted")+`
		FROM applications a
		JOIN applicant_profiles ap ON a.applicant_id = ap.id
		WHERE a.id = $1
	`, id).Scan(&currentStatus, &currentInterviewAt, &reference, &applicantUserID, &applicantEmail, &applicantName)
	if err != nil {
		if err == pgx.ErrNoRows {
			return utils.JsonErrorResponse(c, fiber.StatusNotFound, "Application not found")
		}
		return ac.internalError(c, "Database error", err)
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

	// The status change and its history entry are saved together.
	tx, err := ac.DB.Begin(ctx)
	if err != nil {
		return ac.internalError(c, "Failed to update status", err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		UPDATE applications
		SET status = $1, interview_at = $2, updated_at = NOW()
		WHERE id = $3
	`, req.Status, interviewAt, id)
	if err != nil {
		return ac.internalError(c, "Failed to update status", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO application_history (application_id, status, note)
		VALUES ($1, $2, $3)
	`, id, req.Status, req.Note)
	if err != nil {
		return ac.internalError(c, "Failed to create history", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return ac.internalError(c, "Failed to update status", err)
	}

	// Record the status transition in the audit trail (leazi-style activity log).
	utils.RecordSuccessActivityLog(c, ac.DB, user.ID, "APPLICATION_STATUS_UPDATE",
		fmt.Sprintf("Updated application status (application_id=%s, reference=%s) to %s", id, reference, utils.HumanizeStatus(req.Status)),
		map[string]any{
			"application_id": id,
			"reference":      reference,
			"status":         req.Status,
		})

	// Notify the applicant over both channels (in-app + email). The student
	// must learn about a status change regardless of which channel they use.
	// The email is rendered from an HTML template (leazi pattern).
	statusLabel := utils.HumanizeStatus(req.Status)

	// Format the interview date (Africa/Kigali, UTC+2) when one is provided so
	// an interview status change always carries the scheduled date and time.
	interviewLabel := ""
	if interviewAt != nil {
		kigali := time.FixedZone("CAT", 2*60*60)
		interviewLabel = interviewAt.In(kigali).Format("Monday, 02 January 2006 at 15:04")
	}

	notificationTitle := fmt.Sprintf("Application Status Updated: %s", statusLabel)
	notificationMessage := fmt.Sprintf("Your application %s has been updated to %s. %s", reference, statusLabel, req.Note)
	if interviewLabel != "" {
		notificationMessage = fmt.Sprintf("Your application %s has been updated to %s. Interview scheduled for %s. %s", reference, statusLabel, interviewLabel, req.Note)
	}
	if err := utils.PublishInAppNotification(ctx, ac.Redis, applicantUserID, notificationTitle, notificationMessage); err != nil {
		// Log warning but don't fail the update
		fmt.Printf("WARNING: Failed to send status update notification: %v\n", err)
	}
	if applicantEmail != "" {
		if err := utils.PublishTemplatedEmailNotification(ctx, ac.Redis, applicantEmail, notificationTitle, "application_status_updated", map[string]any{
			"name":         applicantName,
			"reference":    reference,
			"status":       statusLabel,
			"note":         req.Note,
			"interview_at": interviewLabel,
		}); err != nil {
			fmt.Printf("WARNING: Failed to send status update email: %v\n", err)
		}
	}

	// Fetch updated application
	applications, err := ac.fetchApplications(ctx, "", id)
	if err != nil {
		return ac.internalError(c, "Failed to fetch application", err)
	}

	return utils.JsonSuccessResponse(c, "Status updated", applications[0])
}

// AddNote adds an internal review note to an application (admin only)
func (ac *ApplicationController) AddNote(c *fiber.Ctx) error {
	user, _ := utils.GetUserPayload(c)
	if !user.IsAdmin() {
		// Known error - forbidden, no logging
		return utils.JsonErrorResponse(c, fiber.StatusForbidden, "Admin access required")
	}

	id := c.Params("id")
	var req model.AddNoteRequest
	authorStr := user.Email

	if err := c.BodyParser(&req); err != nil {
		// Known error - invalid request body, no logging
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Invalid request body")
	}

	if err := model.Validate.Struct(&req); err != nil {
		// Known error - invalid payload, no logging
		return utils.JsonValidationErrorResponse(c, err)
	}

	ctx := context.Background()

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

	// Record the note in the audit trail (leazi-style activity log).
	utils.RecordSuccessActivityLog(c, ac.DB, user.ID, "REVIEW_NOTE_ADD",
		fmt.Sprintf("Added a review note (application_id=%s, user_id=%d)", id, user.ID),
		map[string]any{
			"application_id": id,
			"note_id":        noteID,
		})

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

// Availability returns booked on-site/hybrid seats per start date, duration and
// session for internships that haven't ended yet, paginated
// (?page=1&limit=10, limit at most 100).
func (ac *ApplicationController) Availability(c *fiber.Ctx) error {
	ctx := context.Background()
	page, limit := pageParams(c)

	const groups = `
		FROM applications
		WHERE ` + seatHoldingFilter + `
		  AND start_date + make_interval(months => duration_months) > CURRENT_DATE
		GROUP BY start_date, duration_months, session`

	var total int
	if err := ac.DB.QueryRow(ctx, `SELECT COUNT(g.start_date) FROM (SELECT start_date `+groups+`) g`).Scan(&total); err != nil {
		return ac.internalError(c, "Database error", err)
	}

	rows, err := ac.DB.Query(ctx, `
		SELECT start_date, duration_months, session, COUNT(id)
		`+groups+`
		ORDER BY start_date, session
		LIMIT $1 OFFSET $2
	`, limit, (page-1)*limit)
	if err != nil {
		return ac.internalError(c, "Database error", err)
	}
	defer rows.Close()

	bookings := []model.Booking{}
	for rows.Next() {
		var startDate time.Time
		var durationMonths, count int
		var session string
		if err := rows.Scan(&startDate, &durationMonths, &session, &count); err != nil {
			return ac.internalError(c, "Error scanning row", err)
		}
		bookings = append(bookings, model.Booking{
			StartDate:      startDate.Format(time.RFC3339),
			EndDate:        startDate.AddDate(0, durationMonths, 0).Format(time.RFC3339),
			Session:        session,
			AvailableSeats: model.SessionCapacity - count,
		})
	}

	return utils.JsonPaginatedSuccessResponse(c, "Availability retrieved", bookings,
		utils.GeneratePaginationData(total, limit, page))
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

	// Add LEFT JOIN for packages table if search is active
	if filter.Search != "" {
		whereClause = utils.AppendCondition(whereClause, "LEFT JOIN packages p ON a.package_id = p.id")
	}

	whereClause, params = utils.AppendSearchClause(whereClause, params,
		[]string{"a.reference", "p.name"},
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

// applicationSummary counts the filtered applications by status. The COUNT(a.id)
// also serves as the total used for pagination.
func (ac *ApplicationController) applicationSummary(ctx context.Context, whereClause string, params []any) (model.ApplicationSummary, error) {
	var s model.ApplicationSummary
	err := ac.DB.QueryRow(ctx, `
		SELECT COUNT(a.id),
		       COUNT(a.id) FILTER (WHERE a.status = 'submitted'),
		       COUNT(a.id) FILTER (WHERE a.status = 'under_review'),
		       COUNT(a.id) FILTER (WHERE a.status = 'shortlisted'),
		       COUNT(a.id) FILTER (WHERE a.status = 'interview'),
		       COUNT(a.id) FILTER (WHERE a.status = 'accepted'),
		       COUNT(a.id) FILTER (WHERE a.status = 'rejected')
		FROM applications a
		JOIN applicant_profiles ap ON a.applicant_id = ap.id
	`+whereClause, params...).Scan(
		&s.Total, &s.Submitted, &s.UnderReview, &s.Shortlisted,
		&s.Interview, &s.Accepted, &s.Rejected,
	)
	return s, err
}

// applicationsByPackage aggregates the filtered applications by package,
// most applications first.
func (ac *ApplicationController) applicationsByPackage(ctx context.Context, whereClause string, params []any) ([]model.PackageCount, error) {
	rows, err := ac.DB.Query(ctx, `
		SELECT a.package_id, p.name as package_name, COUNT(a.id)
		FROM applications a
		JOIN applicant_profiles ap ON a.applicant_id = ap.id
		LEFT JOIN packages p ON a.package_id = p.id
	`+whereClause+`
		GROUP BY a.package_id, p.name
		ORDER BY COUNT(a.id) DESC, a.package_id ASC
	`, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := []model.PackageCount{}
	for rows.Next() {
		var pc model.PackageCount
		if err := rows.Scan(&pc.PackageID, &pc.PackageName, &pc.Count); err != nil {
			return nil, err
		}
		counts = append(counts, pc)
	}
	return counts, nil
}

// applicationTrends returns the number of filtered applications submitted in
// each of the last `months` months, oldest first.
func (ac *ApplicationController) applicationTrends(ctx context.Context, whereClause string, params []any, months int) ([]model.TrendPoint, error) {
	rows, err := ac.DB.Query(ctx, `
		SELECT to_char(date_trunc('month', a.submitted_at), 'YYYY-MM'), COUNT(a.id)
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

// applicationSelect is the shared projection for application rows. Applicant
// PII is decrypted in the query itself so a list costs one round trip instead
// of three extra per row.
var applicationSelect = `
	SELECT a.id, a.reference, a.applicant_id, a.package_id, p.name as package_name,
	       a.department, a.specialization, a.skills,
	       a.motivation, a.experience, a.start_date, a.education_level,
	       a.session, a.duration_months, a.work_mode, a.amount, a.status,
	       a.submitted_at, a.interview_at,
	       ` + utils.DecryptSQL("ap.full_name_encrypted") + `,
	       ` + utils.DecryptSQL("ap.email_encrypted") + `,
	       ` + utils.DecryptSQL("ap.phone_encrypted") + `,
	       COALESCE(ap.country, ''), COALESCE(ap.city, ''),
	       COALESCE(ap.institution, ''), COALESCE(ap.field_of_study, ''),
	       COALESCE(ap.qualification, ''), COALESCE(ap.academic_status, ''),
	       ap.graduation_year, COALESCE(ap.skills, '{}'), COALESCE(ap.bio, ''),
	       COALESCE(ap.github_url, ''), COALESCE(ap.linkedin_url, ''),
	       COALESCE(ap.portfolio_url, '')
	FROM applications a
	JOIN applicant_profiles ap ON a.applicant_id = ap.id
	LEFT JOIN packages p ON a.package_id = p.id
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
	applications, err := scanApplications(rows)
	if err != nil {
		return nil, err
	}
	if err := ac.attachChildren(ctx, applications); err != nil {
		return nil, err
	}
	return applications, nil
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

// scanApplications reads every row (closing rows) before any further query
// runs, so the pool connection is released before the child queries.
func scanApplications(rows pgx.Rows) ([]model.Application, error) {
	defer rows.Close()
	applications := []model.Application{}
	for rows.Next() {
		var app model.Application
		var startDate *time.Time
		var submittedAt time.Time
		var interviewAt *time.Time

		err := rows.Scan(
			&app.ID, &app.Reference, &app.Applicant.ID, &app.PackageID, &app.PackageName,
			&app.Department, &app.Specialization, &app.Skills,
			&app.Motivation, &app.Experience, &startDate, &app.EducationLevel,
			&app.Session, &app.DurationMonths, &app.WorkMode, &app.Amount, &app.Status,
			&submittedAt, &interviewAt,
			&app.Applicant.FullName, &app.Applicant.Email, &app.Applicant.Phone,
			&app.Applicant.Country, &app.Applicant.City,
			&app.Applicant.Institution, &app.Applicant.FieldOfStudy,
			&app.Applicant.Qualification, &app.Applicant.AcademicStatus,
			&app.Applicant.GraduationYear, &app.Applicant.Skills, &app.Applicant.Bio,
			&app.Applicant.GithubURL, &app.Applicant.LinkedinURL,
			&app.Applicant.PortfolioURL,
		)
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
		app.Documents = []model.ApplicationDocument{}
		app.History = []model.StatusEvent{}
		app.Notes = []model.ReviewNote{}

		applications = append(applications, app)
	}
	return applications, rows.Err()
}

// attachChildren loads documents, history and notes for all the given
// applications with one query each (instead of three queries per row).
func (ac *ApplicationController) attachChildren(ctx context.Context, applications []model.Application) error {
	if len(applications) == 0 {
		return nil
	}
	ids := make([]string, len(applications))
	index := make(map[string]*model.Application, len(applications))
	for i := range applications {
		ids[i] = applications[i].ID
		index[applications[i].ID] = &applications[i]
	}

	// Documents
	rows, err := ac.DB.Query(ctx, `
		SELECT source_table_id, id, file_type, name, file_size, COALESCE(uploaded_at, created_at)
		FROM files
		WHERE source_table = 'applications' AND source_table_id = ANY($1) AND deleted_at IS NULL
		ORDER BY created_at
	`, ids)
	if err != nil {
		return err
	}
	for rows.Next() {
		var appID string
		var doc model.ApplicationDocument
		var uploadedAt time.Time
		if err := rows.Scan(&appID, &doc.ID, &doc.Type, &doc.Name, &doc.Size, &uploadedAt); err != nil {
			rows.Close()
			return err
		}
		doc.UploadedAt = uploadedAt.Format(time.RFC3339)
		if app, ok := index[appID]; ok {
			app.Documents = append(app.Documents, doc)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	// Status history, newest first
	rows, err = ac.DB.Query(ctx, `
		SELECT application_id, status, at, COALESCE(note, '')
		FROM application_history
		WHERE application_id = ANY($1)
		ORDER BY at DESC
	`, ids)
	if err != nil {
		return err
	}
	for rows.Next() {
		var appID string
		var event model.StatusEvent
		var at time.Time
		if err := rows.Scan(&appID, &event.Status, &at, &event.Note); err != nil {
			rows.Close()
			return err
		}
		event.At = at.Format(time.RFC3339)
		if app, ok := index[appID]; ok {
			app.History = append(app.History, event)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	// Internal review notes, newest first (callers strip them for applicants)
	rows, err = ac.DB.Query(ctx, `
		SELECT application_id, id, author, text, at
		FROM review_notes
		WHERE application_id = ANY($1)
		ORDER BY at DESC
	`, ids)
	if err != nil {
		return err
	}
	for rows.Next() {
		var appID string
		var note model.ReviewNote
		var at time.Time
		if err := rows.Scan(&appID, &note.ID, &note.Author, &note.Text, &at); err != nil {
			rows.Close()
			return err
		}
		note.At = at.Format(time.RFC3339)
		if app, ok := index[appID]; ok {
			app.Notes = append(app.Notes, note)
		}
	}
	rows.Close()
	return rows.Err()
}

// seatHoldingFilter matches applications that occupy a seat in a daily
// session: on-site/hybrid, not rejected, with a start date. Shared by the
// submit-time capacity check and the availability endpoint.
const seatHoldingFilter = `work_mode IN ('onsite', 'hybrid') AND status <> 'rejected' AND start_date IS NOT NULL`

// applicantRef is the caller's applicant profile id with decrypted contact
// details.
type applicantRef struct {
	ProfileID string
	Name      string
	Email     string
}

// applicantByUser loads a user's profile id, name and email in one query
// (decrypted in SQL). Returns pgx.ErrNoRows when the user has no profile.
func (ac *ApplicationController) applicantByUser(ctx context.Context, userID int) (applicantRef, error) {
	var ref applicantRef
	err := ac.DB.QueryRow(ctx, `
		SELECT id, `+utils.DecryptSQL("full_name_encrypted")+`, `+utils.DecryptSQL("email_encrypted")+`
		FROM applicant_profiles WHERE user_id = $1
	`, userID).Scan(&ref.ProfileID, &ref.Name, &ref.Email)
	return ref, err
}

// internalError answers 500 with a short message and logs the cause as
// CRITICAL (the shared pattern for unexpected failures).
func (ac *ApplicationController) internalError(c *fiber.Ctx, message string, err error) error {
	return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, message, utils.Logger{
		LogLevel:    utils.CRITICAL,
		Message:     err.Error(),
		ServiceName: ac.Config.ServiceName,
	})
}

// stripNotes removes internal review notes, which only admins may see.
func stripNotes(applications []model.Application) {
	for i := range applications {
		applications[i].Notes = []model.ReviewNote{}
	}
}

// uniqueStrings returns the values without duplicates, keeping their order.
func uniqueStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

// pageParams reads ?page and ?limit with the shared defaults (page 1,
// limit 10, limit at most 100).
func pageParams(c *fiber.Ctx) (page, limit int) {
	return utils.PageParams(c)
}

// notifySubmission sends the post-submit notifications (set-password email for
// a new guest, applicant in-app + email, admin fan-out). It is best-effort and
// runs in the background with its own deadline, so failures are only logged.
// Emails are rendered from HTML templates (leazi pattern).
func (ac *ApplicationController) notifySubmission(userID int, applicant applicantRef, req model.NewApplicationPayload, reference string, guest bool) {
	notifySlots <- struct{}{}
	defer func() { <-notifySlots }()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if guest {
		ac.sendGuestAccessEmail(ctx, userID, req.FullName, req.Email)
	}

	// Lookup package details for notification
	pkg, err := utils.LookupPackage(ctx, ac.DB, req.PackageID)
	if err != nil {
		fmt.Printf("WARNING: Failed to lookup package for notification: %v\n", err)
		pkg = &utils.Package{Name: "Unknown Package"}
	}

	applicantName := applicant.Name
	if applicantName == "" {
		applicantName = "An applicant"
	}

	applicantSubject := "Application Submitted Successfully"
	applicantMessage := fmt.Sprintf("Your application for %s has been submitted successfully and is awaiting review. You will be notified once it has been reviewed. Reference: %s", pkg.Name, reference)
	if err := utils.PublishInAppNotification(ctx, ac.Redis, userID, applicantSubject, applicantMessage); err != nil {
		fmt.Printf("WARNING: Failed to send application submission notification: %v\n", err)
	}
	if applicant.Email != "" {
		if err := utils.PublishTemplatedEmailNotification(ctx, ac.Redis, applicant.Email, applicantSubject, "application_submitted", map[string]any{
			"name":      applicantName,
			"reference": reference,
			"package":   pkg.Name,
		}); err != nil {
			fmt.Printf("WARNING: Failed to send application submission email: %v\n", err)
		}
	}

	adminSubject := "New Application Submitted"
	adminMessage := fmt.Sprintf("%s submitted an application for %s. Reference: %s", applicantName, pkg.Name, reference)
	if err := utils.NotifyAdmins(ctx, ac.DB, ac.Redis, adminSubject, adminMessage, "admin_new_application", map[string]any{
		"applicant_name": applicantName,
		"reference":      reference,
		"package":        pkg.Name,
		"login_url":      ac.Config.WebURL,
	}); err != nil {
		fmt.Printf("WARNING: Failed to notify admins of new application: %v\n", err)
	}
}

// notifySlots is a semaphore capping how many submissions notify at once, so a
// burst of applications cannot open unbounded Redis/DB work in the background.
var notifySlots = make(chan struct{}, 10)
