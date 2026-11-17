package controller

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qonics/qonics-appy-api/libs/shared-package/config"
	"github.com/qonics/qonics-appy-api/libs/shared-package/utils"
	"github.com/qonics/qonics-appy-api/services/applicant/model"
)

type ApplicantController struct {
	DB     *pgxpool.Pool
	Config *config.Config
}

func NewApplicantController(db *pgxpool.Pool, cfg *config.Config) *ApplicantController {
	return &ApplicantController{
		DB:     db,
		Config: cfg,
	}
}

// GetProfile returns the current user's profile
func (ac *ApplicantController) GetProfile(c *fiber.Ctx) error {
	userID := c.Locals("user_id")

	// Check if user is authenticated
	if userID == nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, "Unauthorized")
	}

	ctx := context.Background()

	var profile model.ApplicantProfile
	var fullNameEncrypted, emailEncrypted, phoneEncrypted []byte

	// Convert userID to int for database query
	userIDInt := int(userID.(float64))

	err := ac.DB.QueryRow(ctx, `
		SELECT id, full_name_encrypted, email_encrypted, phone_encrypted,
		       COALESCE(country, ''), COALESCE(city, ''), COALESCE(institution, ''),
		       COALESCE(field_of_study, ''), COALESCE(qualification, ''),
		       COALESCE(academic_status, ''), graduation_year,
		       COALESCE(skills, ARRAY[]::TEXT[]), COALESCE(bio, ''),
		       COALESCE(github_url, ''), COALESCE(linkedin_url, ''), COALESCE(portfolio_url, '')
		FROM applicant_profiles WHERE user_id = $1
	`, userIDInt).Scan(
		&profile.ID, &fullNameEncrypted, &emailEncrypted, &phoneEncrypted,
		&profile.Country, &profile.City, &profile.Institution, &profile.FieldOfStudy,
		&profile.Qualification, &profile.AcademicStatus, &profile.GraduationYear,
		&profile.Skills, &profile.Bio, &profile.GithubUrl, &profile.LinkedinUrl, &profile.PortfolioUrl,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			// Known error - profile not found, no logging
			return utils.JsonErrorResponse(c, fiber.StatusNotFound, "Applicant profile not found. Please complete your profile first.")
		}
		// Unknown error - log as critical
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Database error", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}

	// Decrypt PII using pgcrypto
	profile.FullName, err = utils.DecryptData(ctx, ac.DB, fullNameEncrypted)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to decrypt data", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}

	profile.Email, err = utils.DecryptData(ctx, ac.DB, emailEncrypted)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to decrypt data", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}

	profile.Phone, err = utils.DecryptData(ctx, ac.DB, phoneEncrypted)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to decrypt data", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}

	return utils.JsonSuccessResponse(c, "Profile retrieved", profile)
}

