package main

import (
	"context"
	"os"
	"testing"

	"github.com/qonics/qonics-appy-api/libs/shared-package/config"
	"github.com/qonics/qonics-appy-api/libs/shared-package/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotificationWorkerInit(t *testing.T) {
	utils.IsTestMode = true

	// Load .env from project root
	err := os.Chdir("../..")
	if err != nil {
		panic("Failed to change directory: " + err.Error())
	}

	cfg, err := config.LoadConfig("notification-worker")
	require.NoError(t, err)

	db, err := cfg.InitDB()
	require.NoError(t, err)
	defer db.Close()

	// Test database connection
	ctx := context.Background()
	var result int
	err = db.QueryRow(ctx, "SELECT 1").Scan(&result)
	require.NoError(t, err)
	assert.Equal(t, 1, result)
}
