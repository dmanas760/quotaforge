package response

import (
	"encoding/json"
	"net/http"
)

// ErrorEnvelope is the standard error response shape.
type ErrorEnvelope struct {
	Error ErrorBody `json:"error"`
}

// ErrorBody holds error details.
type ErrorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

// WriteJSON writes any value as JSON with the given status code.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WriteError writes a standard error JSON envelope.
func WriteError(w http.ResponseWriter, status int, code, message, requestID string) {
	WriteJSON(w, status, ErrorEnvelope{Error: ErrorBody{
		Code:      code,
		Message:   message,
		RequestID: requestID,
	}})
}
