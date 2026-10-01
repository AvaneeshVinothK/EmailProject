package models

import "time"

// User represents a person using the application and owns one or more email accounts.
type User struct {
	ID        int       `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

// EmailAccount stores the synchronization state for a Gmail account connected to a user.
type EmailAccount struct {
	ID                  int        `json:"id"`
	UserID              int        `json:"user_id"`
	Provider            string     `json:"provider"`
	EmailAddress        string     `json:"email_address"`
	RefreshToken        string     `json:"-"`
	LastHistoryID       *string    `json:"last_history_id"`
	BackfillCompletedAt *time.Time `json:"backfill_completed_at"`
	CreatedAt           time.Time  `json:"created_at"`
}

// Email is the normalized content of a message as stored for classification and review.
type Email struct {
	ID             int       `json:"id"`
	EmailAccountID int       `json:"email_account_id"`
	GmailMessageID string    `json:"gmail_message_id"`
	Sender         string    `json:"sender"`
	Subject        string    `json:"subject"`
	Body           string    `json:"body"`
	ReceivedAt     time.Time `json:"received_at"`
}

// Category represents a persisted job-search classification bucket and the description
// used to guide the classifier toward it.
type Category struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// CategoryName identifies the classification bucket assigned to an email.
type CategoryName string

const (
	// CategoryConfirmation covers automated application-received confirmations.
	CategoryConfirmation CategoryName = "confirmation"
	// CategoryNextSteps covers actionable follow-up messages.
	CategoryNextSteps CategoryName = "next_steps"
	// CategoryRecruiterReachOut covers outreach from recruiters or hiring teams.
	CategoryRecruiterReachOut CategoryName = "recruiter_reach_out"
	// CategoryOnlineAssessment covers coding challenge and assessment communications.
	CategoryOnlineAssessment CategoryName = "online_assessment"
	// CategoryInterview covers interview invitations and interview-related updates.
	CategoryInterview CategoryName = "interview"
	// CategoryOther is the fallback for emails that do not clearly fit any other category.
	CategoryOther CategoryName = "other"
)
