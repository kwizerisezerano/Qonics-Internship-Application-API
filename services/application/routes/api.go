package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/qonics/qonics-appy-api/libs/shared-package/middleware"
	"github.com/qonics/qonics-appy-api/services/application/controller"
)

func SetupRoutes(app *fiber.App, applicationController *controller.ApplicationController, jwtSecret string) {
	api := app.Group("/api/v1")

	// Application routes (authenticated)
	applications := api.Group("/applications")
	applications.Use(middleware.AuthMiddleware(jwtSecret))

	applications.Get("/mine", applicationController.ListMine)
	applications.Post("/", applicationController.Create)
	applications.Get("/:id", applicationController.Get)
	applications.Patch("/:id/status", applicationController.UpdateStatus)
	applications.Post("/:id/notes", applicationController.AddNote)

	// Admin routes - check admin role in controller
	applications.Get("/", applicationController.ListAll)

	// Public routes
	public := api.Group("/sessions")
	public.Get("/bookings", applicationController.Availability)
}
