package metrics

import (
	"time"

	"github.com/gin-gonic/gin"
)

// HTTPMetricsMiddleware creates a Gin middleware for collecting HTTP metrics
func HTTPMetricsMiddleware(collector *MetricsCollector) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}

		// Get request size
		requestSize := c.Request.ContentLength

		// Process request
		c.Next()

		// Calculate duration
		duration := time.Since(start)

		// Get response size
		responseSize := int64(c.Writer.Size())

		// Record metrics
		collector.RecordHTTPRequest(
			c.Request.Method,
			path,
			c.Writer.Status(),
			duration,
			requestSize,
			responseSize,
		)
	}
}
