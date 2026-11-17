package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qonics/qonics-appy-api/libs/shared-package/config"
	"github.com/qonics/qonics-appy-api/libs/shared-package/utils"
	"github.com/qonics/qonics-appy-api/services/auth/model"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

type AuthController struct {
	DB     *pgxpool.Pool
	Config *config.Config
	Redis  *redis.Client
	Logger interface{} // Will be zap logger
}

func NewAuthController(db *pgxpool.Pool, cfg *config.Config, rdb *redis.Client) *AuthController {
	return &AuthController{
		DB:     db,
		Config: cfg,
		Redis:  rdb,
	}
}

// Register handles user registration
func (ac *AuthController) Register(c *fiber.Ctx) error {
	var req model.RegisterRequest
	if err := c.BodyParser(&req); err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Invalid request body")
	}

	if err := model.Validate.Struct(&req); err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Validation failed")
	}

	ctx := context.Background()

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to hash password", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
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

	// Insert user
	var userID int
	err = ac.DB.QueryRow(ctx, `
		INSERT INTO users (full_name_encrypted, full_name_hash, email_encrypted, email_hash, phone_encrypted, phone_hash, password_hash, role)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id
	`, fullNameEncrypted, fullNameHash, emailEncrypted, emailHash, phoneEncrypted, phoneHash, string(hashedPassword), "applicant").Scan(&userID)

	if err != nil {
		// Check for duplicate key error - known error, no logging
		if isDup, keyName := utils.IsErrDuplicate(err); isDup {
			return utils.JsonErrorResponse(c, fiber.StatusConflict, utils.GetDuplicateKeyMessage(keyName))
		}
		// Check for foreign key error - known error, no logging
		if isFK, tableName := utils.IsForeignKeyErr(err); isFK {
			return utils.JsonErrorResponse(c, fiber.StatusBadRequest, utils.GetForeignKeyMessage(tableName))
		}
		// Unknown error - log as critical
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to create user", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}

	// Create applicant profile
	profileID := uuid.New().String()
	_, err = ac.DB.Exec(ctx, `
		INSERT INTO applicant_profiles (id, user_id, full_name_encrypted, full_name_hash, email_encrypted, email_hash, phone_encrypted, phone_hash)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, profileID, userID, fullNameEncrypted, fullNameHash, emailEncrypted, emailHash, phoneEncrypted, phoneHash)

	if err != nil {
		// Check for foreign key error - known error, no logging
		if isFK, tableName := utils.IsForeignKeyErr(err); isFK {
			return utils.JsonErrorResponse(c, fiber.StatusBadRequest, utils.GetForeignKeyMessage(tableName))
		}
		// Unknown error - log as critical
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to create profile", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}

	// Generate JWT token
	token, err := ac.generateJWT(userID, req.Email, "applicant")
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to generate token", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}

	// Set auth cookie
	utils.SetAuthCookie(c, token, utils.COOKIE_MAX_AGE)

	// Log successful registration
	utils.RecordSuccessActivityLog(c, ac.DB, userID, "USER_REGISTER", "User registered successfully", nil)

	// Send welcome email notification
	welcomeSubject := "Welcome to Qonics Internship Program"
	welcomeBody := fmt.Sprintf("Dear %s,\n\nWelcome to the Qonics Internship Program! Your account has been successfully created.\n\nYou can now log in and start your application process.\n\nBest regards,\nThe Qonics Team", req.FullName)
	if err := utils.PublishEmailNotification(ctx, ac.Redis, req.Email, welcomeSubject, welcomeBody, false); err != nil {
		// Log warning but don't fail the registration
		// TODO: Use proper logger when available
		fmt.Printf("WARNING: Failed to send welcome email: %v\n", err)
	}

	// Send in-app notification
	if err := utils.PublishInAppNotification(ctx, ac.Redis, userID, "Welcome!", "Your account has been successfully created. Start your application now!"); err != nil {
		// Log warning but don't fail the registration
		fmt.Printf("WARNING: Failed to send in-app notification: %v\n", err)
	}

	return utils.JsonSuccessResponse(c, "Registration successful", model.Session{
		Token: token,
		User: model.User{
			ID:       userID,
			FullName: req.FullName,
			Email:    req.Email,
			Phone:    req.Phone,
			Role:     "applicant",
		},
	})
}

// Login handles user login
func (ac *AuthController) Login(c *fiber.Ctx) error {
	var req model.LoginRequest
	if err := c.BodyParser(&req); err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Invalid request body")
	}

	if err := model.Validate.Struct(&req); err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Validation failed")
	}

	ctx := context.Background()
	emailHash := utils.HashData(req.Email)
	email := req.Email // Store for later use

	// Get user with encrypted data
	var userID int
	var fullNameEncrypted, emailEncrypted, phoneEncrypted []byte
	var passwordHash, role string
	err := ac.DB.QueryRow(ctx, `
		SELECT id, full_name_encrypted, email_encrypted, phone_encrypted, password_hash, role
		FROM users WHERE email_hash = $1
	`, emailHash).Scan(&userID, &fullNameEncrypted, &emailEncrypted, &phoneEncrypted, &passwordHash, &role)

	if err != nil {
		if err == pgx.ErrNoRows {
			// Known error - user not found, no logging
			return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, "Invalid email or password")
		}
		// Unknown error - log as critical
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Database error", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
		// Known error - wrong password, no logging
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, "Invalid email or password")
	}

	// Decrypt PII using pgcrypto
	fullName, err := utils.DecryptData(ctx, ac.DB, fullNameEncrypted)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to decrypt data", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}

	// email is already available from request
	phone, err := utils.DecryptData(ctx, ac.DB, phoneEncrypted)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to decrypt data", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}

	// Generate JWT token
	token, err := ac.generateJWT(userID, email, role)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to generate token", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}

	// Set auth cookie
	utils.SetAuthCookie(c, token, utils.COOKIE_MAX_AGE)

	// Set device cookie if not present
	if c.Cookies(utils.DEVICE_COOKIE_NAME) == "" {
		deviceID := utils.GenerateDeviceID()
		utils.SetDeviceCookie(c, deviceID)
	}

	// Log successful login
	utils.RecordSuccessActivityLog(c, ac.DB, userID, "USER_LOGIN", "User logged in successfully", nil)

	return utils.JsonSuccessResponse(c, "Login successful", model.Session{
		Token: token,
		User: model.User{
			ID:       userID,
			FullName: fullName,
			Email:    email,
			Phone:    phone,
			Role:     role,
		},
	})
}

// Logout handles user logout
func (ac *AuthController) Logout(c *fiber.Ctx) error {
	userID := c.Locals("user_id")

	// Clear auth cookies
	utils.ClearAuthCookies(c)

	// Log successful logout
	if userID != nil {
		utils.RecordSuccessActivityLog(c, ac.DB, userID.(int), "USER_LOGOUT", "User logged out successfully", nil)
	}

	return utils.JsonSuccessResponse(c, "Logout successful", nil)
}

// ForgotPassword handles password reset request
func (ac *AuthController) ForgotPassword(c *fiber.Ctx) error {
	var req model.ForgotPasswordRequest
	if err := c.BodyParser(&req); err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Invalid request body")
	}

	// In a real implementation, send email with reset token
	// For now, just return success
	return utils.JsonSuccessResponse(c, "If an account exists with this email, a password reset link has been sent", nil)
}

// ResetPassword handles password reset
func (ac *AuthController) ResetPassword(c *fiber.Ctx) error {
	var req model.ResetPasswordRequest
	if err := c.BodyParser(&req); err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Invalid request body")
	}

	// In a real implementation, validate token and update password
	return utils.JsonSuccessResponse(c, "Password reset successful", nil)
}

// ChangePassword handles password change for authenticated users
func (ac *AuthController) ChangePassword(c *fiber.Ctx) error {
	var req model.ChangePasswordRequest
	if err := c.BodyParser(&req); err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Invalid request body")
	}

	userID := c.Locals("user_id")

	// Type guards
	if userID == nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, "Unauthorized")
	}

	userIDInt, ok := userID.(float64)
	if !ok {
		// Try int type as well
		userIDInt2, ok2 := userID.(int)
		if !ok2 {
			return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, "Unauthorized")
		}
		userIDInt = float64(userIDInt2)
	}

	ctx := context.Background()

	// Get current password hash
	var passwordHash string
	err := ac.DB.QueryRow(ctx, "SELECT password_hash FROM users WHERE id = $1", int(userIDInt)).Scan(&passwordHash)
	if err != nil {
		if err == pgx.ErrNoRows {
			return utils.JsonErrorResponse(c, fiber.StatusNotFound, "User not found")
		}
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Database error", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}

	// Verify current password
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.CurrentPassword)); err != nil {
		// Known error - wrong password, no logging
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Current password is incorrect")
	}

	// Check if new password is different from current password
	if req.NewPassword == req.CurrentPassword {
		return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "New password must be different from current password")
	}

	// Hash new password
	newHashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to hash password", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}

	// Update password
	_, err = ac.DB.Exec(ctx, "UPDATE users SET password_hash = $1 WHERE id = $2", string(newHashedPassword), int(userIDInt))
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Failed to update password", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: ac.Config.ServiceName,
		})
	}

	// Log successful password change
	utils.RecordSuccessActivityLog(c, ac.DB, int(userIDInt), "PASSWORD_CHANGE", "User changed password successfully", nil)

	return utils.JsonSuccessResponse(c, "Password changed successfully", nil)
}

// Helper functions

func (ac *AuthController) generateJWT(userID int, email string, role string) (string, error) {
	claims := jwt.MapClaims{
		"user_id": userID,
		"email":   email,
		"role":    role,
		"exp":     time.Now().Add(time.Hour * 24 * 7).Unix(), // 7 days
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(ac.Config.JWT.Secret))
}

// hashString creates a SHA256 hash (helper function)
func hashString(s string) string {
	return utils.HashData(s)
}
