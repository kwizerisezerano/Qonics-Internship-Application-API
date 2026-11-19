package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/qonics/qonics-appy-api/libs/shared-package/middleware"
	"github.com/qonics/qonics-appy-api/services/applicant/controller"
)

func SetupRoutes(app *fiber.App, applicantController *controller.ApplicantController, jwtSecret string) {
	api := app.Group("/api/v1")

	// Applicant routes (authenticated)
	applicant := api.Group("/applicant")
	applicant.Use(middleware.AuthMiddleware(jwtSecret))

	applicant.Get("/profile", applicantController.GetProfile)
	applicant.Put("/profile", applicantController.SaveProfile)
	applicant.Get("/notifications", applicantController.ListNotifications)
	applicant.Post("/notifications/read", applicantController.MarkNotificationsRead)

	// Admin routes
	admin := api.Group("/applicants")
	admin.Use(middleware.AuthMiddleware(jwtSecret))
	admin.Use(middleware.AdminMiddleware())

	admin.Get("/", applicantController.ListApplicants)

	// Activity logs (admin)
	activityLogs := api.Group("/activity-logs")
	activityLogs.Use(middleware.AuthMiddleware(jwtSecret))
	activityLogs.Use(middleware.AdminMiddleware())

	activityLogs.Get("/", applicantController.ListActivityLogs)

	// Settings routes (admin)
	settings := api.Group("/settings")
	settings.Use(middleware.AuthMiddleware(jwtSecret))
	settings.Use(middleware.AdminMiddleware())

	settings.Get("/", applicantController.GetSettings)
	settings.Put("/", applicantController.SaveSettings)
}
