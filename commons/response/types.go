package response

type StandardResponse struct {
	Status    StatusEnum `json:"status"`
	ErrorCode int        `json:"errorCode"`
	Message   string     `json:"message"`
	Data      any        `json:"data"`
	Errors    []Errors   `json:"errors"`
}

type StatusEnum string

const (
	StatusSuccess        StatusEnum = "SUCCESS"
	StatusPartialSuccess StatusEnum = "PARTIAL_SUCCESS"
	StatusFailed         StatusEnum = "FAILED"
)

type Errors struct {
	ErrorCode int    `json:"errorCode"`
	Message   string `json:"message"`
	Data      any    `json:"data"`
}