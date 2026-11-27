package model

import (
	"time"

	"github.com/qonics/qonics-appy-api/libs/shared-package/utils"
)

// Validate is the validator used for incoming application payloads.
var Validate = utils.NewValidator()

func init() {
	// No custom validations needed for package-based model
}

type Application struct {
	ID             string                `json:"id"`
	Reference      string                `json:"reference"`
	Applicant      ApplicantSummary      `json:"applicant"`
	PackageID      *int                  `json:"package_id,omitempty"`
	PackageName    string                `json:"package_name,omitempty"`
	Department     string                `json:"department"`     // Kept for historical records
	Specialization string                `json:"specialization"` // Kept for historical records
	Motivation     string                `json:"motivation"`
	Experience     string                `json:"experience"`
	StartDate      *string               `json:"start_date,omitempty"`
	EducationLevel string                `json:"education_level"`
	Session        string                `json:"session"`
	DurationMonths int                   `json:"duration_months"`
	WorkMode       string                `json:"work_mode"`
	Amount         *float64              `json:"amount"`
	Skills         []string              `json:"skills,omitempty"`
	AgreedToTerms  bool                  `json:"agreed_to_terms"`
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

// PackageCount is the number of applications for a single package.
type PackageCount struct {
	PackageID   int    `json:"package_id"`
	PackageName string `json:"package_name"`
	Count       int    `json:"count"`
}

// TrendPoint is the number of applications submitted in a single month.
type TrendPoint struct {
	Month string `json:"month"`
	Count int    `json:"count"`
}

// ListAllResponse is the payload for GET /applications (admin): every
// application plus the dashboard aggregates (status summary, per-package
// counts and the last twelve months trend).
type ListAllResponse struct {
	Applications []Application      `json:"applications"`
	Summary      ApplicationSummary `json:"summary"`
	ByPackage    []PackageCount     `json:"by_package"`
	Trends       []TrendPoint       `json:"trends"`
}

// ApplicationFilter is the query contract for the application list endpoints
// (GET /applications and GET /applications/mine). The fields tagged with
// `field`/`op` are turned into a parameterised WHERE clause by
// utils.BuildWhereClause. Search performs a cross-column match handled in the
// controller (plain columns with ILIKE, encrypted-PII columns via their hash).
type ApplicationFilter struct {
	Page  int `query:"page"`
	Limit int `query:"limit"`

	// Search matches reference / package_name (ILIKE) and the
	// applicant full_name_hash / email_hash columns.
	Search string `query:"search"`

	Reference string `query:"reference" field:"a.reference" op:"ILIKE" validate:"omitempty,max=100"`
	PackageID int    `query:"package_id" field:"a.package_id" op:"=" validate:"omitempty,min=1"`
	Status    string `query:"status" field:"a.status" op:"=" validate:"omitempty,oneof=submitted under_review shortlisted interview accepted rejected"`

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

	// Package selection is required for pricing
	PackageID int `json:"package_id" validate:"required,min=1"`

	// Duration in months is specified by the applicant
	DurationMonths int `json:"duration_months" validate:"required,min=1,max=24"`

	// Department and specialization are optional - kept for historical records
	// Frontend can send these along with package_id
	Department     string `json:"department" validate:"omitempty"`
	Specialization string `json:"specialization" validate:"omitempty"`

	Motivation     string   `json:"motivation" validate:"required,max=2000"`
	Experience     string   `json:"experience" validate:"required,max=2000"`
	StartDate      string   `json:"start_date"`
	EducationLevel string   `json:"education_level" validate:"required,oneof=university high_school other"`
	Session        string   `json:"session" validate:"required,oneof=morning afternoon"`
	WorkMode       string   `json:"work_mode" validate:"required,oneof=onsite hybrid"`
	AgreedToTerms  bool     `json:"agreed_to_terms" validate:"required"`
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

type Package struct {
	ID                    int      `json:"id"`
	Name                  string   `json:"name"`
	Description           string   `json:"description"`
	Price                 float64  `json:"price"`
	Skills                []string `json:"skills"`
	DurationMonths        int      `json:"duration_months"`
	Currency              string   `json:"currency"`
	IsActive              bool     `json:"is_active"`
	MinimumDurationPeriod int      `json:"minimum_duration_period"`
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
func UsesSeat(workMode string) bool {
	return workMode == "onsite" || workMode == "hybrid"
}
