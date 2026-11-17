package config

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/qonics/qonics-appy-api/libs/shared-package/utils"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
)

type Config struct {
	ServiceName  string
	Port         string
	Database     DatabaseConfig
	DatabaseTest DatabaseConfig
	Redis        RedisConfig
	JWT          JWTConfig
	Email        EmailConfig
}

type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

type RedisConfig struct {
	Host     string
	Port     string
	Password string
	DB       int
}

type JWTConfig struct {
	Secret string
}

type EmailConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	From     string
}

var AppConfig *Config

// LoadConfig loads configuration from environment variables and config file
func LoadConfig(serviceName string) (*Config, error) {
	// Load .env file if exists
	godotenv.Load()

	viper.SetConfigName("config")
	viper.SetConfigType("yml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./config")

	// Set defaults
	viper.SetDefault("port", "9000")
	viper.SetDefault("database.host", "localhost")
	viper.SetDefault("database.port", "5432")
	viper.SetDefault("database.sslmode", "disable")
	viper.SetDefault("database_test.host", "127.0.0.1")
	viper.SetDefault("database_test.port", "5434")
	viper.SetDefault("database_test.sslmode", "disable")
	viper.SetDefault("redis.host", "localhost")
	viper.SetDefault("redis.port", "6379")
	viper.SetDefault("redis.db", 0)
	viper.SetDefault("email.host", "smtp.gmail.com")
	viper.SetDefault("email.port", "587")
	viper.SetDefault("email.from", "noreply@qonics.com")

	// Read from config file if exists
	viper.ReadInConfig()

	// Override with environment variables
	viper.AutomaticEnv()
	viper.SetEnvPrefix("") // No prefix for env vars

	// Explicitly bind env vars for nested configs
	viper.BindEnv("database.host", "DATABASE_HOST")
	viper.BindEnv("database.port", "DATABASE_PORT")
	viper.BindEnv("database.user", "DATABASE_USER")
	viper.BindEnv("database.password", "DATABASE_PASSWORD")
	viper.BindEnv("database.dbname", "DATABASE_DBNAME")
	viper.BindEnv("database.sslmode", "DATABASE_SSLMODE")
	viper.BindEnv("database_test.dbname", "DATABASE_TEST_DBNAME")
	viper.BindEnv("database_test.host", "DATABASE_TEST_HOST")
	viper.BindEnv("database_test.port", "DATABASE_TEST_PORT")
	viper.BindEnv("database_test.user", "DATABASE_TEST_USER")
	viper.BindEnv("database_test.password", "DATABASE_TEST_PASSWORD")
	viper.BindEnv("database_test.sslmode", "DATABASE_TEST_SSLMODE")
	viper.BindEnv("redis.host", "REDIS_HOST")
	viper.BindEnv("redis.port", "REDIS_PORT")
	viper.BindEnv("redis.password", "REDIS_PASSWORD")
	viper.BindEnv("redis.db", "REDIS_DB")
	viper.BindEnv("jwt.secret", "JWT_SECRET")
	viper.BindEnv("encryption.key", "ENCRYPTION_KEY")

	config := &Config{
		ServiceName: serviceName,
		Port:        viper.GetString("port"),
		Database: DatabaseConfig{
			Host:     viper.GetString("database.host"),
			Port:     viper.GetString("database.port"),
			User:     viper.GetString("database.user"),
			Password: viper.GetString("database.password"),
			DBName:   viper.GetString("database.dbname"),
			SSLMode:  viper.GetString("database.sslmode"),
		},
		DatabaseTest: DatabaseConfig{
			Host:     viper.GetString("database_test.host"),
			Port:     viper.GetString("database_test.port"),
			User:     viper.GetString("database_test.user"),
			Password: viper.GetString("database_test.password"),
			DBName:   viper.GetString("database_test.dbname"),
			SSLMode:  viper.GetString("database_test.sslmode"),
		},
		Redis: RedisConfig{
			Host:     viper.GetString("redis.host"),
			Port:     viper.GetString("redis.port"),
			Password: viper.GetString("redis.password"),
			DB:       viper.GetInt("redis.db"),
		},
		JWT: JWTConfig{
			Secret: viper.GetString("jwt.secret"),
		},
		Email: EmailConfig{
			Host:     viper.GetString("email.host"),
			Port:     viper.GetString("email.port"),
			User:     viper.GetString("email.user"),
			Password: viper.GetString("email.password"),
			From:     viper.GetString("email.from"),
		},
	}

	AppConfig = config
	return config, nil
}

// GetDBConnectionString returns the PostgreSQL connection string
func (c *Config) GetDBConnectionString() string {
	return "postgres://" + c.Database.User + ":" + c.Database.Password +
		"@" + c.Database.Host + ":" + c.Database.Port + "/" + c.Database.DBName +
		"?sslmode=" + c.Database.SSLMode
}

// GetTestDBConnectionString returns the PostgreSQL test database connection string
func (c *Config) GetTestDBConnectionString() string {
	return "postgres://" + c.DatabaseTest.User + ":" + c.DatabaseTest.Password +
		"@" + c.DatabaseTest.Host + ":" + c.DatabaseTest.Port + "/" + c.DatabaseTest.DBName +
		"?sslmode=" + c.DatabaseTest.SSLMode
}

// InitDB initializes the database connection pool
func (c *Config) InitDB() (*pgxpool.Pool, error) {
	ctx := context.Background()

	var connStr string
	// Use test database if in test mode
	if utils.IsTestMode {
		connStr = c.GetTestDBConnectionString()
	} else {
		connStr = c.GetDBConnectionString()
	}

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		return nil, err
	}

	// Set encryption key for all connections
	if utils.IsTestMode {
		_, err = pool.Exec(ctx, "SET app.encryption_key = 'secret'")
		if err != nil {
			pool.Close()
			return nil, err
		}
	}

	return pool, nil
}

// InitRedis initializes the Redis client
func (c *Config) InitRedis() *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     c.Redis.Host + ":" + c.Redis.Port,
		Password: c.Redis.Password,
		DB:       c.Redis.DB,
	})
}
