-- Create applications table
CREATE TABLE IF NOT EXISTS applications (
    id VARCHAR(50) PRIMARY KEY,
    reference VARCHAR(50) UNIQUE NOT NULL,
    applicant_id VARCHAR(50) NOT NULL REFERENCES applicant_profiles(id) ON DELETE CASCADE,
    department VARCHAR(100) NOT NULL,
    specialization VARCHAR(255) NOT NULL,
    motivation TEXT NOT NULL,
    experience TEXT NOT NULL,
    start_date DATE,
    education_level VARCHAR(50) NOT NULL CHECK (education_level IN ('university', 'high_school', 'bachelor', 'master', 'phd', 'diploma', 'certificate')),
    session VARCHAR(20) NOT NULL CHECK (session IN ('morning', 'afternoon', 'january-june', 'july-december')),
    duration_months INTEGER NOT NULL,
    work_mode VARCHAR(20) NOT NULL CHECK (work_mode IN ('onsite', 'remote', 'hybrid')),
    status VARCHAR(30) NOT NULL DEFAULT 'submitted' CHECK (status IN ('submitted', 'under_review', 'shortlisted', 'interview', 'accepted', 'rejected')),
    submitted_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    interview_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes
CREATE INDEX idx_applications_applicant_id ON applications(applicant_id);
CREATE INDEX idx_applications_status ON applications(status);
CREATE INDEX idx_applications_department ON applications(department);
CREATE INDEX idx_applications_session ON applications(session);

-- Trigger to automatically update updated_at
CREATE TRIGGER update_applications_updated_at BEFORE UPDATE ON applications
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
