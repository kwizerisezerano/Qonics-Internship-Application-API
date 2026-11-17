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

type NewApplicationPayload struct {
	Department      string `json:"department"`
	Specialization string `json:"specialization"`
	Motivation      string `json:"motivation"`
	Experience      string `json:"experience"`
	StartDate       string `json:"start_date"`
	EducationLevel  string `json:"education_level"`
	Session         string `json:"session"`
	DurationMonths  int    `json:"duration_months"`
	WorkMode        string `json:"work_mode"`
}

type StatusUpdatePayload struct {
	Status      string `json:"status"`
	Note        string `json:"note"`
	InterviewAt string `json:"interview_at"`
}

type ApiResponse struct {
	Status  int         `json:"status"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

func TestApplicationEndpoints(t *testing.T) {
	token := getAuthToken(t)

	// Test List My Applications
	t.Run("ListMine", func(t *testing.T) {
		req, _ := http.NewRequest("GET", baseURL+"/api/v1/applications/mine", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("ListMine request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			t.Fatalf("ListMine failed with status %d: %s", resp.StatusCode, string(respBody))
		}

		var apiResp ApiResponse
		json.NewDecoder(resp.Body).Decode(&apiResp)
		fmt.Printf("ListMine response: %+v\n", apiResp)
	})

	// Test List All Applications (admin only - may fail with normal user)
	t.Run("ListAll", func(t *testing.T) {
		req, _ := http.NewRequest("GET", baseURL+"/api/v1/applications", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("ListAll request failed: %v", err)
		}
		defer resp.Body.Close()

		// May return 403 Forbidden for non-admin users
		if resp.StatusCode == http.StatusForbidden {
			fmt.Println("ListAll: Forbidden (expected for non-admin)")
			return
		}

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			t.Fatalf("ListAll failed with status %d: %s", resp.StatusCode, string(respBody))
		}

		var apiResp ApiResponse
		json.NewDecoder(resp.Body).Decode(&apiResp)
		fmt.Printf("ListAll response: %+v\n", apiResp)
	})

	// Test Create Application
	t.Run("CreateApplication", func(t *testing.T) {
		payload := NewApplicationPayload{
			Department:      "Engineering",
			Specialization:  "Software Development",
			Motivation:      "I want to learn and contribute",
			Experience:      "2 years of web development",
			StartDate:       "2025-01-15T00:00:00Z",
			EducationLevel:  "Bachelor",
			Session:         "january-june",
			DurationMonths:  6,
			WorkMode:        "remote",
		}
		body, _ := json.Marshal(payload)
		req, _ := http.NewRequest("POST", baseURL+"/api/v1/applications", bytes.NewBuffer(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("CreateApplication request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			t.Fatalf("CreateApplication failed with status %d: %s", resp.StatusCode, string(respBody))
		}

		var apiResp ApiResponse
		json.NewDecoder(resp.Body).Decode(&apiResp)
		fmt.Printf("CreateApplication response: %+v\n", apiResp)
	})

	// Test Availability
	t.Run("Availability", func(t *testing.T) {
		req, _ := http.NewRequest("GET", baseURL+"/api/v1/sessions/bookings", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Availability request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			t.Fatalf("Availability failed with status %d: %s", resp.StatusCode, string(respBody))
		}

		var apiResp ApiResponse
		json.NewDecoder(resp.Body).Decode(&apiResp)
		fmt.Printf("Availability response: %+v\n", apiResp)
	})
}

func getAuthToken(t *testing.T) string {
	loginReq := map[string]string{
		"email":    "test@example.com",
		"password": "password123",
	}
	body, _ := json.Marshal(loginReq)
	resp, err := http.Post(baseURL+"/api/v1/auth/login", "application/json", bytes.NewBuffer(body))
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	defer resp.Body.Close()

	var apiResp ApiResponse
	json.NewDecoder(resp.Body).Decode(&apiResp)

	sessionData, _ := json.Marshal(apiResp.Data)
	var session struct {
		Token string `json:"token"`
	}
	json.Unmarshal(sessionData, &session)

	return session.Token
}
