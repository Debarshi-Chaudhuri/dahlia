package dto

// HealthCheckRequest represents request for health check
type HealthCheckRequest struct {
	// No body fields
}

// HealthCheckResponse represents response for health check
type HealthCheckResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
}