// SaveProfile updates the current user's profile
func (ac *ApplicantController) SaveProfile(c *fiber.Ctx) error {
	userID := c.Locals("user_id")

	// Check if user is authenticated
	if userID == nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, "Unauthorized")
	}

	var req model.ApplicantProfile

	if err := c.BodyParser(&req); err != nil {
		// Known error - invalid request body, no logging
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Invalid request body")
	}

	ctx := context.Background()

	// Convert userID to int for database query
	userIDInt := int(userID.(float64))

	// Get existing profile to check for changes
	var existingFullNameEncrypted, existingEmailEncrypted, existingPhoneEncrypted []byte
	var existingSkillsArray []string
	var existingCountry, existingCity, existingInstitution, existingFieldOfStudy string
	var existingQualification, existingAcademicStatus string
	var existingGraduationYear *int
	var existingBio, existingGithubUrl, existingLinkedinUrl, existingPortfolioUrl string

	err := ac.DB.QueryRow(ctx, `
		SELECT full_name_encrypted, email_encrypted, phone_encrypted,
		       COALESCE(country, ''), COALESCE(city, ''), COALESCE(institution, ''),
		       COALESCE(field_of_study, ''), COALESCE(qualification, ''),
		       COALESCE(academic_status, ''), graduation_year,
		       COALESCE(skills, ARRAY[]::TEXT[]), COALESCE(bio, ''),
		       COALESCE(github_url, ''), COALESCE(linkedin_url, ''), COALESCE(portfolio_url, '')
		FROM applicant_profiles WHERE user_id = $1
	`, userIDInt).Scan(
		&existingFullNameEncrypted, &existingEmailEncrypted, &existingPhoneEncrypted,
		&existingCountry, &existingCity, &existingInstitution, &existingFieldOfStudy,
		&existingQualification, &existingAcademicStatus, &existingGraduationYear,
		&existingSkillsArray, &existingBio, &existingGithubUrl, &existingLinkedinUrl, &existingPortfolioUrl,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			// Profile doesn't exist yet, proceed with insert instead
			// For now, return error - profile should be created on registration
			return utils.JsonErrorResponse(c, fiber.StatusNotFound, "Profile not found")
		}
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Database error", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}

	// Decrypt existing PII for comparison
	existingFullName, _ := utils.DecryptData(ctx, ac.DB, existingFullNameEncrypted)
	existingEmail, _ := utils.DecryptData(ctx, ac.DB, existingEmailEncrypted)
	existingPhone, _ := utils.DecryptData(ctx, ac.DB, existingPhoneEncrypted)

	// Check if any field is actually changing
	changed := false
	if existingFullName != req.FullName ||
		existingEmail != req.Email ||
		existingPhone != req.Phone ||
		existingCountry != req.Country ||
		existingCity != req.City ||
		existingInstitution != req.Institution ||
		existingFieldOfStudy != req.FieldOfStudy ||
		existingQualification != req.Qualification ||
		existingAcademicStatus != req.AcademicStatus {
		changed = true
	}

	// Handle GraduationYear comparison (nullable int)
	if !changed {
		if req.GraduationYear == nil && existingGraduationYear != nil {
			changed = true
		} else if req.GraduationYear != nil && existingGraduationYear == nil {
			changed = true
		} else if req.GraduationYear != nil && existingGraduationYear != nil && *req.GraduationYear != *existingGraduationYear {
			changed = true
		}
	}

	if !changed {
		if existingBio != req.Bio ||
			existingGithubUrl != req.GithubUrl ||
			existingLinkedinUrl != req.LinkedinUrl ||
			existingPortfolioUrl != req.PortfolioUrl {
			changed = true
		}
	}

	// Compare skills arrays
	if !changed {
		if len(existingSkillsArray) != len(req.Skills) {
			changed = true
		} else {
			for i, skill := range existingSkillsArray {
				if skill != req.Skills[i] {
					changed = true
					break
				}
			}
		}
	}

	if !changed {
		return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "No changes detected in profile")
	}

	// Encrypt PII using pgcrypto
	fullNameEncrypted, fullNameHash, err := utils.EncryptAndHash(ctx, ac.DB, req.FullName)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to encrypt data", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}

	emailEncrypted, emailHash, err := utils.EncryptAndHash(ctx, ac.DB, req.Email)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to encrypt data", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}

	phoneEncrypted, phoneHash, err := utils.EncryptAndHash(ctx, ac.DB, req.Phone)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to encrypt data", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}

	// Update profile (use NULL for empty strings to distinguish from set values)
	_, err = ac.DB.Exec(ctx, `
		UPDATE applicant_profiles
		SET full_name_encrypted = $1, full_name_hash = $2,
		    email_encrypted = $3, email_hash = $4,
		    phone_encrypted = $5, phone_hash = $6,
		    country = NULLIF($7, ''), city = NULLIF($8, ''), institution = NULLIF($9, ''),
		    field_of_study = NULLIF($10, ''), qualification = NULLIF($11, ''),
		    academic_status = NULLIF($12, ''), graduation_year = $13,
		    skills = $14, bio = NULLIF($15, ''),
		    github_url = NULLIF($16, ''), linkedin_url = NULLIF($17, ''), portfolio_url = NULLIF($18, ''),
		    updated_at = NOW()
		WHERE user_id = $19
	`, fullNameEncrypted, fullNameHash, emailEncrypted, emailHash,
		phoneEncrypted, phoneHash, req.Country, req.City, req.Institution,
		req.FieldOfStudy, req.Qualification, req.AcademicStatus,
		req.GraduationYear, req.Skills, req.Bio,
		req.GithubUrl, req.LinkedinUrl, req.PortfolioUrl, userIDInt)

	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to update profile", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}

	return utils.JsonSuccessResponse(c, "Profile updated", req)
}

