// services/job-service/internal/publisher/publisher.go
package publisher

import "context"

type Publisher interface {
	Publish(ctx context.Context, body []byte) error
	Close() error
}
