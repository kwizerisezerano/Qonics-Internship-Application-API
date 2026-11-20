package model

import (
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/qonics/qonics-appy-api/libs/shared-package/utils"
)

// Specializations is the canonical taxonomy: each department mapped to the
// specialization values it accepts. Kept in sync with the frontend DEPARTMENTS
// config and used to validate incoming payloads.
var Specializations = map[string][]string{
	"software": {"frontend", "backend", "fullstack", "mobile", "database"},
	"iot":      {"embedded", "arduino-esp32", "sensors", "iot-apps", "smart-systems"},
}

// Validate is the validator used for incoming application payloads. In addition
// to the struct tags it carries the custom `specialization` rule below.
var Validate = utils.NewValidator()

func init() {
	// specialization passes only when the value belongs to the payload's
	// department (e.g. department "iot" rejects "backend").
	Validate.RegisterValidation("specialization", func(fl validator.FieldLevel) bool {
		department := fl.Parent().FieldByName("Department").String()
		for _, s := range Specializations[department] {
			if s == fl.Field().String() {
				return true
			}
		}
		return false
	})
}

type Application struct {
	ID             string                `json:"id"`
	Reference      string                `json:"reference"`
	Applicant      ApplicantSummary      `json:"applicant"`
	Department     string                `json:"department"`
	Specialization string                `json:"specialization"`
	Motivation     string                `json:"motivation"`
	Experience     string                `json:"experience"`
	StartDate      *string               `json:"start_date,omitempty"`
	EducationLevel string                `json:"education_level"`
	Session        string                `json:"session"`
	DurationMonths int                   `json:"duration_months"`
	WorkMode       string                `json:"work_mode"`
	Status         string                `json:"status"`
	SubmittedAt    string                `json:"submitted_at"`
	InterviewAt    *string               `json:"interview_at,omitempty"`
	Documents      []ApplicationDocument `json:"documents"`
	History        []StatusEvent         `json:"history"`
	Notes          []ReviewNote          `json:"notes"`
}

// ApplicantSummary is the applicant's profile as embedded in an application,
// so reviewers see academic background and links alongside the application.
type ApplicantSummary struct {
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
	GithubURL      string   `json:"github_url"`
	LinkedinURL    string   `json:"linkedin_url"`
	PortfolioURL   string   `json:"portfolio_url"`
}

// ApplicationSummary is the count-by-status breakdown of a set of applications.
type ApplicationSummary struct {
	Total       int `json:"total"`
	Submitted   int `json:"submitted"`
	UnderReview int `json:"under_review"`
	Shortlisted int `json:"shortlisted"`
	Interview   int `json:"interview"`
	Accepted    int `json:"accepted"`
	Rejected    int `json:"rejected"`
}

// MineResponse is the payload for GET /applications/mine: the caller's
// applications plus their count-by-status summary.
type MineResponse struct {
	Applications []Application      `json:"applications"`
	Summary      ApplicationSummary `json:"summary"`
}

// DepartmentCount is the number of applications for a single department.
type DepartmentCount struct {
	Department string `json:"department"`
	Count      int    `json:"count"`
}

// SpecializationCount is the number of applications for a single
// department + specialization pair.
type SpecializationCount struct {
	Department     string `json:"department"`
	Specialization string `json:"specialization"`
	Count          int    `json:"count"`
}

// TrendPoint is the number of applications submitted in a single month.
type TrendPoint struct {
	Month string `json:"month"`
	Count int    `json:"count"`
}

// ListAllResponse is the payload for GET /applications (admin): every
// application plus the dashboard aggregates (status summary, per-department
// counts, per-specialization counts and the last twelve months trend).
type ListAllResponse struct {
	Applications     []Application         `json:"applications"`
	Summary          ApplicationSummary    `json:"summary"`
	ByDepartment     []DepartmentCount     `json:"by_department"`
	BySpecialization []SpecializationCount `json:"by_specialization"`
	Trends           []TrendPoint          `json:"trends"`
}

