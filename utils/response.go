package utils

import (
	"encoding/json"
	"net/http"
	"strings"
)

type Response struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
	Code    string      `json:"code,omitempty"`
}

func WriteJSONResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Response{
		Success: true,
		Data:    data,
	})
}

func WriteErrorResponse(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(Response{
		Success: false,
		Error:   message,
	})
}

func WriteErrorResponseWithCode(w http.ResponseWriter, status int, message string, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(Response{
		Success: false,
		Error:   message,
		Code:    code,
	})
}

func WriteValidationError(w http.ResponseWriter, message string) {
	WriteErrorResponseWithCode(w, http.StatusBadRequest, message, "VALIDATION_ERROR")
}

func WriteNotFoundError(w http.ResponseWriter) {
	WriteErrorResponseWithCode(w, http.StatusNotFound, "Resource not found", "NOT_FOUND")
}

func WriteInternalServerError(w http.ResponseWriter) {
	WriteErrorResponseWithCode(w, http.StatusInternalServerError, "Internal server error", "INTERNAL_ERROR")
}

func WriteHTTPError(w http.ResponseWriter, err error) {
	message := err.Error()

	// Handle Cloudflare-related errors
	if strings.Contains(message, "access forbidden (403)") ||
		strings.Contains(message, "unexpected status code: 403") {
		WriteErrorResponseWithCode(w, http.StatusServiceUnavailable,
			"The BAAK website is protected by Cloudflare. Please try again later or contact the administrator to configure FlareSolverr.",
			"CLOUDFLARE_BLOCKED")
		return
	}

	// Handle circuit breaker open
	if strings.Contains(message, "circuit breaker is open") ||
		strings.Contains(message, "service temporarily unavailable") {
		WriteErrorResponseWithCode(w, http.StatusServiceUnavailable,
			"Service temporarily unavailable due to repeated connection failures. The system will automatically retry shortly.",
			"CIRCUIT_OPEN")
		return
	}

	// Handle FlareSolverr errors
	if strings.Contains(message, "FlareSolverr") {
		WriteErrorResponseWithCode(w, http.StatusBadGateway,
			"Failed to bypass Cloudflare protection. Please try again later.",
			"FLARESOLVERR_ERROR")
		return
	}

	// Handle rate limiting from upstream
	if strings.Contains(message, "unexpected status code: 429") {
		WriteErrorResponseWithCode(w, http.StatusTooManyRequests,
			"Too many requests to the backend server. Please try again later.",
			"RATE_LIMITED")
		return
	}

	// Handle 5xx errors from upstream
	if strings.Contains(message, "unexpected status code: 5") {
		WriteErrorResponseWithCode(w, http.StatusBadGateway,
			"The backend server is experiencing issues. Please try again later.",
			"UPSTREAM_ERROR")
		return
	}

	// Handle session establishment failures
	if strings.Contains(message, "failed to establish session") {
		WriteErrorResponseWithCode(w, http.StatusServiceUnavailable,
			"Unable to establish connection with BAAK website. The site may be down or blocking connections.",
			"SESSION_ERROR")
		return
	}

	// Default to internal server error for other cases
	WriteInternalServerError(w)
}

