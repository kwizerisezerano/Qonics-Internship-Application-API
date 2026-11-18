package model

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

type ApplicantSummary struct {
	ApplicantProfile
	RegisteredAt      string `json:"registered_at"`
	TotalApplications int    `json:"total_applications"`
}

type AppNotification struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Message string `json:"message"`
	At      string `json:"at"`
	Read    bool   `json:"read"`
}

type AppSettings struct {
	ApplicationsOpen bool   `json:"applications_open"`
	CloseOn          string `json:"close_on"`
	ClosureMessage   string `json:"closure_message"`
}

type MarkNotificationsReadRequest struct {
	IDs []string `json:"ids" validate:"required"`
}

// ApplicantFilter is the query contract for GET /applicants (admin). The tagged
// fields are turned into a parameterised WHERE clause by utils.BuildWhereClause;
// Search is a cross-column match handled in the controller (plain columns with
// ILIKE, encrypted-PII columns via their hash).
type ApplicantFilter struct {
	Page  int `query:"page"`
	Limit int `query:"limit"`

	// Search matches institution / field_of_study (ILIKE) and the applicant
	// full_name_hash / email_hash columns.
	Search string `query:"search"`

	Institution  string `query:"institution" field:"ap.institution" op:"ILIKE" validate:"omitempty,max=100"`
	FieldOfStudy string `query:"field_of_study" field:"ap.field_of_study" op:"ILIKE" validate:"omitempty,max=100"`
}

// NotificationFilter is the query contract for GET /applicant/notifications.
type NotificationFilter struct {
	Page   int   `query:"page"`
	Limit  int   `query:"limit"`
	Read   *bool `query:"read" field:"n.read" op:"="`
	Search string `query:"search"`

	// UserID scopes the list to the caller; set in the controller.
	UserID int `query:"-" field:"n.user_id" op:"="`
}