// ApplicationFilter is the query contract for the application list endpoints
// (GET /applications and GET /applications/mine). The fields tagged with
// `field`/`op` are turned into a parameterised WHERE clause by
// utils.BuildWhereClause. Search performs a cross-column match handled in the
// controller (plain columns with ILIKE, encrypted-PII columns via their hash).
type ApplicationFilter struct {
	Page  int `query:"page"`
	Limit int `query:"limit"`

	// Search matches reference / department / specialization (ILIKE) and the
	// applicant full_name_hash / email_hash columns.
	Search string `query:"search"`

	Reference  string `query:"reference" field:"a.reference" op:"ILIKE" validate:"omitempty,max=100"`
	Department string `query:"department" field:"a.department" op:"=" validate:"omitempty"`
	Status     string `query:"status" field:"a.status" op:"=" validate:"omitempty,oneof=submitted under_review shortlisted interview accepted rejected"`

	// Submission date range. Parsed into time.Time in the controller so pgx
	// binds them as timestamptz; query:"-" keeps Fiber's decoder away from them.
	SubmittedFrom *time.Time `query:"-" field:"a.submitted_at" op:">="`
	SubmittedTo   *time.Time `query:"-" field:"a.submitted_at" op:"<="`
}

type ApplicationDocument struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	UploadedAt string `json:"uploaded_at"`
	URL        string `json:"url,omitempty"`
}

type StatusEvent struct {
	Status string `json:"status"`
	At     string `json:"at"`
	Note   string `json:"note,omitempty"`
}

type ReviewNote struct {
	ID     string `json:"id"`
	Author string `json:"author"`
	Text   string `json:"text"`
	At     string `json:"at"`
}

type NewApplicationPayload struct {
	// Contact details, required only when applying without being logged in
	// (the controller enforces that); ignored for logged-in applicants.
	FullName string `json:"full_name" validate:"omitempty,max=150,name"`
	Email    string `json:"email" validate:"omitempty,email"`
	Phone    string `json:"phone" validate:"omitempty,max=30,phone"`

	Department     string   `json:"department" validate:"required,oneof=software iot"`
	Specialization string   `json:"specialization" validate:"required,specialization"`
	Motivation     string   `json:"motivation" validate:"required,max=2000"`
	Experience     string   `json:"experience" validate:"required,max=2000"`
	StartDate      string   `json:"start_date"`
	EducationLevel string   `json:"education_level" validate:"required,oneof=university high_school"`
	Session        string   `json:"session" validate:"required,oneof=morning afternoon"`
	DurationMonths int      `json:"duration_months" validate:"required,min=1"`
	WorkMode       string   `json:"work_mode" validate:"required,oneof=onsite remote hybrid"`
	DocumentIDs    []string `json:"document_ids" validate:"omitempty,dive,uuid"`
	// GuestToken proves ownership of documents a guest uploaded before applying
	// (returned by the file service's guest upload); only used without a login.
	GuestToken string `json:"guest_token" validate:"omitempty,len=32,hexadecimal"`
}

type StatusUpdatePayload struct {
	Status      string `json:"status" validate:"required,oneof=submitted under_review shortlisted interview accepted rejected"`
	Note        string `json:"note" validate:"omitempty,max=2000"`
	InterviewAt string `json:"interview_at"`
}

type AddNoteRequest struct {
	Text string `json:"text" validate:"required,max=2000"`
}

type Booking struct {
	StartDate      string `json:"start_date"`
	EndDate        string `json:"end_date"`
	Session        string `json:"session"`
	AvailableSeats int    `json:"available_seats"`
}

// SessionCapacity is how many on-site/hybrid interns one daily session holds at
// the same time. It must match SESSION_CAPACITY in the web app
// (app/utils/sessions.ts), which promises applicants "6 seats per session".
const SessionCapacity = 6

// UsesSeat reports whether a work mode occupies a seat in a daily session
// (remote interns don't).
func UsesSeat(workMode string) bool {
	return workMode == "onsite" || workMode == "hybrid"
}

// displayNames maps department and specialization ids to the names people see
// (kept in sync with the frontend DEPARTMENTS config).
var displayNames = map[string]string{
	"software":      "Software Development",
	"iot":           "Internet of Things",
	"frontend":      "Frontend Development",
	"backend":       "Backend Development",
	"fullstack":     "Full-Stack Development",
	"mobile":        "Mobile Development",
	"database":      "Database Development",
	"embedded":      "Embedded Systems",
	"arduino-esp32": "Arduino and ESP32",
	"sensors":       "Sensors and Electronics",
	"iot-apps":      "IoT Application Development",
	"smart-systems": "Smart Systems",
}

// DisplayName returns the human-readable name for a department or
// specialization id, falling back to the id itself when it is unknown.
func DisplayName(id string) string {
	if name, ok := displayNames[id]; ok {
		return name
	}
	return id
}
