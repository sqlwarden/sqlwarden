package response

type APIErrorEnvelope struct {
	Error APIError `json:"error"`
}

type APIError struct {
	Code        string            `json:"code"`
	Message     string            `json:"message"`
	Reason      string            `json:"reason,omitempty"`
	FieldErrors map[string]string `json:"field_errors,omitempty"`
	Errors      []string          `json:"errors,omitempty"`
	Details     any               `json:"details,omitempty"`
}
