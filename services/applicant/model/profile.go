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
