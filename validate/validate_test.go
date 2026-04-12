package validate_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	mkerrors "github.com/pemako/mk/errors"
	"github.com/pemako/mk/validate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type createReq struct {
	Name  string `json:"name"  validate:"required,min=2,max=50"`
	Email string `json:"email" validate:"required,email"`
	Age   int    `json:"age"   validate:"min=0,max=150"`
}

func TestStruct_valid(t *testing.T) {
	err := validate.Struct(&createReq{Name: "Alice", Email: "alice@example.com", Age: 30})
	assert.NoError(t, err)
}

func TestStruct_invalid(t *testing.T) {
	err := validate.Struct(&createReq{Name: "A", Email: "not-an-email", Age: 30})
	require.Error(t, err)
	assert.True(t, mkerrors.Is(err, mkerrors.ErrBadRequest) || mkerrors.Code(err) == 400)
}

func TestBind_valid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/", func(c *gin.Context) {
		var req createReq
		if err := validate.Bind(c, &req); err != nil {
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

func TestBind_invalidEmail(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/", func(c *gin.Context) {
		var req createReq
		if err := validate.Bind(c, &req); err != nil {
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
