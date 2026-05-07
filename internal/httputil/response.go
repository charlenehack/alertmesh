package httputil

import (
	"net/http"

	restful "github.com/emicklei/go-restful/v3"
)

// Response is the standard API response envelope.
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// PagedData wraps list results with pagination metadata.
type PagedData struct {
	Items interface{} `json:"items"`
	Total int64       `json:"total"`
}

// writeJSON ignores the encode error: by the time go-restful would surface
// one, the response status has already been written and the client is
// halfway through reading the body, so the connection is the right place
// for the failure to bubble up — there's no useful caller recovery path.
func writeJSON(resp *restful.Response, status int, body Response) {
	_ = resp.WriteHeaderAndEntity(status, body)
}

func Success(resp *restful.Response, data interface{}) {
	writeJSON(resp, http.StatusOK, Response{
		Code:    0,
		Message: "success",
		Data:    data,
	})
}

func Created(resp *restful.Response, data interface{}) {
	writeJSON(resp, http.StatusCreated, Response{
		Code:    0,
		Message: "created",
		Data:    data,
	})
}

func Error(resp *restful.Response, status int, msg string) {
	writeJSON(resp, status, Response{
		Code:    status,
		Message: msg,
	})
}

func BadRequest(resp *restful.Response, msg string) {
	Error(resp, http.StatusBadRequest, msg)
}

func Unauthorized(resp *restful.Response) {
	Error(resp, http.StatusUnauthorized, "unauthorized")
}

func Forbidden(resp *restful.Response) {
	Error(resp, http.StatusForbidden, "permission denied")
}

func NotFound(resp *restful.Response) {
	Error(resp, http.StatusNotFound, "not found")
}

func InternalError(resp *restful.Response, msg string) {
	Error(resp, http.StatusInternalServerError, msg)
}
