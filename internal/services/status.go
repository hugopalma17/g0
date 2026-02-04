package services

// ReadStatusFile is a small helper for future health/status endpoints.
// For now it returns the provided default.
func ReadStatusFile(_ string, defaultVal string) string {
	return defaultVal
}