// ListApplicants returns all applicants (admin only)
func (ac *ApplicantController) ListApplicants(c *fiber.Ctx) error {
	ctx := context.Background()

	rows, err := ac.DB.Query(ctx, `
		SELECT ap.id, ap.full_name_encrypted, ap.email_encrypted, ap.phone_encrypted,
		       ap.country, ap.city, ap.institution, ap.field_of_study,
		       ap.qualification, ap.academic_status, ap.graduation_year,
		       ap.skills, ap.bio, ap.github_url, ap.linkedin_url, ap.portfolio_url,
		       u.created_at,
		       COALESCE(COUNT(a.id), 0) as total_applications
		FROM applicant_profiles ap
		JOIN users u ON ap.user_id = u.id
		LEFT JOIN applications a ON ap.id = a.applicant_id
		GROUP BY ap.id, u.created_at
		ORDER BY u.created_at DESC
	`)

	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Database error", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}
	defer rows.Close()

	var applicants []model.ApplicantSummary
	for rows.Next() {
		var summary model.ApplicantSummary
		var fullNameEncrypted, emailEncrypted, phoneEncrypted []byte
		var skillsArray []string
		var registeredAt time.Time

		err := rows.Scan(
			&summary.ID, &fullNameEncrypted, &emailEncrypted, &phoneEncrypted,
			&summary.Country, &summary.City, &summary.Institution, &summary.FieldOfStudy,
			&summary.Qualification, &summary.AcademicStatus, &summary.GraduationYear,
			&skillsArray, &summary.Bio, &summary.GithubUrl, &summary.LinkedinUrl, &summary.PortfolioUrl,
			&registeredAt, &summary.TotalApplications,
		)

		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Error scanning row", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     err.Error(),
				ServiceName: ac.Config.ServiceName,
			})
		}

		// Decrypt PII using pgcrypto
		summary.FullName, err = utils.DecryptData(ctx, ac.DB, fullNameEncrypted)
		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to decrypt data", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     err.Error(),
				ServiceName: ac.Config.ServiceName,
			})
		}

		summary.Email, err = utils.DecryptData(ctx, ac.DB, emailEncrypted)
		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to decrypt data", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     err.Error(),
				ServiceName: ac.Config.ServiceName,
			})
		}

		summary.Phone, err = utils.DecryptData(ctx, ac.DB, phoneEncrypted)
		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to decrypt data", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     err.Error(),
				ServiceName: ac.Config.ServiceName,
			})
		}

		summary.Skills = skillsArray
		summary.RegisteredAt = registeredAt.Format(time.RFC3339)

		applicants = append(applicants, summary)
	}

	return utils.JsonSuccessResponse(c, "Applicants retrieved", applicants)
}

// ListNotifications returns notifications for the current user
func (ac *ApplicantController) ListNotifications(c *fiber.Ctx) error {
	userID := c.Locals("user_id")
	ctx := context.Background()

	// Convert userID to int for database query
	userIDInt := int(userID.(float64))
	rows, err := ac.DB.Query(ctx, `
		SELECT id, title, message, at, read
		FROM notifications WHERE user_id = $1
		ORDER BY at DESC
	`, userIDInt)

	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Database error", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}
	defer rows.Close()

	var notifications []model.AppNotification
	for rows.Next() {
		var notif model.AppNotification
		var at time.Time

		err := rows.Scan(&notif.ID, &notif.Title, &notif.Message, &at, &notif.Read)
		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Error scanning row", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     err.Error(),
				ServiceName: ac.Config.ServiceName,
			})
		}

		notif.At = at.Format(time.RFC3339)
		notifications = append(notifications, notif)
	}

	return utils.JsonSuccessResponse(c, "Notifications retrieved", notifications)
}

