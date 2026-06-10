package response

type APIErrorEnvelope struct {
	Error APIError `json:"error"`
}

type APIError struct {
	Code        string            `json:"code"`
	Message     string            `json:"message"`
	FieldErrors map[string]string `json:"field_errors,omitempty"`
	Errors      []string          `json:"errors,omitempty"`
	Details     any               `json:"details,omitempty"`
}
