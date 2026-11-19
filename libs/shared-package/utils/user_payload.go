package utils

import "github.com/gofiber/fiber/v2"

// UserPayloadKey is the fiber Locals key holding the authenticated user's
// UserPayload (set by middleware.AuthMiddleware).
const UserPayloadKey = "user_payload"

// UserPayload is the authenticated caller, taken from the JWT claims.
type UserPayload struct {
	ID    int
	Email string
	Role  string
}

// IsAdmin reports whether the caller has the admin role.
func (u UserPayload) IsAdmin() bool {
	return u.Role == "admin"
}

// NewUserPayload builds a payload from raw JWT claim values.
func NewUserPayload(userID, email, role any) UserPayload {
	payload := UserPayload{}
	switch v := userID.(type) {
	case float64:
		payload.ID = int(v)
	case int:
		payload.ID = v
	case int64:
		payload.ID = int(v)
	}
	payload.Email, _ = email.(string)
	payload.Role, _ = role.(string)
	return payload
}

// GetUserPayload returns the authenticated caller. ok is false when the
// request did not pass through the auth middleware or carried no user id.
func GetUserPayload(c *fiber.Ctx) (payload UserPayload, ok bool) {
	payload, ok = c.Locals(UserPayloadKey).(UserPayload)
	return payload, ok && payload.ID > 0
}