// MarkNotificationsRead marks notifications as read
func (ac *ApplicantController) MarkNotificationsRead(c *fiber.Ctx) error {
	userID := c.Locals("user_id")
	var req model.MarkNotificationsReadRequest

	if err := c.BodyParser(&req); err != nil {
		// Known error - invalid request body, no logging
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Invalid request body")
	}

	ctx := context.Background()

	// Convert userID to int for database query
	userIDInt := int(userID.(float64))

	// Check if notification IDs belong to the user (foreign key validation)
	var count int
	err := ac.DB.QueryRow(ctx, `
		SELECT COUNT(id) FROM notifications
		WHERE user_id = $1 AND id = ANY($2)
	`, userIDInt, req.IDs).Scan(&count)

	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to verify notifications", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}

	if count != len(req.IDs) {
		// Known error - some notifications don't belong to user, no logging
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "One or more notifications not found")
	}

	// Check if notifications are already marked as read
	var alreadyReadCount int
	err = ac.DB.QueryRow(ctx, `
		SELECT COUNT(id) FROM notifications
		WHERE user_id = $1 AND id = ANY($2) AND read = TRUE
	`, userIDInt, req.IDs).Scan(&alreadyReadCount)

	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to check notification status", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}

	if alreadyReadCount > 0 {
		// Known error - some notifications already read, no logging
		return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "One or more notifications are already marked as read")
	}

	_, err = ac.DB.Exec(ctx, `
		UPDATE notifications SET read = TRUE
		WHERE user_id = $1 AND id = ANY($2)
	`, userIDInt, req.IDs)

	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to mark notifications as read", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}

	return utils.JsonSuccessResponse(c, "Notifications marked as read", nil)
}

// GetSettings returns application settings
func (ac *ApplicantController) GetSettings(c *fiber.Ctx) error {
	ctx := context.Background()

	var settings model.AppSettings
	var closeOn *time.Time

	err := ac.DB.QueryRow(ctx, `
		SELECT applications_open, close_on, closure_message
		FROM settings WHERE id = 1
	`).Scan(&settings.ApplicationsOpen, &closeOn, &settings.ClosureMessage)

	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Database error", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}

	if closeOn != nil {
		settings.CloseOn = closeOn.Format(time.RFC3339)
	}

	return utils.JsonSuccessResponse(c, "Settings retrieved", settings)
}

// SaveSettings updates application settings (admin only)
func (ac *ApplicantController) SaveSettings(c *fiber.Ctx) error {
	var req model.AppSettings

	if err := c.BodyParser(&req); err != nil {
		// Known error - invalid request body, no logging
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Invalid request body")
	}

	ctx := context.Background()

	// Get existing settings
	var existingApplicationsOpen bool
	var existingCloseOn *time.Time
	var existingClosureMessage string

	err := ac.DB.QueryRow(ctx, `
		SELECT applications_open, close_on, closure_message
		FROM settings WHERE id = 1
	`).Scan(&existingApplicationsOpen, &existingCloseOn, &existingClosureMessage)

	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Database error", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}

	var closeOn *time.Time
	if req.CloseOn != "" {
		parsedTime, err := time.Parse(time.RFC3339, req.CloseOn)
		if err != nil {
			// Known error - invalid date format, no logging
			return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Invalid date format")
		}
		closeOn = &parsedTime
	}

	// Check if any value is actually changing
	changed := false
	if existingApplicationsOpen != req.ApplicationsOpen ||
		existingClosureMessage != req.ClosureMessage {
		changed = true
	}

	// Compare close_on timestamps
	if (existingCloseOn == nil && closeOn != nil) ||
		(existingCloseOn != nil && closeOn == nil) ||
		(existingCloseOn != nil && closeOn != nil && !existingCloseOn.Equal(*closeOn)) {
		changed = true
	}

	if !changed {
		return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "No changes detected in settings")
	}

	_, err = ac.DB.Exec(ctx, `
		UPDATE settings
		SET applications_open = $1, close_on = $2, closure_message = $3, updated_at = NOW()
		WHERE id = 1
	`, req.ApplicationsOpen, closeOn, req.ClosureMessage)

	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to update settings", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}

	return utils.JsonSuccessResponse(c, "Settings updated", req)
}
