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

type ApplicantProfile struct {
	ID             string   `json:"id"`
	FullName       string   `json:"full_name"`
	Email          string   `json:"email"`
	Phone          string   `json:"phone"`
	Country        string   `json:"country"`
	City           string   `json:"city"`
	Institution    string   `json:"institution"`
	FieldOfStudy   string   `json:"field_of_study"`
	Qualification  string   `json:"qualification"`
	AcademicStatus string   `json:"academic_status"`
	GraduationYear *int     `json:"graduation_year"`
	Skills         []string `json:"skills"`
	Bio            string   `json:"bio"`
	GithubUrl      string   `json:"github_url"`
	LinkedinUrl    string   `json:"linkedin_url"`
	PortfolioUrl   string   `json:"portfolio_url"`
}

type ApiResponse struct {
	Status  int         `json:"status"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

func TestApplicantEndpoints(t *testing.T) {
	token := getAuthToken(t)

	// Test Get Profile
	t.Run("GetProfile", func(t *testing.T) {
		req, _ := http.NewRequest("GET", baseURL+"/api/v1/applicant/profile", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GetProfile request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			t.Fatalf("GetProfile failed with status %d: %s", resp.StatusCode, string(respBody))
		}

		var apiResp ApiResponse
		json.NewDecoder(resp.Body).Decode(&apiResp)
		fmt.Printf("GetProfile response: %+v\n", apiResp)
	})

	// Test Save Profile
	t.Run("SaveProfile", func(t *testing.T) {
		profile := ApplicantProfile{
			FullName:       "Updated Name",
			Email:          "test@example.com",
			Phone:          "+1234567890",
			Country:        "Rwanda",
			City:           "Kigali",
			Institution:    "University of Rwanda",
			FieldOfStudy:   "Computer Science",
			Qualification:  "Bachelor's Degree",
			AcademicStatus: "Graduated",
			Skills:         []string{"Go", "JavaScript", "PostgreSQL"},
			Bio:            "Passionate developer",
		}
		body, _ := json.Marshal(profile)
		req, _ := http.NewRequest("PUT", baseURL+"/api/v1/applicant/profile", bytes.NewBuffer(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("SaveProfile request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			t.Fatalf("SaveProfile failed with status %d: %s", resp.StatusCode, string(respBody))
		}

		var apiResp ApiResponse
		json.NewDecoder(resp.Body).Decode(&apiResp)
		fmt.Printf("SaveProfile response: %+v\n", apiResp)
	})

	// Test List Notifications
	t.Run("ListNotifications", func(t *testing.T) {
		req, _ := http.NewRequest("GET", baseURL+"/api/v1/notifications", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("ListNotifications request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			t.Fatalf("ListNotifications failed with status %d: %s", resp.StatusCode, string(respBody))
		}

		var apiResp ApiResponse
		json.NewDecoder(resp.Body).Decode(&apiResp)
		fmt.Printf("ListNotifications response: %+v\n", apiResp)
	})

	// Test Get Settings
	t.Run("GetSettings", func(t *testing.T) {
		req, _ := http.NewRequest("GET", baseURL+"/api/v1/settings", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GetSettings request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			t.Fatalf("GetSettings failed with status %d: %s", resp.StatusCode, string(respBody))
		}

		var apiResp ApiResponse
		json.NewDecoder(resp.Body).Decode(&apiResp)
		fmt.Printf("GetSettings response: %+v\n", apiResp)
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
