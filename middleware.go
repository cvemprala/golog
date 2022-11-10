package golog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
)

// ResponseWriterRecorder wraps the http.ResponseWriter to order to retrieve information from the response
// The http.ResponseWriter itself doesn't provide interface to access its data.
type ResponseWriterRecorder struct {
	status         int
	body           []byte
	responseWriter http.ResponseWriter
	isStatusSet    bool
}

// NewResponseWriterRecorder creates a new ResponseWriterRecorder wrapping the underlying
// http.ResponseWriter.
func NewResponseWriterRecorder(w http.ResponseWriter) *ResponseWriterRecorder {
	return &ResponseWriterRecorder{
		status:         200,
		responseWriter: w,
	}
}

// Status returns the status code of the response.
func (r *ResponseWriterRecorder) Status() int {
	return r.status
}

// Body returns the body bytes of the response
func (r *ResponseWriterRecorder) Body() []byte {
	return r.body
}

// Header wraps the underlying http.ResponseWriter's Header() method.
func (r *ResponseWriterRecorder) Header() http.Header {
	return r.responseWriter.Header()
}

// WriteHeader wraps the underlying http.ResponseWriter and captures the response cpde.
func (r *ResponseWriterRecorder) WriteHeader(statusCode int) {
	r.responseWriter.WriteHeader(statusCode)
	r.status = statusCode
	r.isStatusSet = true
}

// Write wraps the underlying http.ResponseWriter and captures the response body.
//
// As defined by the http.ResponseWriter interface, if WriteHeader has not yet
// been called, Write calls WriteHeader(http.StatusOK) before writing the data.
func (r *ResponseWriterRecorder) Write(b []byte) (int, error) {
	if !r.isStatusSet {
		r.WriteHeader(http.StatusOK)
	}
	r.body = b
	return r.responseWriter.Write(b)
}

type contextKey string

// List of context keys
const (
	ContextKeyRequestID contextKey = "requestId"
	ContextKeyLogger    contextKey = "logger"
)

// GetRequestID returns the request ID in the context
func GetRequestID(ctx context.Context) string {
	requestID := ctx.Value(ContextKeyRequestID)

	if requestID == nil {
		return "Unknown"
	}

	return requestID.(string)
}

// WithLogger returns a new context with the provided logger.
func WithLogger(ctx context.Context, logger Logger) context.Context {
	return context.WithValue(ctx, ContextKeyLogger, logger)
}

// GetLogger retrieves the current logger from the context. If no logger is
// available, the default logger is returned.
func GetLogger(ctx context.Context) Logger {
	logger := ctx.Value(ContextKeyLogger)

	if logger == nil {
		return New(INFO, os.Stdout)
	}

	return logger.(Logger)
}

// MiddlewareOptions struct
type MiddlewareOptions struct {
	LogResponse bool
}

// NewMiddleware creates a new middleware for logging
func NewMiddleware(next http.Handler, logger Logger) http.Handler {
	return NewMiddlewareWithOptions(next, logger, MiddlewareOptions{
		LogResponse: true,
	})
}

// NewMiddlewareWithOptions creates a new middleware for logging
func NewMiddlewareWithOptions(next http.Handler, logger Logger, options MiddlewareOptions) http.Handler {
	if &logger == nil {
		logger = New(INFO, os.Stdout)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// attach request ID to the request
		requestID := uuid.New().String()
		ctx := context.WithValue(r.Context(), ContextKeyRequestID, requestID)
		r = r.WithContext(ctx)

		// attach the request ID to the logger
		loggerWithRequestID := logger.WithFields(map[string]interface{}{string(ContextKeyRequestID): requestID})
		r = r.WithContext(WithLogger(r.Context(), loggerWithRequestID))

		logRequest(loggerWithRequestID, r)

		responseWriterRecorder := NewResponseWriterRecorder(w)
		if options.LogResponse {
			defer logResponse(loggerWithRequestID, start, r, responseWriterRecorder)
		}

		responseWriterRecorder.Header().Add("Request-ID", requestID)
		next.ServeHTTP(responseWriterRecorder, r)
	})
}

func logRequest(logger Logger, r *http.Request) {
	var requestBody interface{}
	if r.Body != http.NoBody {
		buf, err := ioutil.ReadAll(r.Body)
		if err != nil {
			logger = logger.WithFields(map[string]interface{}{"bodyError": err})
		} else {
			r.Body = ioutil.NopCloser(bytes.NewBuffer(buf))
			bufCopy := bytes.NewBuffer(buf)
			if err := json.Unmarshal(bufCopy.Bytes(), &requestBody); err != nil {
				requestBody = bufCopy.String()
				logger = logger.WithFields(map[string]interface{}{"bodyError": err})
			}
		}
	}

	m := convertRequestBody(requestBody)

	logger.WithFields(map[string]interface{}{
		"remoteAddr":  r.RemoteAddr,
		"protocol":    r.Proto,
		"method":      r.Method,
		"header":      r.Header,
		"uri":         r.RequestURI,
		"userAgent":   r.UserAgent(),
		"requestBody": m,
	}).Debugln("")
}

func convertRequestBody(requestBody interface{}) interface{} {
	switch requestBody.(type) {
	case map[string]interface{}:
		m, _ := requestBody.(map[string]interface{})
		return m
	case []interface{}:
		m, _ := requestBody.([]interface{})
		return m
	case string:
		m, _ := requestBody.(string)
		return m
	default:
		return requestBody
	}
}

func logResponse(logger Logger, start time.Time, r *http.Request, w *ResponseWriterRecorder) {
	var responseBody interface{}
	if w.Body() != nil {
		if err := json.Unmarshal(w.Body(), &responseBody); err != nil {
			responseBody = string(w.Body())
			logger = logger.WithFields(map[string]interface{}{"bodyError": err})
		}
	}
	logger.WithFields(map[string]interface{}{
		"duration":     time.Since(start),
		"header":       w.Header(),
		"responseBody": responseBody,
		"status":       w.Status(),
		"api":          fmt.Sprintf("%s_%s", r.Method, r.URL.Path),
	}).Debugln("")
}
