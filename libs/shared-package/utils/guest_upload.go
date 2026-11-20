package utils

// Guests (people applying without an account) upload their documents before
// their account exists. The file service hands such a guest a random secret
// token on their first upload; the files are owned by a value derived from it,
// and the application service re-assigns them to the new user once the
// application is submitted with the same token.

const (
	// GuestUploadOwnerPrefix marks files owned by a guest upload token.
	GuestUploadOwnerPrefix = "guest:"

	// GuestUploadTokenBytes is the size of a generated token; the token is the
	// hex encoding, so it is twice as many characters.
	GuestUploadTokenBytes = 16

	// MaxGuestFiles is how many documents one guest token may upload.
	MaxGuestFiles = 5
)

// GuestUploadOwner returns the files.owner_id for a guest upload token. Only a
// hash of the token is stored, so reading the files table never reveals a
// token that could still be used to claim the files. It fits owner_id's 50
// characters.
func GuestUploadOwner(token string) string {
	return GuestUploadOwnerPrefix + HashData(token)[:40]
}

// ValidGuestUploadToken reports whether a client-supplied token has the shape
// SecureRandString(GuestUploadTokenBytes) produces.
func ValidGuestUploadToken(token string) bool {
	if len(token) != GuestUploadTokenBytes*2 {
		return false
	}
	for _, r := range token {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}
