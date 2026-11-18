package model

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

type ApplicantSummary struct {
	ID       string `json:"id"`
	FullName string `json:"full_name"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
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

// TrendPoint is the number of applications submitted in a single month.
type TrendPoint struct {
	Month string `json:"month"`
	Count int    `json:"count"`
}

// ListAllResponse is the payload for GET /applications (admin): every
// application plus the dashboard aggregates (status summary, per-department
// counts and the last six months trend).
type ListAllResponse struct {
	Applications []Application      `json:"applications"`
	Summary      ApplicationSummary `json:"summary"`
	ByDepartment []DepartmentCount  `json:"by_department"`
	Trends       []TrendPoint       `json:"trends"`
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
	Department     string   `json:"department" validate:"required"`
	Specialization string   `json:"specialization" validate:"required"`
	Motivation     string   `json:"motivation" validate:"required"`
	Experience     string   `json:"experience" validate:"required"`
	StartDate      string   `json:"start_date"`
	EducationLevel string   `json:"education_level" validate:"required,oneof=university high_school"`
	Session        string   `json:"session" validate:"required,oneof=morning afternoon"`
	DurationMonths int      `json:"duration_months" validate:"required,min=1"`
	WorkMode       string   `json:"work_mode" validate:"required,oneof=onsite remote hybrid"`
	DocumentIDs    []string `json:"document_ids"`
}

type StatusUpdatePayload struct {
	Status      string `json:"status" validate:"required,oneof=submitted under_review shortlisted interview accepted rejected"`
	Note        string `json:"note"`
	InterviewAt string `json:"interview_at"`
}

type AddNoteRequest struct {
	Text string `json:"text" validate:"required"`
}

type Booking struct {
	StartDate      string `json:"start_date"`
	EndDate        string `json:"end_date"`
	Session        string `json:"session"`
	AvailableSeats int    `json:"available_seats"`
}
