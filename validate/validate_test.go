package validate_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mkerrors "github.com/pemako/mk/errors"
	"github.com/pemako/mk/validate"
)

type createReq struct {
	Name  string `json:"name"  validate:"required,min=2,max=50"`
	Email string `json:"email" validate:"required,email"`
	Age   int    `json:"age"   validate:"min=0,max=150"`
}

type queryReq struct {
	Status string `form:"status" validate:"required,oneof=active inactive"`
	Limit  int    `form:"limit"  validate:"min=1,max=100"`
}

type uriReq struct {
	ID int64 `uri:"id" validate:"required,min=1"`
}

type formReq struct {
	Title string `form:"title" validate:"required,min=1"`
}

func TestStruct_valid(t *testing.T) {
	err := validate.Struct(&createReq{Name: "Alice", Email: "alice@example.com", Age: 30})
	assert.NoError(t, err)
}

func TestStruct_invalid(t *testing.T) {
	err := validate.Struct(&createReq{Name: "A", Email: "not-an-email", Age: 30})
	require.Error(t, err)
	assert.Equal(t, 400, mkerrors.Code(err))
}

func TestBindJSON_valid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/", func(c *gin.Context) {
		var req createReq
		if err := validate.BindJSON(c, &req); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, req)
	})

	body, _ := json.Marshal(map[string]any{"name": "Alice", "email": "alice@example.com", "age": 25})
	w := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(context.Background(), "POST", "/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
}

func TestBindJSON_invalidEmail(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/", func(c *gin.Context) {
		var req createReq
		if err := validate.BindJSON(c, &req); err != nil {
			c.JSON(mkerrors.Code(err), err)
			return
		}
		c.JSON(200, req)
	})

	body, _ := json.Marshal(map[string]any{"name": "Alice", "email": "bad-email", "age": 25})
	w := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(context.Background(), "POST", "/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	assert.Equal(t, 400, w.Code)
}

func TestBindQuery_valid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/", func(c *gin.Context) {
		var req queryReq
		if err := validate.BindQuery(c, &req); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, req)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/?status=active&limit=10", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
}

func TestBindQuery_invalid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/", func(c *gin.Context) {
		var req queryReq
		if err := validate.BindQuery(c, &req); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, req)
	})

	w := httptest.NewRecorder()
	// status must be active|inactive
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/?status=unknown&limit=10", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, 400, w.Code)
}

func TestBindURI_valid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/users/:id", func(c *gin.Context) {
		var req uriReq
		if err := validate.BindURI(c, &req); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, req)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/users/42", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
}

func TestBindURI_invalid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/users/:id", func(c *gin.Context) {
		var req uriReq
		if err := validate.BindURI(c, &req); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, req)
	})

	w := httptest.NewRecorder()
	// id=0 fails validate:"min=1"
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/users/0", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, 400, w.Code)
}

func TestBindForm_valid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/", func(c *gin.Context) {
		var req formReq
		if err := validate.BindForm(c, &req); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, req)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(context.Background(), "POST", "/",
		strings.NewReader("title=hello"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
}
