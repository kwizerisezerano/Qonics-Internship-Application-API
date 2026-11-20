package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/qonics/qonics-appy-api/libs/shared-package/middleware"
	"github.com/qonics/qonics-appy-api/services/file/controller"
)

func SetupRoutes(app *fiber.App, fileController *controller.FileController, jwtSecret string) {
	api := app.Group("/api/v1")

	files := api.Group("/files")

	// Guest uploads (public): for people applying without an account. Registered
	// before the auth middleware below so they are not subject to it.
	files.Post("/guest/presigned-url", fileController.GuestPresignedURL)
	files.Post("/guest/confirm-upload", fileController.GuestConfirmUpload)

	// File routes (authenticated)
	files.Use(middleware.AuthMiddleware(jwtSecret))

	files.Post("/presigned-url", fileController.GetPresignedURL)
	files.Post("/confirm-upload", fileController.ConfirmFileUpload)
	files.Get("/:file_id/url", fileController.GetFileURL)
}
