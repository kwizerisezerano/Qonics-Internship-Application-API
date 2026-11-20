package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/qonics/qonics-appy-api/libs/shared-package/middleware"
	"github.com/qonics/qonics-appy-api/services/application/controller"
)

func SetupRoutes(app *fiber.App, applicationController *controller.ApplicationController, jwtSecret string) {
	api := app.Group("/api/v1")

	applications := api.Group("/applications")

	// Submitting is open to guests: without a token the applicant's name, email
	// and phone in the body create their account. Registered before the auth
	// middleware below so it is not subject to it.
	applications.Post("/", middleware.OptionalAuthMiddleware(jwtSecret), applicationController.Create)

	// Everything else (authenticated)
	applications.Use(middleware.AuthMiddleware(jwtSecret))

	applications.Get("/mine", applicationController.ListMine)
	applications.Get("/:id", applicationController.Get)
	applications.Patch("/:id/status", applicationController.UpdateStatus, middleware.AdminMiddleware())
	applications.Post("/:id/notes", applicationController.AddNote, middleware.AdminMiddleware())

	// Admin routes
	applications.Get("/", applicationController.ListAll)

	// Public routes
	public := api.Group("/sessions")
	public.Get("/bookings", applicationController.Availability)
}
