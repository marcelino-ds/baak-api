package utils

import (
	"context"
	"encoding/json"
	"errors"
	"log"
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
	WriteJSONResponseWithStatus(w, http.StatusOK, data)
}

func WriteJSONResponseWithStatus(w http.ResponseWriter, status int, data interface{}) {
	writeResponse(w, status, Response{
		Success: status < 400,
		Data:    data,
	})
}

func WriteErrorResponse(w http.ResponseWriter, status int, message string) {
	code := "INTERNAL_ERROR"
	if status == http.StatusMethodNotAllowed {
		w.Header().Set("Allow", "GET, OPTIONS")
		code = "METHOD_NOT_ALLOWED"
	}
	WriteErrorResponseWithCode(w, status, message, code)
}

func WriteErrorResponseWithCode(w http.ResponseWriter, status int, message string, code string) {
	writeResponse(w, status, Response{
		Success: false,
		Error:   message,
		Code:    code,
	})
}

func writeResponse(w http.ResponseWriter, status int, response Response) {
	body, err := json.Marshal(response)
	if err != nil {
		log.Printf("JSON response encoding failed: %v", err)
		status = http.StatusInternalServerError
		body = []byte(`{"success":false,"error":"Internal server error","code":"INTERNAL_ERROR"}`)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(append(body, '\n'))
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

func WriteConfigurationError(w http.ResponseWriter) {
	WriteErrorResponseWithCode(w, http.StatusInternalServerError, "The API configuration is invalid.", "CONFIGURATION_ERROR")
}

func WriteHTTPError(w http.ResponseWriter, err error) {
	if err == nil {
		WriteInternalServerError(w)
		return
	}
	message := err.Error()
	if errors.Is(err, context.DeadlineExceeded) {
		WriteErrorResponseWithCode(w, http.StatusGatewayTimeout, "The backend server took too long to respond.", "UPSTREAM_TIMEOUT")
		return
	}
	if errors.Is(err, context.Canceled) {
		WriteErrorResponseWithCode(w, http.StatusRequestTimeout, "The request was canceled before the backend response was ready.", "REQUEST_CANCELED")
		return
	}
	var upstreamStatus upstreamStatusError
	switch {
	case errors.Is(err, ErrPDFUnavailable):
		WriteErrorResponseWithCode(w, http.StatusServiceUnavailable, "PDF extraction runtime is unavailable.", "PDF_UNAVAILABLE")
		return
	case errors.Is(err, ErrPDFBusy):
		w.Header().Set("Retry-After", "2")
		WriteErrorResponseWithCode(w, http.StatusServiceUnavailable, "PDF extraction capacity is full.", "PDF_BUSY")
		return
	case errors.Is(err, ErrPDFInvalid):
		WriteErrorResponseWithCode(w, http.StatusUnprocessableEntity, "The document is not a supported PDF or exceeds extraction limits.", "PDF_UNSUPPORTED")
		return
	case errors.Is(err, ErrCircuitOpen):
		w.Header().Set("Retry-After", "30")
		WriteErrorResponseWithCode(w, http.StatusServiceUnavailable, "Service temporarily unavailable while the upstream recovers.", "CIRCUIT_OPEN")
		return
	case errors.Is(err, ErrCloudflareBlocked):
		WriteErrorResponseWithCode(w, http.StatusServiceUnavailable, "The BAAK website returned a Cloudflare challenge.", "CLOUDFLARE_BLOCKED")
		return
	case errors.As(err, &upstreamStatus):
		switch upstreamStatus.code {
		case http.StatusForbidden:
			WriteErrorResponseWithCode(w, http.StatusServiceUnavailable, "The BAAK website rejected the request.", "CLOUDFLARE_BLOCKED")
		case http.StatusTooManyRequests:
			WriteErrorResponseWithCode(w, http.StatusTooManyRequests, "The backend server is rate limiting requests.", "RATE_LIMITED")
		case http.StatusRequestTimeout, http.StatusGatewayTimeout:
			WriteErrorResponseWithCode(w, http.StatusGatewayTimeout, "The backend server timed out.", "UPSTREAM_TIMEOUT")
		default:
			WriteErrorResponseWithCode(w, http.StatusBadGateway, "The backend server returned an unsuccessful response.", "UPSTREAM_ERROR")
		}
		return
	case errors.Is(err, ErrFlareSolverrBusy):
		w.Header().Set("Retry-After", "1")
		WriteErrorResponseWithCode(
			w,
			http.StatusServiceUnavailable,
			"FlareSolverr is busy. Please try again shortly.",
			"FLARESOLVERR_BUSY",
		)
		return
	case errors.Is(err, ErrFlareSolverr):
		WriteErrorResponseWithCode(w, http.StatusBadGateway, "FlareSolverr could not complete the request.", "FLARESOLVERR_ERROR")
		return
	case errors.Is(err, ErrUpstream), errors.Is(err, ErrUnexpectedPage):
		WriteErrorResponseWithCode(w, http.StatusBadGateway, "The backend server did not return usable data.", "UPSTREAM_ERROR")
		return
	}

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

	if errors.Is(err, ErrUnexpectedPage) {
		WriteErrorResponseWithCode(
			w,
			http.StatusBadGateway,
			"The backend server returned an unexpected page format.",
			"UPSTREAM_ERROR",
		)
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
