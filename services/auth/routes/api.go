package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/qonics/qonics-appy-api/libs/shared-package/middleware"
	"github.com/qonics/qonics-appy-api/services/auth/controller"
)

func SetupRoutes(app *fiber.App, authController *controller.AuthController, jwtSecret string) {
	api := app.Group("/api/v1/auth")

	api.Post("/register", authController.Register)
	api.Post("/login", authController.Login)
	api.Post("/logout", authController.Logout)
	api.Post("/forgot-password", authController.ForgotPassword)
	api.Post("/reset-password", authController.ResetPassword)

	// Authenticated routes
	api.Use(middleware.AuthMiddleware(jwtSecret))
	api.Post("/change-password", authController.ChangePassword)
}
