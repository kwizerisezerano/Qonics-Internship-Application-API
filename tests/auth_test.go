package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
)

const baseURL = "http://localhost:8080"

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type RegisterRequest struct {
	FullName string `json:"full_name"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Password string `json:"password"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type Session struct {
	Token string `json:"token"`
	User  struct {
		ID       int    `json:"id"`
		FullName string `json:"full_name"`
		Email    string `json:"email"`
		Phone    string `json:"phone"`
		Role     string `json:"role"`
	} `json:"user"`
}

type ApiResponse struct {
	Status  int         `json:"status"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

func TestAuthEndpoints(t *testing.T) {
	// Test Register
	t.Run("Register", func(t *testing.T) {
		reqBody := RegisterRequest{
			FullName: "Test User",
			Email:    "test@example.com",
			Phone:    "+1234567890",
			Password: "Str0ng@Pass",
		}
		body, _ := json.Marshal(reqBody)
		resp, err := http.Post(baseURL+"/api/v1/auth/register", "application/json", bytes.NewBuffer(body))
		if err != nil {
			t.Fatalf("Register request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			t.Fatalf("Register failed with status %d: %s", resp.StatusCode, string(respBody))
		}

		var apiResp ApiResponse
		json.NewDecoder(resp.Body).Decode(&apiResp)
		fmt.Printf("Register response: %+v\n", apiResp)
	})

	// Test Login
	t.Run("Login", func(t *testing.T) {
		reqBody := LoginRequest{
			Email:    "test@example.com",
			Password: "Str0ng@Pass",
		}
		body, _ := json.Marshal(reqBody)
		resp, err := http.Post(baseURL+"/api/v1/auth/login", "application/json", bytes.NewBuffer(body))
		if err != nil {
			t.Fatalf("Login request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			t.Fatalf("Login failed with status %d: %s", resp.StatusCode, string(respBody))
		}

		var apiResp ApiResponse
		json.NewDecoder(resp.Body).Decode(&apiResp)
		fmt.Printf("Login response: %+v\n", apiResp)
	})

	// Test Forgot Password
	t.Run("ForgotPassword", func(t *testing.T) {
		reqBody := map[string]string{"email": "test@example.com"}
		body, _ := json.Marshal(reqBody)
		resp, err := http.Post(baseURL+"/api/v1/auth/forgot-password", "application/json", bytes.NewBuffer(body))
		if err != nil {
			t.Fatalf("ForgotPassword request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			t.Fatalf("ForgotPassword failed with status %d: %s", resp.StatusCode, string(respBody))
		}

		fmt.Println("ForgotPassword successful")
	})

	// Test Logout (requires auth token)
	t.Run("Logout", func(t *testing.T) {
		// First login to get token
		reqBody := LoginRequest{
			Email:    "test@example.com",
			Password: "Str0ng@Pass",
		}
		body, _ := json.Marshal(reqBody)
		resp, err := http.Post(baseURL+"/api/v1/auth/login", "application/json", bytes.NewBuffer(body))
		if err != nil {
			t.Fatalf("Login request failed: %v", err)
		}
		defer resp.Body.Close()

		var apiResp ApiResponse
		json.NewDecoder(resp.Body).Decode(&apiResp)

		// Extract token from response
		sessionData, _ := json.Marshal(apiResp.Data)
		var session Session
		json.Unmarshal(sessionData, &session)

		// Logout with token
		req, _ := http.NewRequest("POST", baseURL+"/api/v1/auth/logout", nil)
		req.Header.Set("Authorization", "Bearer "+session.Token)
		resp, err = http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Logout request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			t.Fatalf("Logout failed with status %d: %s", resp.StatusCode, string(respBody))
		}

		fmt.Println("Logout successful")
	})
}
