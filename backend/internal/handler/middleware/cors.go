package middleware

import (
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// CORS returns a middleware that handles Cross-Origin Resource Sharing.
func CORS(allowedOrigins []string) gin.HandlerFunc {
	return cors.New(cors.Config{
		AllowOrigins:     allowedOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Authorization", "Content-Type", "Range", "If-Range", "X-Request-ID"},
		ExposeHeaders:    []string{"X-Request-ID", "Content-Range", "Accept-Ranges", "Content-Length", "Content-Disposition", "Retry-After"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	})
}
