package dto

// PaginationResponse represents common pagination metadata
type PaginationResponse struct {
	Count     int    `json:"count"`
	NextToken string `json:"next_token,omitempty"`
}
