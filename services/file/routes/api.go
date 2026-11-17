package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/qonics/qonics-appy-api/libs/shared-package/middleware"
	"github.com/qonics/qonics-appy-api/services/file/controller"
)

func SetupRoutes(app *fiber.App, fileController *controller.FileController, jwtSecret string) {
	api := app.Group("/api/v1")

	// File routes (authenticated)
	files := api.Group("/files")
	files.Use(middleware.AuthMiddleware(jwtSecret))

	files.Post("/presigned-url", fileController.GetPresignedURL)
	files.Post("/confirm-upload", fileController.ConfirmFileUpload)
}
