package main

import (
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/qonics/qonics-appy-api/libs/shared-package/config"
	"github.com/qonics/qonics-appy-api/libs/shared-package/logger"
	"github.com/qonics/qonics-appy-api/services/applicant/controller"
	"github.com/qonics/qonics-appy-api/services/applicant/routes"
)

func main() {
	// Load configuration
	cfg, err := config.LoadConfig("applicant-service")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize logger
	if err := logger.InitLogger(cfg.ServiceName, "info"); err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}

	// Initialize database
	db, err := cfg.InitDB()
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Initialize Redis
	rdb := cfg.InitRedis()
	defer rdb.Close()

	// Initialize Fiber app
	app := fiber.New(fiber.Config{
		AppName:      cfg.ServiceName,
		ErrorHandler: customErrorHandler,
	})

	// Initialize controllers
	applicantController := controller.NewApplicantController(db, cfg, rdb)

	// Setup routes
	routes.SetupRoutes(app, applicantController, cfg.JWT.Secret)

	// Health check
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "service": cfg.ServiceName})
	})

	// Start server
	log.Printf("Starting %s on port %s", cfg.ServiceName, cfg.Port)
	if err := app.Listen(":" + cfg.Port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func customErrorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
	}

	return c.Status(code).JSON(fiber.Map{
		"status":  code,
		"message": err.Error(),
	})
}
